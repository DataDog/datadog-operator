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
2. Generate the updated CRD with `make generate`.

## Adding the API client to `DatadogGenericClient`
1. Add the new client (from the [Datadog Go client](https://github.com/DataDog/datadog-api-client-go)) to the `DatadogGenericClient` struct inside `pkg/datadogclient/client.go`.
2. Reference it in `InitDatadogGenericClient` function within the same file.

## Adding the resource handler and CRUD operations

1. Create a new `internal/controller/datadoggenericresource/<your_resource>.go` file:
    1. Define a `<Resource>Handler` struct that holds the auth context and your typed API client:
        ```go
        type <Resource>Handler struct {
            auth   context.Context
            client *datadogV1.<Resource>Api  // or datadogV2
        }
        ```
    2. Implement the 4 methods of the `ResourceHandler` interface. Each method uses `h.auth` and `h.client` directly:
        * `createResource(instance *v1alpha1.DatadogGenericResource) (CreateResult, error)`: unmarshal `jsonSpec`, call the API Create method, return a `CreateResult{ID, CreatedTime, Creator}`.
        * `getResource(instance *v1alpha1.DatadogGenericResource) error`: call the API Get method using `instance.Status.Id`.
        * `updateResource(instance *v1alpha1.DatadogGenericResource) error`: unmarshal `jsonSpec`, call the API Update method.
        * `deleteResource(instance *v1alpha1.DatadogGenericResource) error`: call the API Delete method using `instance.Status.Id`.
    3. Optionally define helper functions (`create<Resource>`, `get<Resource>`, etc.) for the raw API calls if it helps readability.
2. `internal/controller/datadoggenericresource/utils.go`: Add your handler to the `buildHandlers` function, mapping the new `SupportedResourcesType` to a `<Resource>Handler` initialized with the appropriate client from `DatadogGenericClient`:
    ```go
    v1alpha1.<Resource>: &<Resource>Handler{auth: ddClient.Auth, client: ddClient.<Resource>Client},
    ```

## Post-development tasks

1. Update the supported resources table in [Overview of the DatadogGenericResource controller][2].
2. Provide an example manifest with the new resource for the [examples][3] folder.

[1]: ./how-to-contribute.md
[2]: ./datadog_generic_resource.md
[3]: ../examples/datadoggenericresource/
