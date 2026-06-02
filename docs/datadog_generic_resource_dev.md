# How to contribute a new DatadogGenericResource

This document provides the steps to develop a new type for the `DatadogGenericResource` controller. It is recommended to go over [How to contribute][1] and [Overview of the DatadogGenericResource controller][2] to start with.

## Overview

The general steps to add a new type can be summarized to:
1. Add the new resource type to the CRD.
2. Add the related API client to the GenericResource client.
3. Create a stateful handler implementing the CRUD operations and register it in `buildHandlers`.

## Adding the new resource type

1. In `api/datadoghq/v1alpha1/datadoggenericresource_types.go`:
    1. Add a new constant of type `SupportedResourcesType`.
    2. Append its value to the `kubebuilder:validation:Enum` marker on the `Type` field in `DatadogGenericResourceSpec`.
2. Regenerate generated code and CRD manifests with `make generate && make manifests`.

## Adding the API client to `GenericClients`
1. Add the new client (from the [Datadog Go client](https://github.com/DataDog/datadog-api-client-go)) to the `GenericClients` struct inside `pkg/datadogclient/client.go`.
2. Reference it in `InitGenericClients` function within the same file.

## Adding the resource handler and CRUD operations

1. Create a new `internal/controller/datadoggenericresource/<your_resource>.go` file:
    1. Define a `<Resource>Handler` struct that holds your typed API client:
        ```go
        type <Resource>Handler struct {
            client *datadogV1.<Resource>Api  // or datadogV2
        }
        ```
    2. Implement the 5 methods of the `ResourceHandler` interface. Each method receives `auth` per-call and uses `h.client` directly:
        * `createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error)`: unmarshal `jsonSpec`, call the API Create method, return a `CreateResult{ID, CreatedTime, Creator}`.
        * `getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error`: call the API Get method using `instance.Status.Id`.
        * `updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error`: unmarshal `jsonSpec`, call the API Update method.
        * `deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error`: call the API Delete method using `instance.Status.Id`.
        * `refreshState(auth context.Context, instance *v1alpha1.DatadogGenericResource) (*string, error)`: fetch the live Datadog-side state of the resource and return it as a `*string`. **Return `(nil, nil)` for resource types that do not expose live state** — the controller will skip status mutation and leave the state fields and the `StateSynced` condition untouched. On error, return `(nil, err)` and the controller will preserve the last-known state and flip the `StateSynced` condition to False with `reason=GetError`. See `monitors.go` for a reference implementation and the other handlers (`dashboards.go`, `downtimes.go`, …) for the no-op pattern.
    3. Optionally define helper functions (`create<Resource>`, `get<Resource>`, etc.) for the raw API calls if it helps readability.
2. `internal/controller/datadoggenericresource/utils.go`: Add your handler to the `buildHandlers` function, mapping the new `SupportedResourcesType` to a `<Resource>Handler` initialized with the appropriate client from `GenericClients`:
    ```go
    v1alpha1.<Resource>: &<Resource>Handler{client: clients.<Resource>Client},
    ```

## Post-development tasks

1. Update the supported resources table in [Overview of the DatadogGenericResource controller][2].
2. Provide an example manifest with the new resource for the [examples][3] folder.
3. **If your `refreshState` implementation populates `status.State`** (i.e. your resource exposes a live Datadog-side state — like Monitors do, but unlike Dashboards/Notebooks):
    1. Update the `State` field godoc in `api/datadoghq/v1alpha1/datadoggenericresource_types.go` to enumerate the values your resource type may set (the current godoc lists Monitor's values; extend it rather than overwrite).
    2. Update the "Datadog-side status" table in [Overview of the DatadogGenericResource controller][2] so users know what `.status.state` values to expect for your resource type.
    3. Regenerate the CRD (`make generate && make manifests`) so the updated description propagates to the OpenAPI schema and the `config/crd/bases/` manifests.

[1]: ./how-to-contribute.md
[2]: ./datadog_generic_resource.md
[3]: ../examples/datadoggenericresource/
