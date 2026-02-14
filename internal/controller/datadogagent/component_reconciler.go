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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
)

// checkComponentEnabledWithOverride is a helper function that determines if a component is enabled
// based on both feature requirements and override settings. This is the default logic used by most components.
// Returns (enabled, conflict) where:
//   - enabled=true, conflict=false: component should be reconciled normally
//   - enabled=true, conflict=true: component enabled in features but disabled via override (cleanup with conflict status)
//   - enabled=false, conflict=true: component disabled in features but has override config (cleanup with conflict status)
//   - enabled=false, conflict=false: component disabled (cleanup without conflict status)
func checkComponentEnabledWithOverride(
	componentName datadoghqv2alpha1.ComponentName,
	componentRequired bool,
	overrides map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride,
) (enabled bool, conflict bool) {
	// Check if there's an override for this component
	if componentOverride, ok := overrides[componentName]; ok {
		// If override explicitly disables the component
		if apiutils.BoolValue(componentOverride.Disabled) {
			// Conflict: component is enabled in features but disabled via override
			if componentRequired {
				return false, true
			}
			// No conflict: both features and override disable it
			return false, false
		}
		// Override exists with configuration but doesn't disable
		// If component is disabled in features, this is a conflict (user trying to configure disabled component)
		if !componentRequired {
			return false, true
		}
	}

	// No override or override doesn't disable: use feature setting
	return componentRequired, false
}

// ComponentReconciler defines the interface that all deployment/daemonset components must implement
type ComponentReconciler interface {
	// Name returns the component name (e.g., "clusterAgent", "clusterChecksRunner")
	Name() datadoghqv2alpha1.ComponentName

	// IsEnabled checks if this component should be reconciled based on requiredComponents and override settings
	// Returns (enabled, conflict) where:
	//   - enabled=true, conflict=false: component should be reconciled normally
	//   - enabled=true, conflict=true: component enabled in features but disabled via override (cleanup with conflict status)
	//   - enabled=false, conflict=*: component disabled (cleanup without conflict status if conflict=false)
	IsEnabled(requiredComponents feature.RequiredComponents, overrides map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride) (enabled bool, conflict bool)

	// GetConditionType returns the condition type used for status updates
	GetConditionType() string

	// GetGlobalSettingsFunc returns the function to apply global settings to the component
	GetGlobalSettingsFunc() func(logger logr.Logger, podManagers feature.PodTemplateManagers, dda metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec, resourceManagers feature.ResourceManagers, requiredComponents feature.RequiredComponents)

	// GetNewDeploymentFunc returns the function to create a new deployment for the component
	GetNewDeploymentFunc() func(dda metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec) *appsv1.Deployment

	// GetManageFeatureFunc feature function to manage the component
	GetManageFeatureFunc() func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error

	// UpdateStatus updates the status of the component
	UpdateStatus(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string)

	// DeleteStatus deletes the status of the component
	DeleteStatus(newStatus *datadoghqv2alpha1.DatadogAgentStatus, conditionType string)

	// ForceDeleteComponent forces the deletion of the component
	ForceDeleteComponent(dda *datadoghqv2alpha1.DatadogAgent, componentName datadoghqv2alpha1.ComponentName, requiredComponents feature.RequiredComponents) bool

	// CleanupDependencies deletes any dependencies associated with the component
	CleanupDependencies(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers) (reconcile.Result, error)
}

// ReconcileComponentParams bundles common parameters needed by all components
type ReconcileComponentParams struct {
	Logger             logr.Logger
	DDA                *datadoghqv2alpha1.DatadogAgent
	RequiredComponents feature.RequiredComponents
	Features           []feature.Feature
	ResourceManagers   feature.ResourceManagers
	Status             *datadoghqv2alpha1.DatadogAgentStatus
	Provider           string
	ProviderList       map[string]struct{}
}

// ComponentRegistry manages the registration and reconciliation of all components
type ComponentRegistry struct {
	components []ComponentReconciler
	reconciler *Reconciler
}

// NewComponentRegistry creates a new component registry
func NewComponentRegistry(reconciler *Reconciler) *ComponentRegistry {
	return &ComponentRegistry{
		components: make([]ComponentReconciler, 0),
		reconciler: reconciler,
	}
}

// Register adds a component to the registry
func (r *ComponentRegistry) Register(component ComponentReconciler) {
	r.components = append(r.components, component)
}

