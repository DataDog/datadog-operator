// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package pod

import (
	"fmt"
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils/affinity"
)

// GetContainerStatus extracts the status of container "name" from "statuses".
// It also returns if "name" exists.
func GetContainerStatus(statuses []v1.ContainerStatus, name string) (v1.ContainerStatus, bool) {
	for i := range statuses {
		if statuses[i].Name == name {
			return statuses[i], true
		}
	}

	return v1.ContainerStatus{}, false
}

// GetExistingContainerStatus extracts the status of container "name" from "statuses".
func GetExistingContainerStatus(statuses []v1.ContainerStatus, name string) v1.ContainerStatus {
	status, _ := GetContainerStatus(statuses, name)

	return status
}

// IsPodScheduled return true if it is already assigned to a Node.
func IsPodScheduled(pod *v1.Pod) (string, bool) {
	isScheduled := pod.Spec.NodeName != ""
	nodeName := affinity.GetNodeNameFromAffinity(pod.Spec.Affinity)

	return nodeName, isScheduled
}

// IsPodAvailable returns true if a pod is available; false otherwise.
// Precondition for an available pod is that it must be ready. On top
// of that, there are two cases when a pod can be considered available:
// 1. minReadySeconds == 0, or
// 2. LastTransitionTime (is set) + minReadySeconds < current time
func IsPodAvailable(pod *v1.Pod, minReadySeconds int32, now metav1.Time) bool {
	if !IsPodReady(pod) {
		return false
	}

	c := GetPodReadyCondition(pod.Status)
	minReadySecondsDuration := time.Duration(minReadySeconds) * time.Second
	if minReadySeconds == 0 || !c.LastTransitionTime.IsZero() && c.LastTransitionTime.Add(minReadySecondsDuration).Before(now.Time) {
		return true
	}

	return false
}

// GetNodeNameFromPod returns the NodeName from Pod spec. either from `pod.Spec.NodeName`
// if the pod is already scheduled, of from the NodeAffinity.
func GetNodeNameFromPod(pod *v1.Pod) (string, error) {
	if pod.Spec.NodeName != "" {
		return pod.Spec.NodeName, nil
	}
	nodeName := affinity.GetNodeNameFromAffinity(pod.Spec.Affinity)
	if nodeName == "" {
		return "", fmt.Errorf("unable to retrieve nodeName for the pod: %s/%s", pod.Namespace, pod.Name)
	}

	return nodeName, nil
}

// IsPodReady returns true if a pod is ready; false otherwise.
func IsPodReady(pod *v1.Pod) bool {
	return IsPodReadyConditionTrue(pod.Status)
}

// IsPodReadyConditionTrue returns true if a pod is ready; false otherwise.
func IsPodReadyConditionTrue(status v1.PodStatus) bool {
	condition := GetPodReadyCondition(status)

	return condition != nil && condition.Status == v1.ConditionTrue
}

// GetPodReadyCondition extracts the pod ready condition from the given status and returns that.
// Returns nil if the condition is not present.
func GetPodReadyCondition(status v1.PodStatus) *v1.PodCondition {
	_, condition := GetPodCondition(&status, v1.PodReady)

	return condition
}

// GetPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetPodCondition(status *v1.PodStatus, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if status == nil {
		return -1, nil
	}

	return GetPodConditionFromList(status.Conditions, conditionType)
}

// GetPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func GetPodConditionFromList(conditions []v1.PodCondition, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}

	return -1, nil
}

func containerStatusList(pod *v1.Pod) []v1.ContainerStatus {
	containersStatus := append(pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses...)

	return append(containersStatus, pod.Status.EphemeralContainerStatuses...)
}

// HighestRestartCount checks if a pod in the Canary deployment is restarting
// This returns the count and the "reason" for the pod with the most restarts.
func HighestRestartCount(pod *v1.Pod) (int, datadoghqv1alpha1.ExtendedDaemonSetStatusReason) {
	// track the highest number of restarts among pod containers
	var (
		restartCount int32
		reason       datadoghqv1alpha1.ExtendedDaemonSetStatusReason
	)

	for _, s := range containerStatusList(pod) {
		if s.RestartCount > restartCount {
			restartCount = s.RestartCount
			reason = datadoghqv1alpha1.ExtendedDaemonSetStatusReasonUnknown
			if s.LastTerminationState != (v1.ContainerState{}) && *s.LastTerminationState.Terminated != (v1.ContainerStateTerminated{}) {
				if s.LastTerminationState.Terminated.Reason != "" { // The Reason field is optional and can be empty
					reason = datadoghqv1alpha1.ExtendedDaemonSetStatusReason(s.LastTerminationState.Terminated.Reason)
				}
			}
		}
	}

	return int(restartCount), reason
}

// MostRecentRestart returns the most recent restart time for a pod or the time.
func MostRecentRestart(pod *v1.Pod) (time.Time, datadoghqv1alpha1.ExtendedDaemonSetStatusReason) {
	var (
		restartTime time.Time
		reason      datadoghqv1alpha1.ExtendedDaemonSetStatusReason
	)
	for _, s := range containerStatusList(pod) {
		if s.RestartCount != 0 && s.LastTerminationState != (v1.ContainerState{}) && s.LastTerminationState.Terminated != (&v1.ContainerStateTerminated{}) {
			if s.LastTerminationState.Terminated.FinishedAt.After(restartTime) {
				restartTime = s.LastTerminationState.Terminated.FinishedAt.Time
				reason = datadoghqv1alpha1.ExtendedDaemonSetStatusReasonUnknown
				if s.LastTerminationState.Terminated.Reason != "" { // The Reason field is optional and can be empty
					reason = datadoghqv1alpha1.ExtendedDaemonSetStatusReason(s.LastTerminationState.Terminated.Reason)
				}
			}
		}
	}

	return restartTime, reason
}

