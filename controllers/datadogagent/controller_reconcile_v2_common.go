// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"time"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

type updateDepStatusComponentFunc func(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)
type updateDSStatusComponentFunc func(daemonset *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)
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
			updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, "UpdateFailed", "Unable to update Deployment")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updateDeployment.Name, updateDeployment.Namespace, deploymentKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(updateDeployment, newStatus, now, metav1.ConditionTrue, "UpdateSucceeded", "Deployment updated")
	} else {
		now := metav1.NewTime(time.Now())

		err = r.client.Create(context.TODO(), deployment)
		if err != nil {
			updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, "CreateFailed", "Unable to create Deployment")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(deployment.Name, deployment.Namespace, deploymentKind, datadog.CreationEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(deployment, newStatus, now, metav1.ConditionTrue, "CreateSucceeded", "Deployment created")
	}

	logger.Info("Creating Deployment")

	return result, err
}

func (r *Reconciler) createOrUpdateDaemonset(parentLogger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, daemonset *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateStatusFunc updateDSStatusComponentFunc) (reconcile.Result, error) {
	logger := parentLogger.WithValues("daemonset.Namespace", daemonset.Namespace, "daemonset.Name", daemonset.Name)

	var result reconcile.Result
	var err error

	// Set DatadogAgent instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, daemonset, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// From here the PodTemplateSpec should be ready, we can generate the hash that will be used to compare this daemonset with the current one (if it exists).
	var hash string
	hash, err = comparison.SetMD5DatadogAgentGenerationAnnotation(&daemonset.ObjectMeta, daemonset.Spec)
	if err != nil {
		return result, err
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
		// check if same hash
		needUpdate := !comparison.IsSameSpecMD5Hash(hash, currentDaemonset.GetAnnotations())
		if !needUpdate {
			// Even if the DaemonSet is still the same, its status might have
			// changed (for example, the number of pods ready). This call is
			// needed to keep the agent status updated.
			now := metav1.NewTime(time.Now())
			newStatus.Agent = datadoghqv2alpha1.UpdateDaemonSetStatus(currentDaemonset, newStatus.Agent, &now)

			// Stop reconcile loop since DaemonSet hasn't changed
			return reconcile.Result{}, nil
		}

		logger.Info("Updating Daemonset")

		// TODO: these parameters can be added to the override.PodTemplateSpec. (It exists in v1alpha1)
		keepAnnotationsFilter := ""
		keepLabelsFilter := ""

		// Copy possibly changed fields
		updateDaemonset := daemonset.DeepCopy()
		updateDaemonset.Spec = *daemonset.Spec.DeepCopy()
		updateDaemonset.Annotations = mergeAnnotationsLabels(logger, currentDaemonset.GetAnnotations(), daemonset.GetAnnotations(), keepAnnotationsFilter)
		updateDaemonset.Labels = mergeAnnotationsLabels(logger, currentDaemonset.GetLabels(), daemonset.GetLabels(), keepLabelsFilter)

		now := metav1.NewTime(time.Now())
		err = kubernetes.UpdateFromObject(context.TODO(), r.client, updateDaemonset, currentDaemonset.ObjectMeta)
		if err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updateDaemonset.Name, updateDaemonset.Namespace, daemonSetKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(updateDaemonset, newStatus, now, metav1.ConditionTrue, "DaemonsetUpdated", "Daemonset updated")
	} else {
		now := metav1.NewTime(time.Now())

		err = r.client.Create(context.TODO(), daemonset)
		if err != nil {
			updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, "CreateFailed", "Unable to create Daemonset")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(daemonset.Name, daemonset.Namespace, daemonSetKind, datadog.CreationEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(daemonset, newStatus, now, metav1.ConditionTrue, "CreateSucceeded", "Daemonset created")
	}

	logger.Info("Creating Daemonset")

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
		// check if same hash
		needUpdate := !comparison.IsSameSpecMD5Hash(hash, currentEDS.GetAnnotations())
		if !needUpdate {
			// Even if the EDS is still the same, its status might have
			// changed (for example, the number of pods ready). This call is
			// needed to keep the agent status updated.
			now := metav1.NewTime(time.Now())
			newStatus.Agent = datadoghqv2alpha1.UpdateExtendedDaemonSetStatus(currentEDS, newStatus.Agent, &now)

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
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updateEDS.Name, updateEDS.Namespace, extendedDaemonSetKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(updateEDS, newStatus, now, metav1.ConditionTrue, "ExtendedDaemonSetUpdated", "ExtendedDaemonSet updated")
	} else {
		now := metav1.NewTime(time.Now())

		err = r.client.Create(context.TODO(), eds)
		if err != nil {
			updateStatusFunc(nil, newStatus, now, metav1.ConditionFalse, "CreateFailed", "Unable to create ExtendedDaemonSet")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(eds.Name, eds.Namespace, extendedDaemonSetKind, datadog.CreationEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(eds, newStatus, now, metav1.ConditionTrue, "CreateSucceeded", "ExtendedDaemonSet created")
	}

	logger.Info("Creating ExtendedDaemonSet")

	return result, err
}
