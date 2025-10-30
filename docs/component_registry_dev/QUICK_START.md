# Quick Start: Adding a New Component

This is a quick reference for adding a new deployment or daemonset component to the DatadogAgent controller.

## TL;DR

1. Copy `component_example.go.tmpl` to `component_<name>.go`
2. Fill in the TODOs
3. Add one line to `controller.go`: `r.componentRegistry.Register(NewYourComponent(r))`
4. Done! 🎉

## Step-by-Step Guide

### 1. Create Your Component File

```bash
cp component_example.go.tmpl component_myservice.go
```

Edit the file and replace `Example` with `MyService` throughout.

### 2. Implement Required Methods

```go
type MyServiceComponent struct {
    reconciler *Reconciler
}

func NewMyServiceComponent(reconciler *Reconciler) *MyServiceComponent {
    return &MyServiceComponent{reconciler: reconciler}
}

func (c *MyServiceComponent) Name() datadoghqv2alpha1.ComponentName {
    return datadoghqv2alpha1.MyServiceComponentName  // Define in API types
}

func (c *MyServiceComponent) IsEnabled(requiredComponents feature.RequiredComponents) bool {
    return requiredComponents.MyService.IsEnabled()  // Add to RequiredComponents
}

func (c *MyServiceComponent) GetConditionType() string {
    return common.MyServiceReconcileConditionType  // Define in common/const.go
}

func (c *MyServiceComponent) Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
    // Your reconciliation logic here
}

func (c *MyServiceComponent) Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
    // Your cleanup logic here
}
```

### 3. Register the Component

In `controller.go`, add to `initializeComponentRegistry()`:

```go
func (r *Reconciler) initializeComponentRegistry() {
    r.componentRegistry = NewComponentRegistry(r)

    r.componentRegistry.Register(NewClusterAgentComponent(r))
    r.componentRegistry.Register(NewClusterChecksRunnerComponent(r))
    r.componentRegistry.Register(NewMyServiceComponent(r))  // <-- Add this
}
```

### 4. Test

```bash
make test
make build
```
