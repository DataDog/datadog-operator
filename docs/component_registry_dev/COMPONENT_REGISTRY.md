# Component Registry Pattern

This document describes the component registry pattern used for managing DatadogAgent deployments and daemonsets.

## Overview

The component registry pattern allows you to add new deployments or daemonsets to the DatadogAgent controller without modifying the main reconciliation loop. Each component implements a standard interface and is automatically reconciled by the registry.

## Architecture

### Key Files

- `component_reconciler.go` - Defines the `ComponentReconciler` interface and `ComponentRegistry`
- `component_clusteragent.go` - Example implementation for Cluster Agent
- `component_clusterchecksrunner.go` - Example implementation for Cluster Checks Runner
- `controller.go` - Reconciler initialization and component registration

### Component Interface

All components must implement the `ComponentReconciler` interface:

```go
type ComponentReconciler interface {
    // Name returns the component name (e.g., "cluster-agent", "cluster-checks-runner")
    Name() datadoghqv2alpha1.ComponentName

    // IsEnabled checks if this component should be reconciled based on requiredComponents
    IsEnabled(requiredComponents feature.RequiredComponents) bool

    // Reconcile handles the reconciliation logic for this component
    Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error)

    // Cleanup removes resources when component is disabled
    Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error)

    // GetConditionType returns the condition type used for status updates
    GetConditionType() string
}
```

## Adding a New Component

Follow these steps to add a new deployment or daemonset component:

### Step 1: Create Component File

Create a new file `component_<name>.go` (you can copy over `component_example.go.tmpl`) with your component implementation:

```go
package datadogagent

import (
    "context"
    // ... other imports

    datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
    "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MyNewComponent implements ComponentReconciler for my new deployment
type MyNewComponent struct {
    reconciler *Reconciler
}

// NewMyNewComponent creates a new instance
func NewMyNewComponent(reconciler *Reconciler) *MyNewComponent {
    return &MyNewComponent{
        reconciler: reconciler,
    }
}

// Name returns the component name
func (c *MyNewComponent) Name() datadoghqv2alpha1.ComponentName {
    return datadoghqv2alpha1.MyNewComponentName
}

// IsEnabled checks if the component should be reconciled
func (c *MyNewComponent) IsEnabled(requiredComponents feature.RequiredComponents) bool {
    return requiredComponents.MyNewComponent.IsEnabled()
}

// GetConditionType returns the condition type for status updates
func (c *MyNewComponent) GetConditionType() string {
    return common.MyNewComponentReconcileConditionType
}

// Reconcile reconciles the component
func (c *MyNewComponent) Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
    var result reconcile.Result

    // 1. Create default deployment/daemonset
    deployment := componentmynew.NewDefaultMyNewDeployment(params.DDA)
    podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

    // 2. Apply global settings
    global.ApplyGlobalSettingsMyNew(params.Logger, podManagers, params.DDA.GetObjectMeta(),
        &params.DDA.Spec, params.ResourceManagers, params.RequiredComponents)

    // 3. Apply features
    for _, feat := range params.Features {
        if errFeat := feat.ManageMyNewComponent(podManagers, params.Provider); errFeat != nil {
            return result, errFeat
        }
    }

    // 4. Apply overrides if defined
    if componentOverride, ok := params.DDA.Spec.Override[c.Name()]; ok {
        if apiutils.BoolValue(componentOverride.Disabled) {
            return c.Cleanup(ctx, params)
        }
        override.PodTemplateSpec(params.Logger, podManagers, componentOverride, c.Name(), params.DDA.Name)
        override.Deployment(deployment, componentOverride)
    }

    // 5. Create or update the deployment
    return c.reconciler.createOrUpdateDeployment(params.Logger, params.DDA, deployment,
        params.Status, updateStatusV2WithMyNew)
}

// Cleanup removes the component's resources
func (c *MyNewComponent) Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
    deployment := componentmynew.NewDefaultMyNewDeployment(params.DDA)
    return c.reconciler.cleanupV2MyNew(params.Logger, params.DDA, deployment, params.Status)
}

// Helper functions for status updates
func updateStatusV2WithMyNew(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus,
    updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
    newStatus.MyNewComponent = condition.UpdateDeploymentStatus(deployment, newStatus.MyNewComponent, &updateTime)
    condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime,
        common.MyNewComponentReconcileConditionType, status, reason, message, true)
}

func (r *Reconciler) cleanupV2MyNew(logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent,
    deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
    // Cleanup logic here
    // Delete deployment, RBACs, etc.
    return reconcile.Result{}, nil
}
```

### Step 2: Register the Component

In `controller.go`, add your component to the `initializeComponentRegistry()` function:

```go
func (r *Reconciler) initializeComponentRegistry() {
    r.componentRegistry = NewComponentRegistry(r)

    // Register all components
    r.componentRegistry.Register(NewClusterAgentComponent(r))
    r.componentRegistry.Register(NewClusterChecksRunnerComponent(r))
    r.componentRegistry.Register(NewMyNewComponent(r))  // <-- Add this line
}
```

That's it! Your component will now be automatically reconciled in the correct order.
N.B.: make sure to disable the newly added component in `setProfileSpec` (`internal/controller/datadogagent/profile.go`) so it's only reconciled once as part of the default DatadogAgentInternal.

## Component Lifecycle

The registry handles the full lifecycle of each component:

1. **Check if enabled**: Calls `IsEnabled()` to determine if the component should be reconciled
2. **Check overrides**: Examines `Spec.Override` to see if the component is explicitly disabled
3. **Reconcile or Cleanup**: Either calls `Reconcile()` or `Cleanup()` based on the enabled state
4. **Update status**: Automatically updates the status condition on success using `GetConditionType()`

## Features of the Pattern

### Automatic Handling

- **Override conflicts**: The registry detects when a component is required by features but disabled by override
- **Status updates**: Success conditions are automatically set after reconciliation
- **Error handling**: Errors are propagated correctly and reconciliation stops on first error
- **Logging**: Each component gets its own logger with the component name

### Scalability

- Adding a new component requires only:
  1. One new file implementing `ComponentReconciler`
  2. One line in `initializeComponentRegistry()`
- No changes to the main reconciliation loop
- No changes to other components

### Testability

- Each component can be unit tested independently
- Mock the `ReconcileComponentParams` for isolated testing
- Test the registry separately from individual components

## Example: Cluster Agent Component

See `component_clusteragent.go` for a complete example of a component implementation.

Key points:
- The component creates a default deployment
- Applies global settings and features
- Handles overrides
- Provides cleanup logic
- Updates status appropriately

## Common Patterns

### Dependencies Between Components

If your component depends on another (like CCR depends on Cluster Agent):

```go
func (c *MyNewComponent) Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
    // Check if dependency is enabled
    if !params.RequiredComponents.ClusterAgent.IsEnabled() {
        return c.Cleanup(ctx, params)
    }

    // Check if dependency is disabled via override
    if dcaOverride, ok := params.DDA.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName]; ok {
        if apiutils.BoolValue(dcaOverride.Disabled) {
            return c.Cleanup(ctx, params)
        }
    }

    // Continue with normal reconciliation...
}
```

### Provider-Specific Logic

Use `params.Provider` and `params.ProviderList` for provider-specific behavior:

```go
if c.reconciler.options.IntrospectionEnabled {
    if deployment.Labels == nil {
        deployment.Labels = make(map[string]string)
    }
    deployment.Labels[constants.MD5AgentDeploymentProviderLabelKey] = params.Provider
}
```
