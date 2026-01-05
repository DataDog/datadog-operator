// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/componentmeta"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
)

// ComponentReconciler defines the interface that all deployment/daemonset components must implement
type ComponentReconciler interface {
	// Name returns the component name (e.g., "clusterAgent", "clusterChecksRunner")
	Name() datadoghqv2alpha1.ComponentName

	// Meta returns the component metadata
	Meta() componentmeta.ComponentMeta

	// IsEnabled checks if this component should be reconciled based on requiredComponents
	IsEnabled(requiredComponents feature.RequiredComponents) bool

	// Reconcile handles the reconciliation logic for this component
	Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error)

	// Cleanup removes resources when component is disabled
	Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error)

	// GetConditionType returns the condition type used for status updates
	GetConditionType() string
}

// ReconcileComponentParams bundles common parameters needed by all components
type ReconcileComponentParams struct {
	Logger             logr.Logger
	DDAI               *v1alpha1.DatadogAgentInternal
	RequiredComponents feature.RequiredComponents
	Features           []feature.Feature
	ResourceManagers   feature.ResourceManagers
	Status             *v1alpha1.DatadogAgentInternalStatus
	Provider           string
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
	now := metav1.NewTime(time.Now())

	for _, comp := range r.components {
		componentLogger := params.Logger.WithValues("component", comp.Name())
		compParams := *params
		compParams.Logger = componentLogger

		// Check if component is enabled based on required components
		enabled := comp.IsEnabled(params.RequiredComponents)

		// Check if component is explicitly disabled via override
		explicitlyDisabled := false
		if override, ok := params.DDAI.Spec.Override[comp.Name()]; ok {
			if override.Disabled != nil && *override.Disabled {
				explicitlyDisabled = true
			}
		}

		// If not enabled or explicitly disabled, cleanup and continue
		if !enabled || explicitlyDisabled {
			if enabled && explicitlyDisabled {
				// The override supersedes what's set in requiredComponents; update status to reflect the conflict
				condition.UpdateDatadogAgentInternalStatusConditions(
					params.Status,
					now,
					common.OverrideReconcileConflictConditionType,
					metav1.ConditionTrue,
					"OverrideConflict",
					string(comp.Name())+" component is set to disabled",
					true,
				)
			}

			componentLogger.V(1).Info("Component disabled, cleaning up")
			res, err := comp.Cleanup(ctx, &compParams)
			if utils.ShouldReturn(res, err) {
				return res, err
			}
			continue
		}

		// Reconcile the component
		componentLogger.V(1).Info("Reconciling component")
		res, err := comp.Reconcile(ctx, &compParams)
		if utils.ShouldReturn(res, err) {
			return res, err
		}

		// Update condition on success
		condition.UpdateDatadogAgentInternalStatusConditions(
			params.Status,
			now,
			comp.GetConditionType(),
			metav1.ConditionTrue,
			"reconcile_succeed",
			"reconcile succeed",
			false,
		)

		// Merge result (preserve requeue settings)
		if res.Requeue || res.RequeueAfter > 0 {
			result = res
		}
	}

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
