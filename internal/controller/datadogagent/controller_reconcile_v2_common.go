// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"time"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	updateSucceeded = "UpdateSucceeded"
	createSucceeded = "CreateSucceeded"
	patchSucceeded  = "PatchSucceeded"

	profileWaitForCanaryKey = "agent.datadoghq.com/profile-wait-for-canary"
)

type updateDepStatusComponentFunc func(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)
type updateDSStatusComponentFunc func(daemonsetName string, daemonset *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)
type updateEDSStatusComponentFunc func(eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)

func (r *Reconciler) createOrUpdateDeployment(parentLogger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateStatusFunc updateDepStatusComponentFunc) (reconcile.Result, error) {
	logger := parentLogger.WithValues("deployment.Namespace", deployment.Namespace, "deployment.Name", deployment.Name)

	var result reconcile.Result
	var err error

	// Set DatadogAgent instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, deployment, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// From here the PodTemplateSpec should be ready, we can generate the hash that will be used to compare this deployment with the current one (if it exists).
	var hash string
	hash, err = comparison.SetMD5DatadogAgentGenerationAnnotation(&deployment.ObjectMeta, deployment.Spec)
	if err != nil {
		return result, err
	}

	// Get the current deployment and compare
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	currentDeployment := &appsv1.Deployment{}
	alreadyExists := true
	err = r.client.Get(context.TODO(), nsName, currentDeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("deployment is not found")
			alreadyExists = false
		} else {
			logger.Error(err, "unexpected error during deployment get")
			return reconcile.Result{}, err
		}
	}

	if alreadyExists {
		// check owner reference
		if shouldUpdateOwnerReference(currentDeployment.OwnerReferences) {
			logger.Info("Updating Deployment owner reference")
			now := metav1.NewTime(time.Now())
			patch, e := createOwnerReferencePatch(currentDeployment.OwnerReferences, dda, dda.GetObjectKind().GroupVersionKind())
			if e != nil {
				logger.Error(e, "Unable to patch Deployment owner reference")
				updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, patchSucceeded, "Unable to patch Deployment owner reference")
				return reconcile.Result{}, e
			}
			// use merge patch to replace the entire existing owner reference list
			err = r.client.Patch(context.TODO(), currentDeployment, client.RawPatch(types.MergePatchType, patch))
			if err != nil {
				logger.Error(err, "Unable to patch Deployment owner reference")
				updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, patchSucceeded, "Unable to patch Deployment owner reference")
				return reconcile.Result{}, err
			}
			logger.Info("Deployment owner reference patched")
		}
		if !maps.Equal(deployment.Spec.Selector.MatchLabels, currentDeployment.Spec.Selector.MatchLabels) {
			if err = deleteObjectAndOrphanDependents(context.TODO(), logger, r.client, deployment, deployment.GetLabels()[apicommon.AgentDeploymentComponentLabelKey]); err != nil {
				return result, err
			}
			return result, nil
		}

		// check if same hash
		needUpdate := !comparison.IsSameSpecMD5Hash(hash, currentDeployment.GetAnnotations())
		if !needUpdate {
			// no need to update hasn't changed
			now := metav1.NewTime(time.Now())
			updateStatusFunc(currentDeployment, newStatus, now, metav1.ConditionTrue, "DeploymentUpToDate", "Deployment up-to-date")
			return reconcile.Result{}, nil
		}

		logger.Info("Updating Deployment")

		// TODO: these parameters can be added to the override.PodTemplateSpec. (It exists in v1alpha1)
		keepAnnotationsFilter := ""
		keepLabelsFilter := ""

		// Copy possibly changed fields
		updateDeployment := deployment.DeepCopy()
		updateDeployment.Spec = *deployment.Spec.DeepCopy()
		updateDeployment.Spec.Replicas = getReplicas(currentDeployment.Spec.Replicas, updateDeployment.Spec.Replicas)
		updateDeployment.Annotations = mergeAnnotationsLabels(logger, currentDeployment.GetAnnotations(), deployment.GetAnnotations(), keepAnnotationsFilter)
		updateDeployment.Labels = mergeAnnotationsLabels(logger, currentDeployment.GetLabels(), deployment.GetLabels(), keepLabelsFilter)

		now := metav1.NewTime(time.Now())
		err = kubernetes.UpdateFromObject(context.TODO(), r.client, updateDeployment, currentDeployment.ObjectMeta)
		if err != nil {
			updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, updateSucceeded, "Unable to update Deployment")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updateDeployment.Name, updateDeployment.Namespace, kubernetes.DeploymentKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(updateDeployment, newStatus, now, metav1.ConditionTrue, updateSucceeded, "Deployment updated")
	} else {
		now := metav1.NewTime(time.Now())

		err = r.client.Create(context.TODO(), deployment)
		if err != nil {
			updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, createSucceeded, "Unable to create Deployment")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(deployment.Name, deployment.Namespace, kubernetes.DeploymentKind, datadog.CreationEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(deployment, newStatus, now, metav1.ConditionTrue, createSucceeded, "Deployment created")
	}

	logger.Info("Creating Deployment")

	return result, err
}

