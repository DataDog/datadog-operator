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
	"github.com/go-logr/logr"

	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const prefix = "https://api."

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

// GenericClients holds multiple API clients for the Generic Resource controller.
type GenericClients struct {
	SyntheticsClient *datadogV1.SyntheticsApi
	NotebooksClient  *datadogV1.NotebooksApi
	MonitorsClient   *datadogV1.MonitorsApi
}

// InitGenericClients creates stateless Datadog API clients for generic resources.
func InitGenericClients() GenericClients {
	configV1 := datadogapi.NewConfiguration()
	apiClient := datadogapi.NewAPIClient(configV1)
	return GenericClients{
		SyntheticsClient: datadogV1.NewSyntheticsApi(apiClient),
		NotebooksClient:  datadogV1.NewNotebooksApi(apiClient),
		MonitorsClient:   datadogV1.NewMonitorsApi(apiClient),
	}
}

// SetupAuth creates an authenticated context for Datadog API calls.
// Auth contexts should be short-lived and recreated when credentials change.
func GetAuth(logger logr.Logger, creds config.Creds) (context.Context, error) {
	// Initialize the official Datadog V1 API client.
	authV1 := context.WithValue(
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

	apiURL := ""
	if os.Getenv(constants.DDddURL) != "" {
		apiURL = os.Getenv(constants.DDddURL)
	} else if os.Getenv(constants.DDURL) != "" {
		apiURL = os.Getenv(constants.DDURL)
	} else if site := os.Getenv(constants.DDSite); site != "" {
		apiURL = prefix + strings.TrimSpace(site)
	}

	if apiURL != "" {
		logger.Info("Got API URL for DatadogOperator controller", "URL", apiURL)
		parsedAPIURL, parseErr := url.Parse(apiURL)
		if parseErr != nil {
			return authV1, fmt.Errorf(`invalid API URL : %w`, parseErr)
		}
		if parsedAPIURL.Host == "" || parsedAPIURL.Scheme == "" {
			return authV1, fmt.Errorf(`missing protocol or host : %s`, apiURL)
		}
		// If API URL is passed, set and use the API name and protocol on ServerIndex{1}.
		authV1 = context.WithValue(authV1, datadogapi.ContextServerIndex, 1)
		authV1 = context.WithValue(authV1, datadogapi.ContextServerVariables, map[string]string{
			"name":     parsedAPIURL.Host,
			"protocol": parsedAPIURL.Scheme,
		})
	}

	return authV1, nil
}
