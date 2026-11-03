// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/pkg/version"
)

func TestClientsSendOperatorUserAgent(t *testing.T) {
	originalVersion := version.Version
	t.Cleanup(func() { version.Version = originalVersion })

	userAgents := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgents <- r.UserAgent()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true}`))
	}))
	t.Cleanup(server.Close)

	tests := []struct {
		name    string
		version string
	}{
		{name: "build version", version: originalVersion},
		{name: "release", version: "v1.30.0"},
		{name: "release without prefix", version: "1.30.0"},
		{name: "release candidate", version: "v1.30.0-rc.1"},
		{name: "semver build metadata", version: "1.30.0+build.123"},
		{name: "nightly or local commit", version: "3e90dd3d7"},
		{name: "full commit", version: "3e90dd3d7d578eb851d7be94d470be1d612f2edf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version.Version = tt.version
			generic := InitGenericClients(nil)
			clients := map[string]*datadogapi.APIClient{
				"base":                       NewAPIClient(),
				"monitors":                   InitMonitorClient(nil).Client,
				"slos":                       InitSLOClient(nil).Client,
				"dashboards":                 InitDashboardClient(nil).Client,
				"generic dashboards":         generic.DashboardsClient.Client,
				"generic synthetics":         generic.SyntheticsClient.Client,
				"generic notebooks":          generic.NotebooksClient.Client,
				"generic monitors":           generic.MonitorsClient.Client,
				"generic slos":               generic.SLOsClient.Client,
				"generic slo corrections":    generic.SLOCorrectionsClient.Client,
				"generic downtimes":          generic.DowntimesClient.Client,
				"generic notification rules": generic.MonitorNotificationRulesClient.Client,
				"generic software catalog":   generic.SoftwareCatalogClient.Client,
			}
			for name, client := range clients {
				t.Run(name, func(t *testing.T) {
					client.GetConfig().Servers = datadogapi.ServerConfigurations{{URL: server.URL}}
					_, _, err := datadogV1.NewAuthenticationApi(client).Validate(context.Background())
					require.NoError(t, err)
					require.Equal(t, "Datadog Operator/"+tt.version, <-userAgents)
				})
			}
		})
	}
}

func TestDelegatedAuthClientsSendBearerToken(t *testing.T) {
	type authHeaders struct{ authorization, apiKey, appKey string }
	received := make(chan authHeaders, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- authHeaders{r.Header.Get("Authorization"), r.Header.Get("DD-API-KEY"), r.Header.Get("DD-APPLICATION-KEY")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)

	delegatedConfig := &datadogapi.DelegatedTokenConfig{OrgUUID: "org-1", Provider: datadogapi.ProviderAWS}
	client := InitMonitorClient(delegatedConfig)
	client.Client.GetConfig().Servers = datadogapi.ServerConfigurations{{URL: server.URL}}

	ctx := context.WithValue(context.Background(), datadogapi.ContextDelegatedToken, &datadogapi.DelegatedTokenCredentials{
		OrgUUID:        "org-1",
		DelegatedToken: "dd-token",
		Expiration:     time.Now().Add(time.Hour),
	})
	_, _, err := client.ListMonitors(ctx)
	require.NoError(t, err)
	require.Equal(t, authHeaders{authorization: "Bearer dd-token"}, <-received)

	// Without delegated auth, the same client type keeps using API/APP keys.
	keysClient := InitMonitorClient(nil)
	keysClient.Client.GetConfig().Servers = datadogapi.ServerConfigurations{{URL: server.URL}}
	ctx = context.WithValue(context.Background(), datadogapi.ContextAPIKeys, map[string]datadogapi.APIKey{
		"apiKeyAuth": {Key: "api"},
		"appKeyAuth": {Key: "app"},
	})
	_, _, err = keysClient.ListMonitors(ctx)
	require.NoError(t, err)
	require.Equal(t, authHeaders{apiKey: "api", appKey: "app"}, <-received)
}
