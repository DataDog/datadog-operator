// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package datadogclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/DataDog/datadog-operator/pkg/config"

	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
)

// DatadogClient contains the Datadog API Client and Authentication context
type DatadogClient struct {
	Client *datadogapiclientv1.APIClient
	Auth   context.Context
}

// InitDatadogClient initializes the Datadog API Client and establishes credentials
func InitDatadogClient(apiKey, appKey string) (DatadogClient, error) {
	if apiKey == "" || appKey == "" {
		return DatadogClient{}, errors.New("error obtaining API key and/or app key")
	}

	// Initialize the official Datadog V1 API client
	authV1 := context.WithValue(
		context.Background(),
		datadogapiclientv1.ContextAPIKeys,
		map[string]datadogapiclientv1.APIKey{
			"apiKeyAuth": {
				Key: apiKey,
			},
			"appKeyAuth": {
				Key: appKey,
			},
		},
	)
	configV1 := datadogapiclientv1.NewConfiguration()

	if apiURL := os.Getenv(config.DDURLEnvVar); apiURL != "" {
		parsedAPIURL, parseErr := url.Parse(apiURL)
		if parseErr != nil {
			return DatadogClient{}, fmt.Errorf(`invalid API Url : %v`, parseErr)
		}
		if parsedAPIURL.Host == "" || parsedAPIURL.Scheme == "" {
			return DatadogClient{}, fmt.Errorf(`missing protocol or host : %v`, apiURL)
		}
		// If api url is passed, set and use the api name and protocol on ServerIndex{1}
		authV1 = context.WithValue(authV1, datadogapiclientv1.ContextServerIndex, 1)
		authV1 = context.WithValue(authV1, datadogapiclientv1.ContextServerVariables, map[string]string{
			"name":     parsedAPIURL.Host,
			"protocol": parsedAPIURL.Scheme,
		})
	}
	client := datadogapiclientv1.NewAPIClient(configV1)

	return DatadogClient{Client: client, Auth: authV1}, nil
}