// ReconcileComponents reconciles all registered components in order
func (r *ComponentRegistry) ReconcileComponents(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	var result reconcile.Result

	for _, comp := range r.components {
		// Check if component is enabled and if there's a conflict
		enabled, conflict := comp.IsEnabled(params.RequiredComponents, params.DDA.Spec.Override)

		var res reconcile.Result
		var err error

		if !enabled {
			// Component is disabled, clean it up
			if conflict {
				// Set conflict status condition
				condition.UpdateDatadogAgentStatusConditions(
					params.Status,
					metav1.NewTime(time.Now()),
					common.OverrideReconcileConflictConditionType,
					metav1.ConditionTrue,
					"OverrideConflict",
					fmt.Sprintf("%s component is set to disabled", comp.Name()),
					true,
				)
			}
			res, err = r.Cleanup(ctx, params, comp)
		} else {
			// Component is enabled, reconcile it
			res, err = r.reconcileComponent(ctx, params, comp)
		}

		if utils.ShouldReturn(res, err) {
			return res, err
		}

		// Merge result (preserve requeue settings)
		if res.Requeue || res.RequeueAfter > 0 {
			result = res
		}
	}

	return result, nil
}

// Reconcile reconciles component
func (r *ComponentRegistry) reconcileComponent(ctx context.Context, params *ReconcileComponentParams, component ComponentReconciler) (reconcile.Result, error) {
	var result reconcile.Result
	now := metav1.NewTime(time.Now())
	deploymentLogger := params.Logger.WithValues("component", component.Name())

	// Start by creating the Default Cluster-Agent deployment
	deployment := component.GetNewDeploymentFunc()(params.DDA.GetObjectMeta(), &params.DDA.Spec)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	component.GetGlobalSettingsFunc()(deploymentLogger, podManagers, params.DDA.GetObjectMeta(), &params.DDA.Spec, params.ResourceManagers, params.RequiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	var featErrors []error
	for _, feat := range params.Features {
		if errFeat := component.GetManageFeatureFunc()(feat, podManagers, params.Provider); errFeat != nil {
			featErrors = append(featErrors, errFeat)
		}
	}
	if len(featErrors) > 0 {
		err := utilerrors.NewAggregate(featErrors)
		component.UpdateStatus(deployment, params.Status, now, metav1.ConditionFalse, fmt.Sprintf("%s feature error", component.Name()), err.Error())
		return result, err
	}

	// Check for force delete (e.g., CCR when ClusterAgent is disabled)
	if component.ForceDeleteComponent(params.DDA, component.Name(), params.RequiredComponents) {
		return r.Cleanup(ctx, params, component)
	}

	// Apply override if defined for the component
	if componentOverride, ok := params.DDA.Spec.Override[component.Name()]; ok {
		override.PodTemplateSpec(deploymentLogger, podManagers, componentOverride, component.Name(), params.DDA.Name)
		override.Deployment(deployment, componentOverride)
	}

	if r.reconciler.options.IntrospectionEnabled {
		// Add provider label to deployment
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		deployment.Labels[constants.MD5AgentDeploymentProviderLabelKey] = params.Provider
	}

	res, err := r.reconciler.createOrUpdateDeployment(deploymentLogger, params.DDA, deployment, params.Status, component.UpdateStatus)

	if err == nil {
		// Update condition to success since the deployment was created or updated successfully
		condition.UpdateDatadogAgentStatusConditions(
			params.Status,
			now,
			component.GetConditionType(),
			metav1.ConditionTrue,
			"reconcile_succeed",
			"reconcile succeed",
			false,
		)
	}

	return res, err
}

// Cleanup removes the component deployment, associated resources and updates status
func (r *ComponentRegistry) Cleanup(ctx context.Context, params *ReconcileComponentParams, component ComponentReconciler) (reconcile.Result, error) {
	deployment := component.GetNewDeploymentFunc()(params.DDA.GetObjectMeta(), &params.DDA.Spec)
	result, err := r.reconciler.deleteDeploymentWithEvent(ctx, params.Logger, params.DDA, deployment)

	if err != nil {
		return result, err
	}

	// Do status and other resource cleanup if the deployment was deleted successfully
	if result, err = component.CleanupDependencies(ctx, params.Logger, params.DDA, params.ResourceManagers); err != nil {
		return result, err
	}
	component.DeleteStatus(params.Status, component.GetConditionType())

	return result, nil
}

// GetComponent returns a registered component by name
func (r *ComponentRegistry) GetComponent(name datadoghqv2alpha1.ComponentName) ComponentReconciler {
	for _, comp := range r.components {
		if comp.Name() == name {
			return comp
		}
	}
	return nil
}

// ListComponents returns all registered components
func (r *ComponentRegistry) ListComponents() []ComponentReconciler {
	return r.components
}