var cannotStartReasons = map[string]struct{}{
	"ErrImagePull":               {},
	"ImagePullBackOff":           {},
	"ImageInspectError":          {},
	"ErrImageNeverPull":          {},
	"RegistryUnavailable":        {},
	"InvalidImageName":           {},
	"CreateContainerConfigError": {},
	"CreateContainerError":       {},
	"PreStartHookError":          {},
	"PostStartHookError":         {},
	"PreCreateHookError":         {},
}

// IsCannotStartReason returns true for a reason that is considered an abnormal cannot start condition.
func IsCannotStartReason(reason string) bool {
	_, found := cannotStartReasons[reason]

	return found
}

// CannotStart returns true if the Pod is currently experiencing abnormal start condition.
func CannotStart(pod *v1.Pod) (bool, datadoghqv1alpha1.ExtendedDaemonSetStatusReason) {
	for _, s := range containerStatusList(pod) {
		if s.State.Waiting != nil && IsCannotStartReason(s.State.Waiting.Reason) {
			return true, convertReasonToEDSStatusReason(s.State.Waiting.Reason)
		}
	}

	return false, datadoghqv1alpha1.ExtendedDaemonSetStatusReasonUnknown
}

func convertReasonToEDSStatusReason(reason string) datadoghqv1alpha1.ExtendedDaemonSetStatusReason {
	t := datadoghqv1alpha1.ExtendedDaemonSetStatusReason(reason)
	switch t {
	case datadoghqv1alpha1.ExtendedDaemonSetStatusReasonCLB,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonOOM,
		datadoghqv1alpha1.ExtendedDaemonSetStatusRestartsTimeoutExceeded,
		datadoghqv1alpha1.ExtendedDaemonSetStatusSlowStartTimeoutExceeded,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonErrImagePull,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonImagePullBackOff,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonImageInspectError,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonErrImageNeverPull,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonRegistryUnavailable,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonInvalidImageName,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonCreateContainerConfigError,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonCreateContainerError,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonPreStartHookError,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonPostStartHookError,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonPreCreateHookError,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonStartError,
		datadoghqv1alpha1.ExtendedDaemonSetStatusReasonUnknown:
		return t
	default:
		return datadoghqv1alpha1.ExtendedDaemonSetStatusReasonUnknown
	}
}

// PendingCreate returns true if the Pod is pending create (may be an eventually resolving state).
func PendingCreate(pod *v1.Pod) bool {
	for _, s := range containerStatusList(pod) {
		if s.State.Waiting != nil && s.State.Waiting.Reason == "ContainerCreating" {
			return true
		}
	}

	return false
}

// HasPodSchedulerIssue returns true if a pod remained unscheduled for more than 10 minutes
// or if it stayed in `Terminating` state for longer than its grace period.
func HasPodSchedulerIssue(pod *v1.Pod) bool {
	_, isScheduled := IsPodScheduled(pod)
	if !isScheduled && pod.CreationTimestamp.Add(10*time.Minute).Before(time.Now()) {
		return true
	}

	if pod.DeletionTimestamp != nil && pod.DeletionGracePeriodSeconds != nil &&
		pod.DeletionTimestamp.Add(time.Duration(*pod.DeletionGracePeriodSeconds)*time.Second).Before(time.Now()) {
		return true
	}

	return false
}

// UpdatePodCondition updates existing pod condition or creates a new one. Sets LastTransitionTime to now if the
// status has changed.
// Returns true if pod condition has changed or has been added.
func UpdatePodCondition(status *v1.PodStatus, condition *v1.PodCondition) bool {
	condition.LastTransitionTime = metav1.Now()
	// Try to find this pod condition.
	conditionIndex, oldCondition := GetPodCondition(status, condition.Type)

	if oldCondition == nil {
		// We are adding new pod condition.
		status.Conditions = append(status.Conditions, *condition)

		return true
	}
	// We are updating an existing condition, so we need to check if it has changed.
	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	isEqual := condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason &&
		condition.Message == oldCondition.Message &&
		condition.LastProbeTime.Equal(&oldCondition.LastProbeTime) &&
		condition.LastTransitionTime.Equal(&oldCondition.LastTransitionTime)

	status.Conditions[conditionIndex] = *condition
	// Return true if one of the fields have changed.
	return !isEqual
}

// IsEvicted returns whether the status corresponds to an evicted pod.
func IsEvicted(status *v1.PodStatus) bool {
	if status.Phase == v1.PodFailed && status.Reason == "Evicted" {
		return true
	}

	return false
}

// SortPodByCreationTime return the pods sorted by creation time
// from the newer to the older.
func SortPodByCreationTime(pods []*v1.Pod) []*v1.Pod {
	sort.Sort(podByCreationTimestamp(pods))

	return pods
}

type podByCreationTimestamp []*v1.Pod

func (o podByCreationTimestamp) Len() int      { return len(o) }
func (o podByCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o podByCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name > o[j].Name
	}

	return o[j].CreationTimestamp.Before(&o[i].CreationTimestamp)
}