func (r *Reconciler) createOrUpdateDaemonset(parentLogger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, daemonset *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateStatusFunc updateDSStatusComponentFunc, profile *v1alpha1.DatadogAgentProfile) (reconcile.Result, error) {
	logger := parentLogger.WithValues("daemonset.Namespace", daemonset.Namespace, "daemonset.Name", daemonset.Name)

	var result reconcile.Result
	var err error

	// Set DatadogAgent instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, daemonset, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Get the current daemonset and compare
	nsName := types.NamespacedName{
		Name:      daemonset.GetName(),
		Namespace: daemonset.GetNamespace(),
	}

	currentDaemonset := &appsv1.DaemonSet{}
	alreadyExists := true
	err = r.client.Get(context.TODO(), nsName, currentDaemonset)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("daemonset is not found")
			alreadyExists = false
		} else {
			logger.Error(err, "unexpected error during daemonset get")
			return reconcile.Result{}, err
		}
	}

	if alreadyExists {
		// check owner reference
		if shouldUpdateOwnerReference(currentDaemonset.OwnerReferences) {
			logger.Info("Updating Daemonset owner reference")
			now := metav1.NewTime(time.Now())
			patch, e := createOwnerReferencePatch(currentDaemonset.OwnerReferences, dda, dda.GetObjectKind().GroupVersionKind())
			if e != nil {
				logger.Error(e, "Unable to patch Daemonset owner reference")
				updateStatusFunc(currentDaemonset.Name, currentDaemonset, newStatus, now, metav1.ConditionFalse, updateSucceeded, "Unable to patch Daemonset owner reference")
				return reconcile.Result{}, e
			}
			// use merge patch to replace the entire existing owner reference list
			err = r.client.Patch(context.TODO(), currentDaemonset, client.RawPatch(types.MergePatchType, patch))
			if err != nil {
				logger.Error(err, "Unable to patch Daemonset owner reference")
				updateStatusFunc(currentDaemonset.Name, currentDaemonset, newStatus, now, metav1.ConditionFalse, updateSucceeded, "Unable to patch Daemonset owner reference")
				return reconcile.Result{}, err
			}
			logger.Info("Daemonset owner reference patched")
		}

		if !maps.Equal(daemonset.Spec.Selector.MatchLabels, currentDaemonset.Spec.Selector.MatchLabels) {
			if err = deleteObjectAndOrphanDependents(context.TODO(), logger, r.client, daemonset, constants.DefaultAgentResourceSuffix); err != nil {
				return result, err
			}
			return result, nil
		}

		now := metav1.Now()
		if agentprofile.CreateStrategyEnabled() {
			if profile.Status.CreateStrategy != nil {
				profile.Status.CreateStrategy.PodsReady = currentDaemonset.Status.NumberReady
			}
			if shouldCheckCreateStrategyStatus(profile) {
				newStatus := v1alpha1.WaitingStatus

				if int(profile.Status.CreateStrategy.NodesLabeled-currentDaemonset.Status.NumberReady) < int(profile.Status.CreateStrategy.MaxUnavailable) {
					newStatus = v1alpha1.InProgressStatus
				}

				if profile.Status.CreateStrategy.Status != newStatus {
					profile.Status.CreateStrategy.LastTransition = &now
				}
				profile.Status.CreateStrategy.Status = newStatus
			}
			r.updateDAPStatus(logger, profile)
		}

		// When overriding node labels in <1.7.0, the hash could be updated
		// without updating the pod template spec in <1.7.0 since pod template
		// labels were copied over directly from the existing daemonset.
		// With operator <1.7.0, it would look like:
		// 1. Set override node label `abc: def`
		//    a. Daemonset annotation: `agentspechash: 12345`
		// 2. Change label to `abc: xyz`
		//    a. Daemonset annotation: `agentspechash: 67890`
		//    b. Pod template spec still has `abc: def` (set in step 1)
		// To ensure the pod template label updates, we compare the existing
		// daemonset's pod template labels with the new daemonset's pod
		// template labels.
		var currentDaemonsetPodTemplateLabelHash string
		currentDaemonsetPodTemplateLabelHash, err = comparison.GenerateMD5ForSpec(currentDaemonset.Spec.Template.Labels)
		if err != nil {
			return result, err
		}

		// From here the PodTemplateSpec should be ready, we can generate the hash that will be used to compare this daemonset with the current one (if it exists).
		var hash, daemonsetPodTemplateLabelHash string
		hash, err = comparison.SetMD5DatadogAgentGenerationAnnotation(&daemonset.ObjectMeta, daemonset.Spec)
		if err != nil {
			return result, err
		}
		// create a separate hash to compare pod template labels
		daemonsetPodTemplateLabelHash, err = comparison.GenerateMD5ForSpec(daemonset.Spec.Template.Labels)
		if err != nil {
			return result, err
		}

		// check if same hash
		needUpdate := !comparison.IsSameSpecMD5Hash(hash, currentDaemonset.GetAnnotations()) || currentDaemonsetPodTemplateLabelHash != daemonsetPodTemplateLabelHash
		if !needUpdate {
			// Even if the DaemonSet is still the same, its status might have
			// changed (for example, the number of pods ready). This call is
			// needed to keep the agent status updated.
			newStatus.AgentList = condition.UpdateDaemonSetStatus(currentDaemonset.Name, currentDaemonset, newStatus.AgentList, &now)
			newStatus.Agent = condition.UpdateCombinedDaemonSetStatus(newStatus.AgentList)

			// Stop reconcile loop since DaemonSet hasn't changed
			return reconcile.Result{}, nil
		}

		// TODO: these parameters can be added to the override.PodTemplateSpec. (It exists in v1alpha1)
		keepAnnotationsFilter := ""
		keepLabelsFilter := ""

		// Copy possibly changed fields
		updateDaemonset := daemonset.DeepCopy()
		updateDaemonset.Spec = *daemonset.Spec.DeepCopy()
		updateDaemonset.Annotations = mergeAnnotationsLabels(logger, currentDaemonset.GetAnnotations(), daemonset.GetAnnotations(), keepAnnotationsFilter)
		updateDaemonset.Labels = mergeAnnotationsLabels(logger, currentDaemonset.GetLabels(), daemonset.GetLabels(), keepLabelsFilter)
		// manually remove the old profile label because mergeAnnotationsLabels
		// won't filter labels with "datadoghq.com" in the key
		delete(updateDaemonset.Labels, agentprofile.OldProfileLabelKey)

		updateProfileDS := true
		if shouldProfileWaitForCanary(logger, dda.Annotations) {
			ddaLastSpecUpdate := getDDALastUpdatedTime(dda.ManagedFields, dda.CreationTimestamp)
			updateProfileDS, err = r.shouldUpdateProfileDaemonSet(profile, ddaLastSpecUpdate, now)
		}
		if err != nil {
			return result, err
		}

		if updateProfileDS {
			logger.Info("Updating Daemonset")

			err = kubernetes.UpdateFromObject(context.TODO(), r.client, updateDaemonset, currentDaemonset.ObjectMeta)
			if err != nil {
				updateStatusFunc(updateDaemonset.Name, updateDaemonset, newStatus, now, metav1.ConditionFalse, updateSucceeded, "Unable to update Daemonset")
				return reconcile.Result{}, err
			}
			event := buildEventInfo(updateDaemonset.Name, updateDaemonset.Namespace, kubernetes.DaemonSetKind, datadog.UpdateEvent)
			r.recordEvent(dda, event)
			updateStatusFunc(updateDaemonset.Name, updateDaemonset, newStatus, now, metav1.ConditionTrue, updateSucceeded, "Daemonset updated")
		}
	} else {
		// From here the PodTemplateSpec should be ready, we can generate the hash that will be added to this daemonset.
		_, err = comparison.SetMD5DatadogAgentGenerationAnnotation(&daemonset.ObjectMeta, daemonset.Spec)
		if err != nil {
			return result, err
		}

		now := metav1.Now()
		logger.Info("Creating Daemonset")

		err = r.client.Create(context.TODO(), daemonset)
		if err != nil {
			updateStatusFunc(daemonset.Name, nil, newStatus, now, metav1.ConditionFalse, createSucceeded, "Unable to create Daemonset")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(daemonset.Name, daemonset.Namespace, kubernetes.DaemonSetKind, datadog.CreationEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(daemonset.Name, daemonset, newStatus, now, metav1.ConditionTrue, createSucceeded, "Daemonset created")
	}

	return result, err
}

func (r *Reconciler) createOrUpdateExtendedDaemonset(parentLogger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, eds *edsv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateStatusFunc updateEDSStatusComponentFunc) (reconcile.Result, error) {
	logger := parentLogger.WithValues("ExtendedDaemonSet.Namespace", eds.Namespace, "ExtendedDaemonSet.Name", eds.Name)

	var result reconcile.Result
	var err error

	// Set DatadogAgent instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, eds, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// From here the PodTemplateSpec should be ready, we can generate the hash that will be used to compare this extendeddaemonset with the current one (if it exists).
	var hash string
	hash, err = comparison.SetMD5DatadogAgentGenerationAnnotation(&eds.ObjectMeta, eds.Spec)
	if err != nil {
		return result, err
	}

	// Get the current extendeddaemonset and compare
	nsName := types.NamespacedName{
		Name:      eds.GetName(),
		Namespace: eds.GetNamespace(),
	}

	currentEDS := &edsv1alpha1.ExtendedDaemonSet{}
	alreadyExists := true
	err = r.client.Get(context.TODO(), nsName, currentEDS)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ExtendedDaemonSet is not found")
			alreadyExists = false
		} else {
			logger.Error(err, "unexpected error during ExtendedDaemonSet get")
			return reconcile.Result{}, err
		}
	}

	if alreadyExists {
		// check owner reference
		if shouldUpdateOwnerReference(currentEDS.OwnerReferences) {
			logger.Info("Updating ExtendedDaemonSet owner reference")
			now := metav1.NewTime(time.Now())
			patch, e := createOwnerReferencePatch(currentEDS.OwnerReferences, dda, dda.GetObjectKind().GroupVersionKind())
			if e != nil {
				logger.Error(e, "Unable to patch ExtendedDaemonSet owner reference")
				updateStatusFunc(currentEDS, newStatus, now, metav1.ConditionFalse, updateSucceeded, "Unable to patch ExtendedDaemonSet owner reference")
				return reconcile.Result{}, e
			}
			// use merge patch to replace the entire existing owner reference list
			err = r.client.Patch(context.TODO(), currentEDS, client.RawPatch(types.MergePatchType, patch))
			if err != nil {
				logger.Error(err, "Unable to patch ExtendedDaemonSet owner reference")
				updateStatusFunc(currentEDS, newStatus, now, metav1.ConditionFalse, updateSucceeded, "Unable to patch ExtendedDaemonSet owner reference")
				return reconcile.Result{}, err
			}
			logger.Info("ExtendedDaemonSet owner reference patched")
		}
		// check if same hash
		needUpdate := !comparison.IsSameSpecMD5Hash(hash, currentEDS.GetAnnotations())
		if !needUpdate {
			// Even if the EDS is still the same, its status might have
			// changed (for example, the number of pods ready). This call is
			// needed to keep the agent status updated.
			now := metav1.NewTime(time.Now())
			newStatus.AgentList = condition.UpdateExtendedDaemonSetStatus(currentEDS, newStatus.AgentList, &now)
			newStatus.Agent = condition.UpdateCombinedDaemonSetStatus(newStatus.AgentList)

			// Stop reconcile loop since EDS hasn't changed
			return reconcile.Result{}, nil
		}

		logger.Info("Updating ExtendedDaemonSet")

		// TODO: these parameters can be added to the override.PodTemplateSpec. (It exists in v1alpha1)
		keepAnnotationsFilter := ""
		keepLabelsFilter := ""

		// Copy possibly changed fields
		updateEDS := eds.DeepCopy()
		updateEDS.Spec = *eds.Spec.DeepCopy()
		updateEDS.Annotations = mergeAnnotationsLabels(logger, currentEDS.GetAnnotations(), eds.GetAnnotations(), keepAnnotationsFilter)
		updateEDS.Labels = mergeAnnotationsLabels(logger, currentEDS.GetLabels(), eds.GetLabels(), keepLabelsFilter)

		now := metav1.NewTime(time.Now())
		err = kubernetes.UpdateFromObject(context.TODO(), r.client, updateEDS, currentEDS.ObjectMeta)
		if err != nil {
			updateStatusFunc(updateEDS, newStatus, now, metav1.ConditionFalse, updateSucceeded, "Unable to update ExtendedDaemonSet")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updateEDS.Name, updateEDS.Namespace, kubernetes.ExtendedDaemonSetKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(updateEDS, newStatus, now, metav1.ConditionTrue, updateSucceeded, "ExtendedDaemonSet updated")
	} else {
		now := metav1.NewTime(time.Now())

		err = r.client.Create(context.TODO(), eds)
		if err != nil {
			updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, createSucceeded, "Unable to create ExtendedDaemonSet")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(eds.Name, eds.Namespace, kubernetes.ExtendedDaemonSetKind, datadog.CreationEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(eds, newStatus, now, metav1.ConditionTrue, createSucceeded, "ExtendedDaemonSet created")
	}

	logger.Info("Creating ExtendedDaemonSet")

	return result, err
}

func shouldCheckCreateStrategyStatus(profile *v1alpha1.DatadogAgentProfile) bool {
	if profile == nil {
		return false
	}

	if profile.Name == "" || profile.Name == "default" {
		return false
	}

	if profile.Status.CreateStrategy == nil {
		return false
	}

	return profile.Status.CreateStrategy.Status != v1alpha1.CompletedStatus
}

// shouldUpdateProfileDaemonSet determines if we should update a daemonset
// created from a profile based on the canary status, if one exists
// * true causes the daemonset to be updated immediately
// * false causes the reconcile to skip updating the daemonset
func (r *Reconciler) shouldUpdateProfileDaemonSet(profile *v1alpha1.DatadogAgentProfile, ddaLastUpdateTime metav1.Time, now metav1.Time) (bool, error) {
	// eds needs to be enabled
	if !r.options.ExtendedDaemonsetOptions.Enabled {
		return true, nil
	}

	// profiles need to be enabled
	if !r.options.DatadogAgentProfileEnabled {
		return true, nil
	}

	// profile should not be nil or the default profile
	if profile == nil {
		return true, nil
	}
	if agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		return true, nil
	}

	// TODO: check that EDS is for a specific DDA
	edsList := edsv1alpha1.ExtendedDaemonSetList{}
	if err := r.client.List(context.TODO(), &edsList, client.MatchingLabels{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
	}); err != nil {
		return false, err
	}

	for _, eds := range edsList.Items {
		// eds canary was paused
		if eds.Annotations[edsv1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey] == "true" {
			r.log.Info("Waiting to update profile DaemonSet because the canary was paused")
			return false, nil
		}

		// wait if eds has an active canary
		if eds.Status.Canary != nil {
			r.log.Info("Waiting to update profile DaemonSet because of an active canary")
			return false, nil
		}
		// get ers associated with eds
		ersList := edsv1alpha1.ExtendedDaemonSetReplicaSetList{}
		if err := r.client.List(context.TODO(), &ersList, client.MatchingLabels{
			edsv1alpha1.ExtendedDaemonSetNameLabelKey: eds.Name,
		}); err != nil {
			return false, err
		}
		// there should be at least 1 ers
		if len(ersList.Items) == 0 {
			return false, errors.New("there must exist at least 1 ExtendedDaemonSetReplicaSet")
		}
		// wait for canary ers to be cleaned up if there are multiple ers
		if len(ersList.Items) > 1 {
			r.log.Info("Waiting to update profile DaemonSet until unused ExtendedDaemonSetReplicaSets are cleaned up")
			return false, nil
		}
		ers := ersList.Items[0]
		// the eds's active ers should match the ers name
		if eds.Status.ActiveReplicaSet != ers.Name {
			return false, errors.New("ExtendedDaemonSetReplicaSet name does not match ExtendedDaemonSet's active replicaset")
		}

		// add reconcile requeue time buffer to allow eds time to update before making a decision
		if ddaLastUpdateTime.Add(defaultRequeuePeriod).After(now.Time) {
			r.log.Info("Waiting to update profile DaemonSet after DatadogAgent update", "last update", ddaLastUpdateTime, "wait period", defaultRequeuePeriod)
			return false, nil
		}

		hashesMatch := eds.Annotations[constants.MD5AgentDeploymentAnnotationKey] == ers.Annotations[constants.MD5AgentDeploymentAnnotationKey]
		// eds canary was validated manually
		if eds.Annotations[edsv1alpha1.ExtendedDaemonSetCanaryValidAnnotationKey] == ers.Name && hashesMatch {
			r.log.Info("Updating profile DaemonSet because the canary was validated and EDS and ERS hashes match")
			return true, nil
		}

		// wait for canary duration to elapse
		if ddaLastUpdateTime.Add(r.options.ExtendedDaemonsetOptions.CanaryDuration).After(now.Time) {
			r.log.Info("Waiting to update profile DaemonSet because the canary duration has not yet elapsed")
			return false, nil
		}

		// if eds and ers agentspechash match, canary was successful
		if hashesMatch {
			r.log.Info("Updating profile DaemonSet because the EDS and ERS hashes match")
			return true, nil
		}
	}
	return false, nil
}

