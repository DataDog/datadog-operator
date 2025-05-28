// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api provides test helpers to interact with the Datadog API
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
)

// Client represents the datadog API context
type Client struct {
	api    *datadog.APIClient
	ctx    context.Context
	http   http.Client
	apiKey string
	appKey string
}

// NewClient initialise a client with the API and APP keys
func NewClient() *Client {
	apiKey, _ := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	appKey, _ := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: apiKey,
			},
			"appKeyAuth": {
				Key: appKey,
			},
		},
	)

	cfg := datadog.NewConfiguration()

	return &Client{
		api:    datadog.NewAPIClient(cfg),
		ctx:    ctx,
		http:   http.Client{},
		apiKey: apiKey,
		appKey: appKey,
	}
}

// GetAPIKey returns the APIKey
func GetAPIKey() (string, error) {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if err != nil {
		return "", fmt.Errorf("could not get APIKey")
	}
	return apiKey, nil
}

// GetAPPKey returns the APPKey
func GetAPPKey() (string, error) {
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	if err != nil {
		return "", fmt.Errorf("could not get APPKey")
	}
	return appKey, nil
}
