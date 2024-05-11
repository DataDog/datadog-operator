// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package strategy

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
	eds "github.com/DataDog/extendeddaemonset/controllers/extendeddaemonset"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/conditions"
	podUtils "github.com/DataDog/extendeddaemonset/pkg/controller/utils/pod"
)

// ManageCanaryDeployment used to manage ReplicaSet in Canary state.
func ManageCanaryDeployment(client client.Client, daemonset *v1alpha1.ExtendedDaemonSet, params *Parameters) (*Result, error) {
	// Manage canary status
	result := manageCanaryStatus(daemonset.GetAnnotations(), params, time.Now())

	err := ensureCanaryPodLabels(client, params)
	if err != nil {
		result.Result = requeuePromptly()
	}

	// Populate list of unscheduled pods on nodes due to resource limitation
	result.UnscheduledNodesDueToResourcesConstraints = manageUnscheduledPodNodes(params.UnscheduledPods)

	// Cleanup Pods
	err = cleanupPods(client, params.Logger, result.NewStatus, params.PodToCleanUp)
	if err != nil {
		result.Result = requeuePromptly()
	}

	return result, nil
}

// manageCanaryStatus manages ReplicaSet status in Canary state.
func manageCanaryStatus(annotations map[string]string, params *Parameters, now time.Time) *Result {
	result := &Result{}
	result.NewStatus = params.NewStatus.DeepCopy()
	result.NewStatus.Status = string(ReplicaSetStatusCanary)

	// TODO: these are set here as well as in manageCanaryPodFailures. Consider simplifying.
	result.IsFailed = eds.IsCanaryDeploymentFailed(params.Replicaset)
	result.IsPaused, result.PausedReason = eds.IsCanaryDeploymentPaused(annotations, params.Replicaset)
	result.IsUnpaused = eds.IsCanaryDeploymentUnpaused(annotations)

	var (
		metaNow = metav1.NewTime(now)

		desiredPods, currentPods, availablePods, readyPods int32

		needRequeue            bool
		podsToCheckForRestarts []*v1.Pod

		podsToCreate []*NodeItem
		podsToDelete []*NodeItem
	)

	// First scan canary node list for pods to create or delete
	for _, nodeName := range params.CanaryNodes {
		node := params.NodeByName[nodeName]
		desiredPods++
		if pod, ok := params.PodByNodeName[node]; ok {
			if pod == nil {
				podsToCreate = append(podsToCreate, node)

				continue
			}

			if pod.DeletionTimestamp != nil {
				needRequeue = true

				continue
			}

			if !compareCurrentPodWithNewPod(params, pod, node) {
				podsToDelete = append(podsToDelete, node)

				continue
			}

			currentPods++
			if podUtils.IsPodAvailable(pod, 0, metaNow) {
				availablePods++
			}
			if podUtils.IsPodReady(pod) {
				readyPods++
			}

			podsToCheckForRestarts = append(podsToCheckForRestarts, pod)
		}
	}

	// Update result to reflect active pods currently experiencing restarts or failures otherwise
	// potentially placing canary into paused or failed state
	manageCanaryPodFailures(podsToCheckForRestarts, params, result, now)

	// Update pod counts
	result.NewStatus.Desired = desiredPods
	result.NewStatus.Ready = readyPods
	result.NewStatus.Available = availablePods
	result.NewStatus.Current = currentPods

	result.PodsToDelete = podsToDelete

	// Do not create any pods if canary is paused or failed
	if len(podsToCreate) != 0 && !result.IsPaused && !result.IsFailed {
		result.PodsToCreate = podsToCreate
		needRequeue = true
	}

	params.Logger.V(1).Info("NewStatus", "Desired", desiredPods, "Ready", readyPods, "Available", availablePods, "Current", currentPods)
	params.Logger.V(1).Info(
		"Result",
		"PodsToCreate", len(result.PodsToCreate),
		"PodsToDelete", len(result.PodsToDelete),
		"IsFailed", result.IsFailed,
		"FailedReason", result.FailedReason,
		"IsPaused", result.IsPaused,
		"PausedReason", result.PausedReason,
	)

	if needRequeue || !result.IsFailed && !result.IsPaused && result.NewStatus.Desired != result.NewStatus.Ready {
		result.Result = requeuePromptly()
	}

	return result
}

