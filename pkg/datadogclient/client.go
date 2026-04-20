// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogclient

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	datadogV1 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	datadogV2 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/go-logr/logr"

	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const prefix = "https://api."

// ParsedAPIURL holds the parsed API URL components used for constructing auth contexts.
type ParsedAPIURL struct {
	Host     string
	Protocol string
}

// ParseURL parses the API URL from environment variables and returns a ParsedAPIURL.
// This should be called once at startup. Returns nil if no custom API URL is configured.
func ParseURL(logger logr.Logger) (*ParsedAPIURL, error) {
	apiURL := ""
	if os.Getenv(constants.DDddURL) != "" {
		apiURL = os.Getenv(constants.DDddURL)
	} else if os.Getenv(constants.DDURL) != "" {
		apiURL = os.Getenv(constants.DDURL)
	} else if site := os.Getenv(constants.DDSite); site != "" {
		apiURL = prefix + strings.TrimSpace(site)
	}

	if apiURL == "" {
		return nil, nil
	}

	logger.Info("Got API URL for DatadogOperator controller", "URL", apiURL)
	parsedAPIURL, parseErr := url.Parse(apiURL)
	if parseErr != nil {
		return nil, fmt.Errorf(`invalid API URL : %w`, parseErr)
	}
	if parsedAPIURL.Host == "" || parsedAPIURL.Scheme == "" {
		return nil, fmt.Errorf(`missing protocol or host : %s`, apiURL)
	}

	return &ParsedAPIURL{
		Host:     parsedAPIURL.Host,
		Protocol: parsedAPIURL.Scheme,
	}, nil
}

// GetAuth creates a fresh authentication context from the given credentials and parsed API URL.
// This should be called on every reconcile to use the latest credentials.
func GetAuth(creds config.Creds, apiURL *ParsedAPIURL) context.Context {
	auth := context.WithValue(
		context.Background(),
		datadogapi.ContextAPIKeys,
		map[string]datadogapi.APIKey{
			"apiKeyAuth": {
				Key: creds.APIKey,
			},
			"appKeyAuth": {
				Key: creds.AppKey,
			},
		},
	)

	if apiURL != nil {
		auth = context.WithValue(auth, datadogapi.ContextServerIndex, 1)
		auth = context.WithValue(auth, datadogapi.ContextServerVariables, map[string]string{
			"name":     apiURL.Host,
			"protocol": apiURL.Protocol,
		})
	}

	return auth
}

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
