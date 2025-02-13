TODO: depending on refactor https://github.com/DataDog/datadog-operator/pull/1640
1. Add in a new const `SupportedResourcesType` in `api/datadoghq/v1alpha1/datadoggenericresource_types.go`
2. Add it to validation enum `allowedCustomResourcesEnumMap` (`api/datadoghq/v1alpha1/datadoggenericresource_validation.go`)
3. Add a new client within `pkg/datadogclient/client.go/InitDatadogGenericClient`
4. Add this new client to the `internal/controller/datadoggenericresource/controller.go` file inside `Reconciler` struct
5. Add a new `internal/controller/datadoggenericresource/resource_replace_me.go`: 
	* `ResourceReplaceMeHandler`
	* CRUD operations
6. Add new resource inside `internal/controller/datadoggenericresource/utils.go` to `getHandler`

Example PR: https://github.com/DataDog/datadog-operator/pull/1641