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
			generic := InitGenericClients()
			clients := map[string]*datadogapi.APIClient{
				"base":                       NewAPIClient(),
				"monitors":                   InitMonitorClient().Client,
				"slos":                       InitSLOClient().Client,
				"dashboards":                 InitDashboardClient().Client,
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
					require.Equal(t, "datadog-operator/"+tt.version, <-userAgents)
				})
			}
		})
	}
}