// manageCanaryPodFailures checks if canary should be failed or paused due to restarts or other failures.
// Note that pausing the canary will have no effect if it has been validated or failed.
func manageCanaryPodFailures(pods []*v1.Pod, params *Parameters, result *Result, now time.Time) {
	var (
		canary               = params.Strategy.Canary
		autoPauseEnabled     = *canary.AutoPause.Enabled
		autoPauseMaxRestarts = int(*canary.AutoPause.MaxRestarts)

		autoFailEnabled             = *canary.AutoFail.Enabled
		autoFailMaxRestarts         = int(*canary.AutoFail.MaxRestarts)
		autoFailMaxRestartsDuration *metav1.Duration
		autoFailCanaryTimeout       *metav1.Duration

		newRestartTime      time.Time
		restartingPodStatus string

		cannotStart          bool
		cannotStartPodReason v1alpha1.ExtendedDaemonSetStatusReason
		cannotStartPodStatus string
	)

	if canary.AutoFail.MaxRestartsDuration != nil {
		autoFailMaxRestartsDuration = canary.AutoFail.MaxRestartsDuration
	}

	if canary.AutoFail.CanaryTimeout != nil {
		autoFailCanaryTimeout = canary.AutoFail.CanaryTimeout
	}

	startCondition := conditions.GetExtendedDaemonSetReplicaSetStatusCondition(result.NewStatus, v1alpha1.ConditionTypeCanary)
	restartCondition := conditions.GetExtendedDaemonSetReplicaSetStatusCondition(params.NewStatus, v1alpha1.ConditionTypePodRestarting)

	// Note that we still need to evaluate restarts regardless of the enabled autoPause or autoFail
	// since we maintain the restarting condition that can be checked by canary.noRestartsDuration
	for _, pod := range pods {
		restartCount, highRestartReason := podUtils.HighestRestartCount(pod)
		if restartCount != 0 {
			restartTime, recentRestartReason := podUtils.MostRecentRestart(pod)
			if restartTime.After(newRestartTime) {
				newRestartTime = restartTime
				restartingPodStatus = fmt.Sprintf("Pod %s restarting with reason: %s", pod.ObjectMeta.Name, string(recentRestartReason))
			}
		}

		var cannotStartReason v1alpha1.ExtendedDaemonSetStatusReason
		cannotStart, cannotStartReason = podUtils.CannotStart(pod)
		// We do not want to raise an error yet if MaxSlowStartDuration is specified and not exceeded
		if cannotStart && params.Strategy.Canary.AutoPause.MaxSlowStartDuration != nil && !now.After(pod.Status.StartTime.Time.Add(params.Strategy.Canary.AutoPause.MaxSlowStartDuration.Duration)) {
			cannotStart = false
			cannotStartReason = v1alpha1.ExtendedDaemonSetStatusReasonUnknown
		} else if cannotStart {
			cannotStartPodStatus = fmt.Sprintf("Pod %s cannot start with reason: %s", pod.ObjectMeta.Name, string(cannotStartReason))
			cannotStartPodReason = cannotStartReason
		} else if autoPauseEnabled && podUtils.PendingCreate(pod) && params.Strategy.Canary.AutoPause.MaxSlowStartDuration != nil {
			if now.After(pod.Status.StartTime.Time.Add(params.Strategy.Canary.AutoPause.MaxSlowStartDuration.Duration)) {
				params.Logger.Info(
					"PendingCreate",
					"PodName", pod.ObjectMeta.Name,
					"Exceeded", params.Strategy.Canary.AutoPause.MaxSlowStartDuration.Duration,
				)
				cannotStart = true
				cannotStartReason = v1alpha1.ExtendedDaemonSetStatusSlowStartTimeoutExceeded
				cannotStartPodStatus = fmt.Sprintf("Pod %s cannot start with reason: %s", pod.ObjectMeta.Name, cannotStartReason)
				cannotStartPodReason = cannotStartReason
			}
		}

		// If the Canary is already marked as failed on a previous iteration, continue
		if result.IsFailed {
			continue
		}

		switch {
		// Autofail: restarts count exceeds maxRestarts
		case autoFailEnabled && restartCount > autoFailMaxRestarts:
			result.IsFailed = true
			result.FailedReason = highRestartReason
			params.Logger.Info(
				"AutoFailed",
				"RestartCount", restartCount,
				"MaxRestarts", autoFailMaxRestarts,
				"Reason", highRestartReason,
			)
		// Autofail: restarts duration timeout
		case autoFailEnabled && autoFailMaxRestartsDuration != nil && restartCondition != nil && restartCondition.LastUpdateTime.Sub(restartCondition.LastTransitionTime.Time) > autoFailMaxRestartsDuration.Duration:
			result.IsFailed = true
			result.FailedReason = v1alpha1.ExtendedDaemonSetStatusRestartsTimeoutExceeded
			params.Logger.Info(
				"AutoFailed",
				"Reason", v1alpha1.ExtendedDaemonSetStatusRestartsTimeoutExceeded,
			)
		// Autofail: general timeout
		case autoFailEnabled && startCondition != nil && autoFailCanaryTimeout != nil && now.Sub(startCondition.LastTransitionTime.Time) > autoFailCanaryTimeout.Duration:
			result.IsFailed = true
			result.FailedReason = v1alpha1.ExtendedDaemonSetStatusTimeoutExceeded
			params.Logger.Info(
				"AutoFailed",
				"Reason", v1alpha1.ExtendedDaemonSetStatusTimeoutExceeded,
			)
		case result.IsUnpaused:
			// Unpausing is a manual action and takes precedence
			result.IsPaused = false
			result.PausedReason = ""
		case autoPauseEnabled:
			// Handle cases related to failure to start states
			if cannotStart {
				result.IsPaused = true
				result.PausedReason = cannotStartReason
				params.Logger.Info(
					"AutoPaused",
					"CannotStart", true,
					"Reason", cannotStartReason,
				)
			} else if restartCount > autoPauseMaxRestarts {
				result.IsPaused = true
				result.PausedReason = highRestartReason
				params.Logger.Info(
					"AutoPaused",
					"RestartCount", restartCount,
					"MaxRestarts", autoFailMaxRestarts,
					"Reason", highRestartReason,
				)
			}
		}
	}

	// Update Failed and Paused condition
	conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(result.NewStatus, metav1.NewTime(now), v1alpha1.ConditionTypeCanaryFailed, conditions.BoolToCondition(result.IsFailed), string(result.FailedReason), "", false, true)
	conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(result.NewStatus, metav1.NewTime(now), v1alpha1.ConditionTypeCanaryPaused, conditions.BoolToCondition(result.IsPaused), string(result.PausedReason), "", false, true)

	var lastRestartTime time.Time
	if restartCondition != nil {
		lastRestartTime = restartCondition.LastUpdateTime.Time
	}

	// Track pod restart condition in the status
	if !newRestartTime.IsZero() && newRestartTime.After(lastRestartTime) {
		conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(
			result.NewStatus,
			metav1.NewTime(newRestartTime),
			v1alpha1.ConditionTypePodRestarting,
			v1.ConditionTrue,
			string(cannotStartPodReason),
			restartingPodStatus,
			false,
			true,
		)
	}

	conditionStatus := v1.ConditionFalse
	// Track pod restart condition in the status
	if cannotStart {
		conditionStatus = v1.ConditionTrue
		params.Logger.Info(
			"UpdateCannotStartCondition",
			"Reason", cannotStartPodReason,
			"State", cannotStartPodStatus,
		)
	}

	conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(
		result.NewStatus,
		metav1.NewTime(now),
		v1alpha1.ConditionTypePodCannotStart,
		conditionStatus,
		string(cannotStartPodReason),
		cannotStartPodStatus,
		false,
		true,
	)

	if result.IsFailed {
		result.NewStatus.Status = string(ReplicaSetStatusCanaryFailed)
	}
}

