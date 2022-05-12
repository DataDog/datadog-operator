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
)

type updateStatusComponentFunc func(newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, message string)

func (r *Reconciler) createOrUpdateDeployment(parentLogger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateStatusFunc updateStatusComponentFunc) (reconcile.Result, error) {
	logger := parentLogger.WithValues("deployment.Namespace", deployment.Namespace, "deployment.Name", deployment.Name)

	var result reconcile.Result
	var err error

	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, deployment, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// From here the PodTemplateSpec should be ready, we can generate the hash that will be use to compare this deployment with the current (if exist).
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
	alreadyExist := true
	err = r.client.Get(context.TODO(), nsName, currentDeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("deployment is not found")
			alreadyExist = false
		} else {
			logger.Error(err, "unexpected error during deployment get")
			return reconcile.Result{}, err
		}
	}

	if alreadyExist {
		// check if same hash
		needUpdate := !comparison.IsSameSpecMD5Hash(hash, currentDeployment.GetAnnotations())
		if !needUpdate {
			// no need to update to stop here the process
			return reconcile.Result{}, nil
		}

		logger.Info("Updating Deployment")

		// TODO: these parameter can be added to the override.PodTemplateSpec. (it exist in v1alpha)
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
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updateDeployment.Name, updateDeployment.Namespace, deploymentKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(newStatus, now, metav1.ConditionTrue, "Deployment updated")
	} else {
		now := metav1.NewTime(time.Now())

		err = r.client.Create(context.TODO(), deployment)
		if err != nil {
			updateStatusFunc(newStatus, now, metav1.ConditionFalse, "Unable to create Deployment")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(deployment.Name, deployment.Namespace, deploymentKind, datadog.CreationEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(newStatus, now, metav1.ConditionTrue, "Deployment created")
	}

	logger.Info("Creating Deployment")

	return result, err
}

func (r *Reconciler) createOrUpdateDaemonset(parentLogger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, daemonset *appsv1.DaemonSet, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateStatusFunc updateStatusComponentFunc) (reconcile.Result, error) {
	logger := parentLogger.WithValues("daemonset.Namespace", daemonset.Namespace, "daemonset.Name", daemonset.Name)

	var result reconcile.Result
	var err error

	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, daemonset, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// From here the PodTemplateSpec should be ready, we can generate the hash that will be use to compare this deployment with the current (if exist).
	var hash string
	hash, err = comparison.SetMD5DatadogAgentGenerationAnnotation(&daemonset.ObjectMeta, daemonset.Spec)
	if err != nil {
		return result, err
	}

	// Get the current deployment and compare
	nsName := types.NamespacedName{
		Name:      daemonset.GetName(),
		Namespace: daemonset.GetNamespace(),
	}

	currentDaemonset := &appsv1.DaemonSet{}
	alreadyExist := true
	err = r.client.Get(context.TODO(), nsName, currentDaemonset)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("daemonset is not found")
			alreadyExist = false
		} else {
			logger.Error(err, "unexpected error during daemonset get")
			return reconcile.Result{}, err
		}
	}

	if alreadyExist {
		// check if same hash
		needUpdate := !comparison.IsSameSpecMD5Hash(hash, currentDaemonset.GetAnnotations())
		if !needUpdate {
			// no need to update to stop here the process
			return reconcile.Result{}, nil
		}

		logger.Info("Updating Deployment")

		// TODO: these parameter can be added to the override.PodTemplateSpec. (it exist in v1alpha)
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
		event := buildEventInfo(updateDaemonset.Name, updateDaemonset.Namespace, deploymentKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(newStatus, now, metav1.ConditionTrue, "Daemonset updated")
	} else {
		now := metav1.NewTime(time.Now())

		err = r.client.Create(context.TODO(), daemonset)
		if err != nil {
			updateStatusFunc(newStatus, now, metav1.ConditionFalse, "Unable to create Daemonset")
			return reconcile.Result{}, err
		}
		event := buildEventInfo(daemonset.Name, daemonset.Namespace, daemonSetKind, datadog.CreationEvent)
		r.recordEvent(dda, event)
		updateStatusFunc(newStatus, now, metav1.ConditionTrue, "Daemonset created")
	}

	logger.Info("Creating Daemonset")

	return result, err
}
