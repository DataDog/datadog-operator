// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"fmt"
	"time"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	updateSucceeded = "UpdateSucceeded"
	createSucceeded = "CreateSucceeded"
	patchSucceeded  = "PatchSucceeded"
)

type updateDepStatusComponentFunc func(deployment *appsv1.Deployment, newStatus *v1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)
type updateDSStatusComponentFunc func(daemonsetName string, daemonset *appsv1.DaemonSet, newStatus *v1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)
type updateEDSStatusComponentFunc func(eds *edsv1alpha1.ExtendedDaemonSet, newStatus *v1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)

func (r *Reconciler) createOrUpdateDeployment(parentLogger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, deployment *appsv1.Deployment, newStatus *v1alpha1.DatadogAgentInternalStatus, updateStatusFunc updateDepStatusComponentFunc) (reconcile.Result, error) {
	logger := parentLogger.WithValues("deployment.Namespace", deployment.Namespace, "deployment.Name", deployment.Name)

	var result reconcile.Result
	var err error

	// Set DatadogAgentInternal instance as the owner and controller
	if err = controllerutil.SetControllerReference(ddai, deployment, r.scheme); err != nil {
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
			patch, e := createOwnerReferencePatch(currentDeployment.OwnerReferences, ddai, ddai.GetObjectKind().GroupVersionKind())
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
		r.recordEvent(ddai, event)
		updateStatusFunc(updateDeployment, newStatus, now, metav1.ConditionTrue, updateSucceeded, "Deployment updated")
	} else {
		now := metav1.NewTime(time.Now())

		err = r.client.Create(context.TODO(), deployment)
		if err != nil {
			updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, createSucceeded, "Unable to create Deployment")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(deployment.Name, deployment.Namespace, kubernetes.DeploymentKind, datadog.CreationEvent)
		r.recordEvent(ddai, event)
		updateStatusFunc(deployment, newStatus, now, metav1.ConditionTrue, createSucceeded, "Deployment created")
	}

	logger.Info("Creating Deployment")

	return result, err
}

func (r *Reconciler) createOrUpdateDaemonset(parentLogger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, daemonset *appsv1.DaemonSet, newStatus *v1alpha1.DatadogAgentInternalStatus, updateStatusFunc updateDSStatusComponentFunc) (reconcile.Result, error) {
	logger := parentLogger.WithValues("daemonset.Namespace", daemonset.Namespace, "daemonset.Name", daemonset.Name)

	var result reconcile.Result
	var err error

	// Set DatadogAgent instance as the owner and controller
	if err = controllerutil.SetControllerReference(ddai, daemonset, r.scheme); err != nil {
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
			patch, e := createOwnerReferencePatch(currentDaemonset.OwnerReferences, ddai, ddai.GetObjectKind().GroupVersionKind())
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
		now := metav1.Now()

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

		// TODO: remove in 1.8.0 when v1alpha1 is removed
		// Spec.Selector is an immutable field and changing it leads to an error.
		// Template.Labels must include Spec.Selector.
		// See https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/#pod-selector
		daemonset.Spec.Selector = currentDaemonset.Spec.Selector
		daemonset.Spec.Template.Labels = ensureSelectorInPodTemplateLabels(logger, daemonset.Spec.Selector, daemonset.Spec.Template.Labels)

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
			newStatus.Agent = condition.UpdateDaemonSetStatusDDAI(currentDaemonset.Name, currentDaemonset, newStatus.Agent, &now)

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

		logger.Info("Updating Daemonset")
		err = kubernetes.UpdateFromObject(context.TODO(), r.client, updateDaemonset, currentDaemonset.ObjectMeta)
		if err != nil {
			updateStatusFunc(updateDaemonset.Name, updateDaemonset, newStatus, now, metav1.ConditionFalse, updateSucceeded, "Unable to update Daemonset")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updateDaemonset.Name, updateDaemonset.Namespace, kubernetes.DaemonSetKind, datadog.UpdateEvent)
		r.recordEvent(ddai, event)
		updateStatusFunc(updateDaemonset.Name, updateDaemonset, newStatus, now, metav1.ConditionTrue, updateSucceeded, "Daemonset updated")

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
		r.recordEvent(ddai, event)
		updateStatusFunc(daemonset.Name, daemonset, newStatus, now, metav1.ConditionTrue, createSucceeded, "Daemonset created")
	}

	return result, err
}

