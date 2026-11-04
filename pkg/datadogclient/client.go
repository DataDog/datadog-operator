// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogclient

import (
	"net/http"
	"strings"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	"github.com/DataDog/datadog-operator/pkg/version"
)

// DelegatedAuth provides delegated token authentication to API clients.
// config.CredentialManager implements it.
type DelegatedAuth interface {
	// DelegatedTokenConfig returns nil when delegated auth is disabled.
	DelegatedTokenConfig() *datadogapi.DelegatedTokenConfig
	// InvalidateDelegatedToken is called when Datadog rejects a token with 401 Unauthorized.
	InvalidateDelegatedToken(token string)
}

// NewAPIClient creates a Datadog API client identified by the operator build version.
// It authenticates with the API/APP keys carried by the request context.
func NewAPIClient() *datadogapi.APIClient {
	return NewAPIClientWithDelegatedAuth(nil)
}

// NewAPIClientWithDelegatedAuth creates a Datadog API client. When delegated auth is
// enabled, the client authenticates every request with the delegated token carried by the
// request context (datadogapi.ContextDelegatedToken) instead of API/APP keys, and reports
// tokens rejected with 401 Unauthorized so they are renewed on the next reconcile.
// Otherwise, it uses the API/APP keys carried by the request context.
// Clients for intake endpoints (metrics, events) must keep using NewAPIClient.
func NewAPIClientWithDelegatedAuth(auth DelegatedAuth) *datadogapi.APIClient {
	config := datadogapi.NewConfiguration()
	config.UserAgent = "Datadog Operator/" + version.GetVersion()
	if auth != nil {
		if delegatedTokenConfig := auth.DelegatedTokenConfig(); delegatedTokenConfig != nil {
			config.DelegatedTokenConfig = delegatedTokenConfig
			config.HTTPClient = &http.Client{
				Transport: &invalidateOnUnauthorized{base: http.DefaultTransport, invalidate: auth.InvalidateDelegatedToken},
			}
		}
	}
	return datadogapi.NewAPIClient(config)
}

// invalidateOnUnauthorized reports the bearer token of requests rejected with
// 401 Unauthorized. 403 Forbidden is not reported: it means the identity lacks a
// permission, which a new token would not fix.
type invalidateOnUnauthorized struct {
	base       http.RoundTripper
	invalidate func(token string)
}

func (t *invalidateOnUnauthorized) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		if token, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer "); ok {
			t.invalidate(token)
		}
	}
	return resp, err
}

// InitMonitorClient creates a stateless Datadog Monitor API client.
func InitMonitorClient(auth DelegatedAuth) *datadogV1.MonitorsApi {
	apiClient := NewAPIClientWithDelegatedAuth(auth)
	return datadogV1.NewMonitorsApi(apiClient)
}

// InitSLOClient creates a stateless Datadog SLO API client.
func InitSLOClient(auth DelegatedAuth) *datadogV1.ServiceLevelObjectivesApi {
	apiClient := NewAPIClientWithDelegatedAuth(auth)
	return datadogV1.NewServiceLevelObjectivesApi(apiClient)
}

// InitDashboardClient creates a stateless Datadog Dashboard API client.
func InitDashboardClient(auth DelegatedAuth) *datadogV1.DashboardsApi {
	apiClient := NewAPIClientWithDelegatedAuth(auth)
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
func InitGenericClients(auth DelegatedAuth) *GenericClients {
	apiClient := NewAPIClientWithDelegatedAuth(auth)
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
