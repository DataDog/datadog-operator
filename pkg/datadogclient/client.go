// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogclient

import (
	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	"github.com/DataDog/datadog-operator/pkg/version"
)

// NewAPIClient creates a Datadog API client identified by the operator build version.
func NewAPIClient() *datadogapi.APIClient {
	config := datadogapi.NewConfiguration()
	config.UserAgent = "Datadog Operator/" + version.GetVersion()
	return datadogapi.NewAPIClient(config)
}

// InitMonitorClient creates a stateless Datadog Monitor API client.
func InitMonitorClient() *datadogV1.MonitorsApi {
	apiClient := NewAPIClient()
	return datadogV1.NewMonitorsApi(apiClient)
}

// InitSLOClient creates a stateless Datadog SLO API client.
func InitSLOClient() *datadogV1.ServiceLevelObjectivesApi {
	apiClient := NewAPIClient()
	return datadogV1.NewServiceLevelObjectivesApi(apiClient)
}

// InitDashboardClient creates a stateless Datadog Dashboard API client.
func InitDashboardClient() *datadogV1.DashboardsApi {
	apiClient := NewAPIClient()
	return datadogV1.NewDashboardsApi(apiClient)
}

// GenericClients holds the stateless API clients for generic resource operations.
type GenericClients struct {
	DashboardsClient               *datadogV1.DashboardsApi
	SyntheticsClient               *datadogV1.SyntheticsApi
	NotebooksClient                *datadogV1.NotebooksApi
	MonitorsClient                 *datadogV1.MonitorsApi
	SLOsClient                     *datadogV1.ServiceLevelObjectivesApi
	SLOCorrectionsClient           *datadogV1.ServiceLevelObjectiveCorrectionsApi
	DowntimesClient                *datadogV2.DowntimesApi
	MonitorNotificationRulesClient *datadogV2.MonitorsApi
	SoftwareCatalogClient          *datadogV2.SoftwareCatalogApi
}

// InitGenericClients creates stateless Datadog API clients for generic resource operations.
func InitGenericClients() *GenericClients {
	apiClient := NewAPIClient()
	return &GenericClients{
		DashboardsClient:               datadogV1.NewDashboardsApi(apiClient),
		SyntheticsClient:               datadogV1.NewSyntheticsApi(apiClient),
		NotebooksClient:                datadogV1.NewNotebooksApi(apiClient),
		MonitorsClient:                 datadogV1.NewMonitorsApi(apiClient),
		SLOsClient:                     datadogV1.NewServiceLevelObjectivesApi(apiClient),
		SLOCorrectionsClient:           datadogV1.NewServiceLevelObjectiveCorrectionsApi(apiClient),
		DowntimesClient:                datadogV2.NewDowntimesApi(apiClient),
		MonitorNotificationRulesClient: datadogV2.NewMonitorsApi(apiClient),
		SoftwareCatalogClient:          datadogV2.NewSoftwareCatalogApi(apiClient),
	}
}