// ensureCanaryPodLabels ensures that canary label is set on canary pods.
func ensureCanaryPodLabels(client client.Client, params *Parameters) error {
	for _, nodeName := range params.CanaryNodes {
		node := params.NodeByName[nodeName]
		if pod, ok := params.PodByNodeName[node]; ok && pod != nil {
			if pod.Labels == nil {
				continue
			}

			// Check if that ERS is the Pod's parent, to not label a pod
			// from the previous ERS (if the previous Pod is not yet deleted).
			if pod.Labels[v1alpha1.ExtendedDaemonSetReplicaSetNameLabelKey] == params.Replicaset.GetName() {
				params.Logger.V(1).Info("Add Canary label", "podName", pod.GetName())
				err := addPodLabel(params.Logger,
					client,
					pod,
					v1alpha1.ExtendedDaemonSetReplicaSetCanaryLabelKey,
					v1alpha1.ExtendedDaemonSetReplicaSetCanaryLabelValue,
				)
				if err != nil {
					params.Logger.Error(err, fmt.Sprintf("Couldn't add the canary label for pod '%s/%s', will retry later", pod.GetNamespace(), pod.GetName()))

					return err
				}
			}
		}
	}

	return nil
}

func requeueIn(requeueAfter time.Duration) reconcile.Result {
	return reconcile.Result{
		Requeue:      true,
		RequeueAfter: requeueAfter,
	}
}

func requeuePromptly() reconcile.Result {
	return requeueIn(time.Second)
}
