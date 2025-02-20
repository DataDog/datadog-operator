# How to contribute a new DatadogGenericResource

This document provides the steps to develop a new type for the `DatadogGenericResource` controller. It is recommended to go over [How to contribute][1] and [Overview of the DatadogGenericResource controller][2] to start with.

## Overview

The general steps to add a new type can be summarized to:
1. Add the new resource type to the CRD.
2. Add the related API client to the GenericResource client.
3. Add the CRUD operations for the new resource type within the GenericResource controller and reference them.

Example pull request: https://github.com/DataDog/datadog-operator/pull/1635.

## Adding the new resource type

1. The CRD is defined inside `api/datadoghq/v1alpha1/datadoggenericresource_types.go`: add a new constant of type `SupportedResourcesType` and reference its value in the `kubebuilder:validation:Enum` marker inside the `DatadogGenericResourceSpec` struct.
2. Add this constant to the validation map `allowedCustomResourcesEnumMap` (`api/datadoghq/v1alpha1/datadoggenericresource_validation.go`).
3. Generate the updated CRD with `make generate`.

## Adding the API client to `DatadogGenericClient`
1. Add the new client (from the [Datadog Go client](https://github.com/DataDog/datadog-api-client-go)) to the `DatadogGenericClient` struct inside `pkg/datadogclient/client.go`.
2. Reference it in `InitDatadogGenericClient` function within the same file.

## Adding the resource-CRUD operations and referencing them

1. `internal/controller/datadoggenericresource/controller.go`:
    1. Add the new API client to the `Reconciler` struct.
	2. Reference it in the `NewReconciler` function.
2. Create a new `internal/controller/datadoggenericresource/<your_resource>.go` file:
    1. Define an empty `<Resource>Handler` struct for the `ResourceHandler` interface.
	2. Define the 4 operations functions:
	    * create<Resource>: unmarshal the `jsonSpec` from the `DatadogGenericResource` instance into the struct expected by the API client, call its Create method, return the response (resource).
		* get<Resource>: call the Get method of your API client, using the ID of the `DatadogGenericResource` instance (optionally: convert it from string to the expected type).
		* update<Resource>: unmarshal the `jsonSpec` from the instance, call the API client Update method, return the updated resource.
		* delete<Resource>: call the API client Delete method, using the ID of the instance.
	3. Define the 4 methods of `<Resource>Handler` with their respective signatures:
	    * `createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string)`: call your `create<Resource>` function, extract the different fields to update the status of the `DatadogGenericResource` instance.
		* `getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource)`: call your `get<Resource>` function.
		* `updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource)`: call your `update<Resource>` function.
		* `deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource)`: call your `delete<Resource>` function.
3. `internal/controller/datadoggenericresource/utils.go`: Reference your `<Resource>Handler` inside the `getHandler` function.

## Post-development tasks

1. Update the supported resources table in [Overview of the DatadogGenericResource controller][2].
2. Provide an example manifest with the new resource for the [examples][3] folder.

[1]: ./how-to-contribute.md
[2]: ./datadog_generic_resource.md
[3]: ../examples/datadoggenericresource/