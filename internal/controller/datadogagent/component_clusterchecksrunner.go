// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
)

// ClusterChecksRunnerComponent implements ComponentReconciler for the Cluster Checks Runner deployment
type ClusterChecksRunnerComponent struct {
	reconciler *Reconciler
}

// NewClusterChecksRunnerComponent creates a new ClusterChecksRunner component
func NewClusterChecksRunnerComponent(reconciler *Reconciler) *ClusterChecksRunnerComponent {
	return &ClusterChecksRunnerComponent{
		reconciler: reconciler,
	}
}

// Name returns the component name
func (c *ClusterChecksRunnerComponent) Name() datadoghqv2alpha1.ComponentName {
	return datadoghqv2alpha1.ClusterChecksRunnerComponentName
}

// IsEnabled checks if the ClusterChecksRunner component should be reconciled
// CCR requires the Cluster Agent to be enabled as well
func (c *ClusterChecksRunnerComponent) IsEnabled(requiredComponents feature.RequiredComponents) bool {
	return requiredComponents.ClusterChecksRunner.IsEnabled()
}

// GetConditionType returns the condition type for status updates
func (c *ClusterChecksRunnerComponent) GetConditionType() string {
	return common.ClusterChecksRunnerReconcileConditionType
}

// Reconcile reconciles the ClusterChecksRunner component
func (c *ClusterChecksRunnerComponent) Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	var result reconcile.Result

	// Start by creating the Default Cluster Checks Runner deployment
	deployment := componentccr.NewDefaultClusterChecksRunnerDeployment(params.DDA)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	global.ApplyGlobalSettingsClusterChecksRunner(params.Logger, podManagers, params.DDA.GetObjectMeta(), &params.DDA.Spec, params.ResourceManagers, params.RequiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	for _, feat := range params.Features {
		if errFeat := feat.ManageClusterChecksRunner(podManagers, ""); errFeat != nil {
			return result, errFeat
		}
	}

	// CCR requires the Cluster Agent to be enabled
	dcaEnabled := params.RequiredComponents.ClusterAgent.IsEnabled()

	// If the Cluster Agent is disabled via override, then CCR should be disabled too
	if dcaOverride, ok := params.DDA.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName]; ok {
		if apiutils.BoolValue(dcaOverride.Disabled) {
			return c.Cleanup(ctx, params)
		}
	} else if !dcaEnabled {
		return c.Cleanup(ctx, params)
	}

	// If Override is defined for the CCR component, apply the override on the PodTemplateSpec
	if componentOverride, ok := params.DDA.Spec.Override[c.Name()]; ok {
		if apiutils.BoolValue(componentOverride.Disabled) {
			// This case is handled by the registry, but we double-check here
			return c.Cleanup(ctx, params)
		}
		override.PodTemplateSpec(params.Logger, podManagers, componentOverride, c.Name(), params.DDA.Name)
		override.Deployment(deployment, componentOverride)
	}

	return c.reconciler.createOrUpdateDeployment(params.Logger, params.DDA, deployment, params.Status, updateStatusV2WithClusterChecksRunner)
}

// Cleanup removes the ClusterChecksRunner deployment
func (c *ClusterChecksRunnerComponent) Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	deployment := componentccr.NewDefaultClusterChecksRunnerDeployment(params.DDA)
	return c.reconciler.cleanupV2ClusterChecksRunner(params.Logger, params.DDA, deployment, params.Status)
}

// The following functions are kept for backward compatibility with existing code

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

func updateStatusV2WithClusterChecksRunner(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.ClusterChecksRunner = condition.UpdateDeploymentStatus(deployment, newStatus.ClusterChecksRunner, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.ClusterChecksRunnerReconcileConditionType, status, reason, message, true)
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
