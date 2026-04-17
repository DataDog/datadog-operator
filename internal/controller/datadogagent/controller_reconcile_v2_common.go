// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/helm"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	updateSucceeded = "UpdateSucceeded"
	createSucceeded = "CreateSucceeded"
	patchSucceeded  = "PatchSucceeded"
)

type updateDepStatusComponentFunc func(deployment *appsv1.Deployment, newStatus *v2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)

func (r *Reconciler) createOrUpdateDeployment(parentLogger logr.Logger, dda *v2alpha1.DatadogAgent, deployment *appsv1.Deployment, newStatus *v2alpha1.DatadogAgentStatus, updateStatusFunc updateDepStatusComponentFunc) (reconcile.Result, error) {
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
	currentDeployment, err := r.getCurrentDeployment(dda, deployment)
	if err != nil {
		return reconcile.Result{}, err
	}

	alreadyExists := true
	if currentDeployment == nil {
		logger.Info("deployment is not found")
		alreadyExists = false
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
		if restartDeployment(deployment, currentDeployment) {
			// if its a helm cluster checks runner deployment, delete the workload and dependents
			helmCCRDeployment := currentDeployment.GetLabels()[kubernetes.AppKubernetesComponentLabelKey] == "clusterchecks-agent" && helm.IsHelmMigration(dda)
			if helmCCRDeployment {
				if err = deleteAllWorkloadsAndDependentsBackground(context.TODO(), logger, r.client, currentDeployment, currentDeployment.GetLabels()[apicommon.AgentDeploymentComponentLabelKey]); err != nil {
					return result, err
				}
				return result, nil
			} else {
				if err = deleteObjectAndOrphanDependents(context.TODO(), logger, r.client, deployment, deployment.GetLabels()[apicommon.AgentDeploymentComponentLabelKey]); err != nil {
					return result, err
				}
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

	// By comparing annotations, we reconcile either instantly if the spec annotation changed or after the reconcile period
	// if only the annotations changed.
	if !maps.Equal(currentDDAI.Annotations, ddai.Annotations) {
		r.log.Info("updating DatadogAgentInternal", "ns", ddai.Namespace, "name", ddai.Name)
		if err := kubernetes.UpdateFromObject(context.TODO(), r.client, ddai, currentDDAI.ObjectMeta); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) addDDAIStatusToDDAStatus(status *v2alpha1.DatadogAgentStatus, ddai metav1.ObjectMeta) error {
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

// getCurrentDeployment returns the current deployment for a given DDA
func (r *Reconciler) getCurrentDeployment(dda, deployment metav1.Object) (*appsv1.Deployment, error) {
	// Helm-migrated deployment
	if helm.IsHelmMigration(dda) {
		componentType := deployment.GetLabels()[apicommon.AgentDeploymentComponentLabelKey]
		if componentType == "" {
			r.log.Info("No component label found in deployment, using default")
			componentType = constants.DefaultAgentResourceSuffix
		}
		dsList := appsv1.DeploymentList{}
		matchLabels := client.MatchingLabels{
			kubernetes.AppKubernetesManageByLabelKey:   "Helm",
			kubernetes.AppKubernetesNameLabelKey:       dda.GetName(),
			apicommon.AgentDeploymentComponentLabelKey: componentType,
		}

		if err := r.client.List(context.TODO(), &dsList, matchLabels); err != nil {
			return nil, err
		}

		switch len(dsList.Items) {
		case 0: // Migration has completed; check for default deployment
			r.log.Info("Helm-managed deployment has been migrated, checking for default deployment", "component", componentType)
		case 1:
			r.log.Info("Found Helm-managed deployment", "name", dsList.Items[0].Name)
			return &dsList.Items[0], nil
		default:
			return nil, fmt.Errorf("expected 1 deployment for datadog helm release: %s, got %d", dda.GetName(), len(dsList.Items))
		}
	}

	// Default deployment
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}
	currentDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, currentDeployment); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("deployment is not found")
			return nil, nil
		}
		return nil, err
	}
	return currentDeployment, nil
}

func restartDeployment(deployment, currentDeployment *appsv1.Deployment) bool {
	// name change
	if deployment.Name != currentDeployment.Name {
		return true
	}

	// selectors are immutable
	if !maps.Equal(deployment.Spec.Selector.MatchLabels, currentDeployment.Spec.Selector.MatchLabels) {
		return true
	}

	return false
}

func IsEqualStatus(current *v2alpha1.DatadogAgentStatus, newStatus *v2alpha1.DatadogAgentStatus) bool {
	if current == nil && newStatus == nil {
		return true
	}
	if current == nil || newStatus == nil {
		return false
	}

	if !condition.IsEqualDaemonSetStatus(current.Agent, newStatus.Agent) ||
		!apiequality.Semantic.DeepEqual(current.RemoteConfigConfiguration, newStatus.RemoteConfigConfiguration) {
		return false
	}

	// Compare AgentList order-insensitively to avoid spurious diffs and prevent panics
	if len(current.AgentList) != len(newStatus.AgentList) {
		return false
	}
	if len(current.AgentList) > 0 {
		// Clone to avoid mutating the original slices
		ac := slices.Clone(current.AgentList)
		bc := slices.Clone(newStatus.AgentList)

		sortByNameFunc := func(a, b *v2alpha1.DaemonSetStatus) int {
			var an, bn string
			if a != nil {
				an = a.DaemonsetName
			}
			if b != nil {
				bn = b.DaemonsetName
			}
			return strings.Compare(an, bn)
		}

		slices.SortFunc(ac, sortByNameFunc)
		slices.SortFunc(bc, sortByNameFunc)
		for i := range ac {
			if !condition.IsEqualDaemonSetStatus(ac[i], bc[i]) {
				return false
			}
		}
	}

	if !condition.IsEqualDeploymentStatus(current.ClusterAgent, newStatus.ClusterAgent) ||
		!condition.IsEqualDeploymentStatus(current.ClusterChecksRunner, newStatus.ClusterChecksRunner) {
		return false
	}

	if !apiequality.Semantic.DeepEqual(current.Experiment, newStatus.Experiment) {
		return false
	}

	return condition.IsEqualConditions(current.Conditions, newStatus.Conditions)
}