// getDDALastUpdatedTime returns the latest timestamp from managedFields
// ignoring the `status` subresource. If there are no managed fields, it uses
// the creation timestamp
func getDDALastUpdatedTime(managedFields []metav1.ManagedFieldsEntry, creationTimestamp metav1.Time) metav1.Time {
	lastUpdateTime := creationTimestamp
	for _, mf := range managedFields {
		if mf.Subresource != "status" {
			if mf.Time != nil && mf.Time.After(lastUpdateTime.Time) {
				lastUpdateTime = *mf.Time
			}
		}
	}
	return lastUpdateTime
}

// shouldProfileWaitForCanary returns the value of the profile-wait-for-canary annotation
func shouldProfileWaitForCanary(logger logr.Logger, annotations map[string]string) bool {
	if val, exists := annotations[profileWaitForCanaryKey]; exists {
		waitForCanary, err := strconv.ParseBool(val)
		if err != nil {
			logger.Error(err, "Failed to parse annotation value", "key", profileWaitForCanaryKey, "value", val)
			return false
		}
		return waitForCanary
	}
	return false
}

func (r *Reconciler) createOrUpdateDDAI(ddai *v1alpha1.DatadogAgentInternal) error {
	currentDDAI := &v1alpha1.DatadogAgentInternal{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: ddai.Name, Namespace: ddai.Namespace}, currentDDAI); err != nil {
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "unexpected error during DDAI get")
			return err
		}
		// Create the DDAI object if it doesn't exist
		r.log.Info("creating DatadogAgentInternal", "ns", ddai.Namespace, "name", ddai.Name)
		if err := r.client.Create(context.TODO(), ddai); err != nil {
			return err
		}
		return nil
	}

	if currentDDAI.Annotations[constants.MD5DDAIDeploymentAnnotationKey] != ddai.Annotations[constants.MD5DDAIDeploymentAnnotationKey] {
		r.log.Info("updating DatadogAgentInternal", "ns", ddai.Namespace, "name", ddai.Name)
		if err := kubernetes.UpdateFromObject(context.TODO(), r.client, ddai, currentDDAI.ObjectMeta); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) addDDAIStatusToDDAStatus(status *datadoghqv2alpha1.DatadogAgentStatus, ddai metav1.ObjectMeta) error {
	currentDDAI := &v1alpha1.DatadogAgentInternal{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: ddai.Name, Namespace: ddai.Namespace}, currentDDAI); err != nil {
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "unexpected error during DDAI get")
			return err
		}
		// DDAI not yet created
		return nil
	}

	status.Agent = condition.CombineDaemonSetStatus(status.Agent, currentDDAI.Status.Agent)
	status.ClusterAgent = condition.CombineDeploymentStatus(status.ClusterAgent, currentDDAI.Status.ClusterAgent)
	status.ClusterChecksRunner = condition.CombineDeploymentStatus(status.ClusterChecksRunner, currentDDAI.Status.ClusterChecksRunner)

	// TODO: Add and/or merge conditions once DDAI reconcile PR is merged

	return nil
}
