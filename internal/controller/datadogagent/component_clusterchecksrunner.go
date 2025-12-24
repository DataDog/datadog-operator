// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
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

func (c *ClusterChecksRunnerComponent) GetGlobalSettingsFunc() func(logger logr.Logger, podManagers feature.PodTemplateManagers, dda metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec, resourceManagers feature.ResourceManagers, requiredComponents feature.RequiredComponents) {
	return global.ApplyGlobalSettingsClusterChecksRunner
}

func (c *ClusterChecksRunnerComponent) GetNewDeploymentFunc() func(dda metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec) *appsv1.Deployment {
	return componentccr.NewDefaultClusterChecksRunnerDeployment
}

func (c *ClusterChecksRunnerComponent) GetManageFeatureFunc() func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
	return func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
		return feat.ManageClusterChecksRunner(managers, provider)
	}
}

// Reconcile reconciles component
func (c *ClusterChecksRunnerComponent) Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	var result reconcile.Result
	now := metav1.NewTime(time.Now())
	deploymentLogger := params.Logger.WithValues("component", c.Name())

	// Start by creating the Default Cluster-Agent deployment
	deployment := c.GetNewDeploymentFunc()(params.DDA.GetObjectMeta(), &params.DDA.Spec)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	c.GetGlobalSettingsFunc()(params.Logger, podManagers, params.DDA.GetObjectMeta(), &params.DDA.Spec, params.ResourceManagers, params.RequiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	var featErrors []error
	for _, feat := range params.Features {
		if errFeat := c.GetManageFeatureFunc()(feat, podManagers, params.Provider); errFeat != nil {
			featErrors = append(featErrors, errFeat)
		}
	}
	if len(featErrors) > 0 {
		err := utilerrors.NewAggregate(featErrors)
		c.UpdateStatus(deployment, params.Status, now, metav1.ConditionFalse, fmt.Sprintf("%s feature error", c.Name()), err.Error())
		return result, err
	}

	// The requiredComponents can change depending on if updates to features result in disabled components
	componentEnabled := c.IsEnabled(params.RequiredComponents)

	if c.ForceDeleteComponent(params.DDA, c.Name(), params.RequiredComponents) {
		return c.Cleanup(ctx, params)
	}

	// If Override is defined for the component, apply the override on the PodTemplateSpec, it will cascade to container.
	if componentOverride, ok := params.DDA.Spec.Override[c.Name()]; ok {
		if apiutils.BoolValue(componentOverride.Disabled) {
			if componentEnabled {
				// The override supersedes what's set in requiredComponents; update status to reflect the conflict
				condition.UpdateDatadogAgentStatusConditions(
					params.Status,
					metav1.NewTime(time.Now()),
					common.OverrideReconcileConflictConditionType,
					metav1.ConditionTrue,
					"OverrideConflict",
					fmt.Sprintf("%s component is set to disabled", c.Name()),
					true,
				)
			}
			return c.Cleanup(ctx, params)
		}
		override.PodTemplateSpec(params.Logger, podManagers, componentOverride, c.Name(), params.DDA.Name)
		override.Deployment(deployment, componentOverride)
	} else if !componentEnabled {
		return c.Cleanup(ctx, params)
	}

	if c.reconciler.options.IntrospectionEnabled {
		// Add provider label to deployment
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		deployment.Labels[constants.MD5AgentDeploymentProviderLabelKey] = params.Provider
	}

	return c.reconciler.createOrUpdateDeployment(deploymentLogger, params.DDA, deployment, params.Status, c.UpdateStatus)
}

// Cleanup removes the component deployment, associated resources and updates status
func (c *ClusterChecksRunnerComponent) Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	deployment := c.GetNewDeploymentFunc()(params.DDA.GetObjectMeta(), &params.DDA.Spec)
	result, err := c.reconciler.deleteDeploymentWithEvent(ctx, params.Logger, params.DDA, deployment)

	if err != nil {
		return result, err
	}

	// Do status and other resource cleanup if the deployment was deleted successfully
	if result, err := c.CleanupDependencies(ctx, params.Logger, params.DDA, params.ResourceManagers); err != nil {
		return result, err
	}
	c.DeleteStatus(params.Status, c.GetConditionType())

	return result, nil
}

func (c *ClusterChecksRunnerComponent) UpdateStatus(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.ClusterChecksRunner = condition.UpdateDeploymentStatus(deployment, newStatus.ClusterChecksRunner, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.ClusterChecksRunnerReconcileConditionType, status, reason, message, true)
}

func (c *ClusterChecksRunnerComponent) DeleteStatus(newStatus *datadoghqv2alpha1.DatadogAgentStatus, conditionType string) {
	newStatus.ClusterChecksRunner = nil
	condition.DeleteDatadogAgentStatusCondition(newStatus, conditionType)
}

func (c *ClusterChecksRunnerComponent) ForceDeleteComponent(dda *datadoghqv2alpha1.DatadogAgent, componentName datadoghqv2alpha1.ComponentName, requiredComponents feature.RequiredComponents) bool {
	dcaEnabled := requiredComponents.ClusterAgent.IsEnabled()
	// If the Cluster Agent is disabled, then CCR should be disabled too
	if dcaOverride, ok := dda.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName]; ok {
		if apiutils.BoolValue(dcaOverride.Disabled) {
			return true
		}
	} else if !dcaEnabled {
		return true
	}
	return false
}

func (c *ClusterChecksRunnerComponent) CleanupDependencies(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