func (r *Reconciler) createOrUpdateExtendedDaemonset(parentLogger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, eds *edsv1alpha1.ExtendedDaemonSet, newStatus *v1alpha1.DatadogAgentInternalStatus, updateStatusFunc updateEDSStatusComponentFunc) (reconcile.Result, error) {
	logger := parentLogger.WithValues("ExtendedDaemonSet.Namespace", eds.Namespace, "ExtendedDaemonSet.Name", eds.Name)

	var result reconcile.Result
	var err error

	// Set DatadogAgent instance as the owner and controller
	if err = controllerutil.SetControllerReference(ddai, eds, r.scheme); err != nil {
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
			patch, e := createOwnerReferencePatch(currentEDS.OwnerReferences, ddai, ddai.GetObjectKind().GroupVersionKind())
			if e != nil {
				logger.Error(e, "Unable to patch ExtendedDaemonSet owner reference")
				updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, patchSucceeded, "Unable to patch ExtendedDaemonSet owner reference")
				return reconcile.Result{}, e
			}
			// use merge patch to replace the entire existing owner reference list
			err = r.client.Patch(context.TODO(), currentEDS, client.RawPatch(types.MergePatchType, patch))
			if err != nil {
				logger.Error(err, "Unable to patch ExtendedDaemonSet owner reference")
				updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, patchSucceeded, "Unable to patch ExtendedDaemonSet owner reference")
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
			newStatus.Agent = condition.UpdateExtendedDaemonSetStatusDDAI(currentEDS, newStatus.Agent, &now)

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
		r.recordEvent(ddai, event)
		updateStatusFunc(updateEDS, newStatus, now, metav1.ConditionTrue, updateSucceeded, "ExtendedDaemonSet updated")
	} else {
		now := metav1.NewTime(time.Now())

		err = r.client.Create(context.TODO(), eds)
		if err != nil {
			updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, createSucceeded, "Unable to create ExtendedDaemonSet")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(eds.Name, eds.Namespace, kubernetes.ExtendedDaemonSetKind, datadog.CreationEvent)
		r.recordEvent(ddai, event)
		updateStatusFunc(eds, newStatus, now, metav1.ConditionTrue, createSucceeded, "ExtendedDaemonSet created")
	}

	logger.Info("Creating ExtendedDaemonSet")

	return result, err
}

// TODO: remove in 1.8.0 when v1alpha1 is removed
// ensureSelectorInPodTemplateLabels checks that a label selector's MatchLabels
// are present in the pod template labels. If the label is missing, it adds it
// to the pod template labels. If the value doesn't match, it changes the label
// value to match the selector.
// If the selector labels aren't present in the pod template labels, there will
// be a `selector does not match template labels` error when updating the agent
func ensureSelectorInPodTemplateLabels(logger logr.Logger, selector *metav1.LabelSelector, labels map[string]string) map[string]string {
	if selector != nil {
		if labels == nil {
			labels = map[string]string{}
		}
		for k, v := range selector.MatchLabels {
			value, ok := labels[k]
			if !ok {
				logger.Info("Selector not in template labels, adding to template labels", "selector label", fmt.Sprintf("%s: %s", k, v))
				labels[k] = v
			}
			if value != v {
				logger.Info("Selector value does not match template labels, modifying template labels", "selector label", fmt.Sprintf("%s: %s", k, v), "template label", fmt.Sprintf("%s: %s", k, value))
				labels[k] = v
			}
		}
	}

	return labels
}

func IsEqualStatus(current *v1alpha1.DatadogAgentInternalStatus, newStatus *v1alpha1.DatadogAgentInternalStatus) bool {
	if !condition.IsEqualDaemonSetStatus(current.Agent, newStatus.Agent) ||
		!apiequality.Semantic.DeepEqual(current.RemoteConfigConfiguration, newStatus.RemoteConfigConfiguration) {
		return false
	}

	if !condition.IsEqualDeploymentStatus(current.ClusterAgent, newStatus.ClusterAgent) ||
		!condition.IsEqualDeploymentStatus(current.ClusterChecksRunner, newStatus.ClusterChecksRunner) {
		return false
	}

	return condition.IsEqualConditions(current.Conditions, newStatus.Conditions)
}
