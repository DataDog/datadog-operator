// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogclient

import (
	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// InitMonitorClient creates a stateless Datadog Monitor API client.
func InitMonitorClient() *datadogV1.MonitorsApi {
	configV1 := datadogapi.NewConfiguration()
	apiClient := datadogapi.NewAPIClient(configV1)
	return datadogV1.NewMonitorsApi(apiClient)
}

// InitSLOClient creates a stateless Datadog SLO API client.
func InitSLOClient() *datadogV1.ServiceLevelObjectivesApi {
	configV1 := datadogapi.NewConfiguration()
	apiClient := datadogapi.NewAPIClient(configV1)
	return datadogV1.NewServiceLevelObjectivesApi(apiClient)
}

// InitDashboardClient creates a stateless Datadog Dashboard API client.
func InitDashboardClient() *datadogV1.DashboardsApi {
	configV1 := datadogapi.NewConfiguration()
	apiClient := datadogapi.NewAPIClient(configV1)
	return datadogV1.NewDashboardsApi(apiClient)
}

// GenericClients holds the stateless API clients for generic resource operations.
type GenericClients struct {
	DashboardsClient *datadogV1.DashboardsApi
	SyntheticsClient *datadogV1.SyntheticsApi
	NotebooksClient  *datadogV1.NotebooksApi
	MonitorsClient   *datadogV1.MonitorsApi
	DowntimesClient  *datadogV2.DowntimesApi
}

// InitGenericClients creates stateless Datadog API clients for generic resource operations.
func InitGenericClients() *GenericClients {
	configV1 := datadogapi.NewConfiguration()
	apiClient := datadogapi.NewAPIClient(configV1)
	return &GenericClients{
		DashboardsClient: datadogV1.NewDashboardsApi(apiClient),
		SyntheticsClient: datadogV1.NewSyntheticsApi(apiClient),
		NotebooksClient:  datadogV1.NewNotebooksApi(apiClient),
		MonitorsClient:   datadogV1.NewMonitorsApi(apiClient),
		DowntimesClient:  datadogV2.NewDowntimesApi(apiClient),
	}
}
