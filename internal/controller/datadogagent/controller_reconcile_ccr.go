// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (r *Reconciler) reconcileV2ClusterChecksRunner(logger logr.Logger, requiredComponents feature.RequiredComponents, features []feature.Feature, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var result reconcile.Result

	// Start by creating the Default Cluster-Agent deployment
	deployment := componentccr.NewDefaultClusterChecksRunnerDeployment(dda)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	deployment.Spec.Template = *override.ApplyGlobalSettingsClusterChecksRunner(logger, podManagers, dda, resourcesManager, requiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range features {
		if errFeat := feat.ManageClusterChecksRunner(podManagers); errFeat != nil {
			return result, errFeat
		}
	}

	deploymentLogger := logger.WithValues("component", common.ClusterChecksRunnerReconcileConditionType)

	// The requiredComponents can change depending on if updates to features result in disabled components
	ccrEnabled := requiredComponents.ClusterChecksRunner.IsEnabled()
	dcaEnabled := requiredComponents.ClusterAgent.IsEnabled()

	// If the Cluster Agent is disabled, then CCR should be disabled too
	if dcaOverride, ok := dda.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName]; ok {
		if apiutils.BoolValue(dcaOverride.Disabled) {
			return r.cleanupV2ClusterChecksRunner(deploymentLogger, dda, deployment, newStatus)
		}
	} else if !dcaEnabled {
		return r.cleanupV2ClusterChecksRunner(deploymentLogger, dda, deployment, newStatus)
	}

	// If Override is defined for the CCR component, apply the override on the PodTemplateSpec, it will cascade to container.
	if componentOverride, ok := dda.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName]; ok {
		if apiutils.BoolValue(componentOverride.Disabled) {
			if ccrEnabled {
				// The override supersedes what's set in requiredComponents; update status to reflect the conflict
				condition.UpdateDatadogAgentStatusConditions(
					newStatus,
					metav1.NewTime(time.Now()),
					common.OverrideReconcileConflictConditionType,
					metav1.ConditionTrue,
					"OverrideConflict",
					"ClusterChecks component is set to disabled",
					true,
				)
			}
			// Delete CCR
			return r.cleanupV2ClusterChecksRunner(deploymentLogger, dda, deployment, newStatus)
		}
		override.PodTemplateSpec(logger, podManagers, componentOverride, datadoghqv2alpha1.ClusterChecksRunnerComponentName, dda.Name)
		override.Deployment(deployment, componentOverride)
	} else if !ccrEnabled {
		return r.cleanupV2ClusterChecksRunner(deploymentLogger, dda, deployment, newStatus)
	}

	return r.createOrUpdateDeployment(deploymentLogger, dda, deployment, newStatus, updateStatusV2WithClusterChecksRunner)
}

func updateStatusV2WithClusterChecksRunner(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.ClusterChecksRunner = condition.UpdateDeploymentStatus(deployment, newStatus.ClusterChecksRunner, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.ClusterChecksRunnerReconcileConditionType, status, reason, message, true)
}

func (r *Reconciler) cleanupV2ClusterChecksRunner(logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	// ClusterChecksRunnerDeployment attached to this instance
	ClusterChecksRunnerDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, ClusterChecksRunnerDeployment); err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	} else {
		logger.Info("Deleting Cluster Checks Runner Deployment", "deployment.Namespace", ClusterChecksRunnerDeployment.Namespace, "deployment.Name", ClusterChecksRunnerDeployment.Name)
		event := buildEventInfo(ClusterChecksRunnerDeployment.Name, ClusterChecksRunnerDeployment.Namespace, kubernetes.DeploymentKind, datadog.DeletionEvent)
		r.recordEvent(dda, event)
		if err := r.client.Delete(context.TODO(), ClusterChecksRunnerDeployment); err != nil {
			return reconcile.Result{}, err
		}
	}

	deleteStatusWithClusterChecksRunner(newStatus)

	return reconcile.Result{}, nil
}

func deleteStatusWithClusterChecksRunner(newStatus *datadoghqv2alpha1.DatadogAgentStatus) {
	newStatus.ClusterChecksRunner = nil
	condition.DeleteDatadogAgentStatusCondition(newStatus, common.ClusterChecksRunnerReconcileConditionType)
}

// cleanupOldCCRDeployments deletes CCR deployments when a CCR Deployment's name is changed using clusterChecksRunner name override
func (r *Reconciler) cleanupOldCCRDeployments(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus) error {
	matchLabels := client.MatchingLabels{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(dda).String(),
	}
	deploymentName := getDeploymentNameFromCCR(dda)
	deploymentList := appsv1.DeploymentList{}
	if err := r.client.List(ctx, &deploymentList, matchLabels); err != nil {
		return err
	}
	for _, deployment := range deploymentList.Items {
		if deploymentName != deployment.Name {
			if _, err := r.cleanupV2ClusterChecksRunner(logger, dda, &deployment, newStatus); err != nil {
				return err
			}
		}
	}

	return nil
}

// getDeploymentNameFromCCR returns the expected CCR deployment name based on
// the DDA name and clusterChecksRunner name override
func getDeploymentNameFromCCR(dda *datadoghqv2alpha1.DatadogAgent) string {
	deploymentName := componentccr.GetClusterChecksRunnerName(dda)
	if componentOverride, ok := dda.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName]; ok {
		if componentOverride.Name != nil && *componentOverride.Name != "" {
			deploymentName = *componentOverride.Name
		}
	}
	return deploymentName
}
