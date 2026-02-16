// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/condition"
)

// BaseComponent provides default implementations for common component functionality
type BaseComponent struct {
	name              datadoghqv2alpha1.ComponentName
	conditionType     string
	globalSettings    func(logr.Logger, feature.PodTemplateManagers, metav1.Object, *datadoghqv2alpha1.DatadogAgentSpec, feature.ResourceManagers, feature.RequiredComponents)
	newDeployment     func(metav1.Object, *datadoghqv2alpha1.DatadogAgentSpec) *appsv1.Deployment
	manageFeature     func(feature.Feature, feature.PodTemplateManagers, string) error
	statusFieldGetter func(*datadoghqv2alpha1.DatadogAgentStatus) **datadoghqv2alpha1.DeploymentStatus
}

// ComponentBuilder helps construct components with common functionality
type ComponentBuilder struct {
	name              datadoghqv2alpha1.ComponentName
	conditionType     string
	globalSettings    func(logr.Logger, feature.PodTemplateManagers, metav1.Object, *datadoghqv2alpha1.DatadogAgentSpec, feature.ResourceManagers, feature.RequiredComponents)
	newDeployment     func(metav1.Object, *datadoghqv2alpha1.DatadogAgentSpec) *appsv1.Deployment
	manageFeature     func(feature.Feature, feature.PodTemplateManagers, string) error
	statusFieldGetter func(*datadoghqv2alpha1.DatadogAgentStatus) **datadoghqv2alpha1.DeploymentStatus
	requiredGetter    func(feature.RequiredComponents) *feature.RequiredComponent
}

// NewComponentBuilder creates a new component builder
func NewComponentBuilder(name datadoghqv2alpha1.ComponentName) *ComponentBuilder {
	return &ComponentBuilder{
		name: name,
	}
}

// WithConditionType sets the condition type for status updates
func (b *ComponentBuilder) WithConditionType(conditionType string) *ComponentBuilder {
	b.conditionType = conditionType
	return b
}

// WithGlobalSettings sets the global settings function
func (b *ComponentBuilder) WithGlobalSettings(fn func(logr.Logger, feature.PodTemplateManagers, metav1.Object, *datadoghqv2alpha1.DatadogAgentSpec, feature.ResourceManagers, feature.RequiredComponents)) *ComponentBuilder {
	b.globalSettings = fn
	return b
}

// WithNewDeployment sets the deployment creation function
func (b *ComponentBuilder) WithNewDeployment(fn func(metav1.Object, *datadoghqv2alpha1.DatadogAgentSpec) *appsv1.Deployment) *ComponentBuilder {
	b.newDeployment = fn
	return b
}

// WithManageFeature sets the feature management function
func (b *ComponentBuilder) WithManageFeature(fn func(feature.Feature, feature.PodTemplateManagers, string) error) *ComponentBuilder {
	b.manageFeature = fn
	return b
}

// WithStatusField sets the status field getter
func (b *ComponentBuilder) WithStatusField(getter func(*datadoghqv2alpha1.DatadogAgentStatus) **datadoghqv2alpha1.DeploymentStatus) *ComponentBuilder {
	b.statusFieldGetter = getter
	return b
}

// WithRequiredComponent sets the required component getter
func (b *ComponentBuilder) WithRequiredComponent(getter func(feature.RequiredComponents) *feature.RequiredComponent) *ComponentBuilder {
	b.requiredGetter = getter
	return b
}

// Build creates the BaseComponent
func (b *ComponentBuilder) Build() BaseComponent {
	return BaseComponent{
		name:              b.name,
		conditionType:     b.conditionType,
		globalSettings:    b.globalSettings,
		newDeployment:     b.newDeployment,
		manageFeature:     b.manageFeature,
		statusFieldGetter: b.statusFieldGetter,
	}
}

// Name returns the component name
func (b *BaseComponent) Name() datadoghqv2alpha1.ComponentName {
	return b.name
}

// GetConditionType returns the condition type for status updates
func (b *BaseComponent) GetConditionType() string {
	return b.conditionType
}

// GetGlobalSettingsFunc returns the global settings function
func (b *BaseComponent) GetGlobalSettingsFunc() func(logr.Logger, feature.PodTemplateManagers, metav1.Object, *datadoghqv2alpha1.DatadogAgentSpec, feature.ResourceManagers, feature.RequiredComponents) {
	return b.globalSettings
}

// GetNewDeploymentFunc returns the deployment creation function
func (b *BaseComponent) GetNewDeploymentFunc() func(metav1.Object, *datadoghqv2alpha1.DatadogAgentSpec) *appsv1.Deployment {
	return b.newDeployment
}

// GetManageFeatureFunc returns the feature management function
func (b *BaseComponent) GetManageFeatureFunc() func(feature.Feature, feature.PodTemplateManagers, string) error {
	return b.manageFeature
}

// UpdateStatus updates the component status with default implementation
func (b *BaseComponent) UpdateStatus(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	if b.statusFieldGetter != nil {
		statusField := b.statusFieldGetter(newStatus)
		*statusField = condition.UpdateDeploymentStatus(deployment, *statusField, &updateTime)
	}
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, b.conditionType, status, reason, message, true)
}

// DeleteStatus deletes the component status with default implementation
func (b *BaseComponent) DeleteStatus(newStatus *datadoghqv2alpha1.DatadogAgentStatus, conditionType string) {
	if b.statusFieldGetter != nil {
		statusField := b.statusFieldGetter(newStatus)
		*statusField = nil
	}
	condition.DeleteDatadogAgentStatusCondition(newStatus, conditionType)
}

// ForceDeleteComponent returns false by default (most components don't force delete)
func (b *BaseComponent) ForceDeleteComponent(dda *datadoghqv2alpha1.DatadogAgent, componentName datadoghqv2alpha1.ComponentName, requiredComponents feature.RequiredComponents) bool {
	return false
}

// CleanupDependencies returns no-op by default (most components have no special cleanup)
func (b *BaseComponent) CleanupDependencies(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}