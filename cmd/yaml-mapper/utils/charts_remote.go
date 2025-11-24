// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"helm.sh/helm/v3/pkg/chartutil"
)

const (
	defaultHttpTimeout      = 5 * time.Second
	defaultHttpMaxRetries   = 3
	defaultHttpRetryWaitMin = 200 * time.Millisecond
	defaultHttpRetryWaitMax = 2 * time.Second
)

var httpClient = &http.Client{
	Timeout: defaultHttpTimeout,
}

var retryableClient = func() *retryablehttp.Client {
	c := retryablehttp.NewClient()
	c.HTTPClient = httpClient
	c.RetryWaitMin = defaultHttpRetryWaitMin
	c.RetryWaitMax = defaultHttpRetryWaitMax
	c.RetryMax = defaultHttpMaxRetries
	return c
}()

// FetchLatestValues fetches the latest Datadog Helm chart values.yaml and writes it to a temp file.
func FetchLatestValues() (string, error) {
	// Get the latest chart version
	chartYamlPath, err := FetchYAMLFile("https://raw.githubusercontent.com/DataDog/helm-charts/main/charts/datadog/Chart.yaml", "datadog-Chart")

	if err != nil {
		return "", err
	}

	ddChart, err := chartutil.LoadChartfile(chartYamlPath)
	defer os.Remove(chartYamlPath)
	if err != nil {
		return "", fmt.Errorf("error loading Chart.yaml: %w", err)
	}

	chartVersion := ddChart.Version

	// Fetch the values.yaml file for the chart version
	chartValuesPath, err := FetchYAMLFile(fmt.Sprintf("https://raw.githubusercontent.com/DataDog/helm-charts/refs/tags/datadog-%s/charts/datadog/values.yaml", chartVersion), "datadog-values")
	if err != nil {
		return "", err
	}

	return chartValuesPath, nil
}

// fetchURL makes GET HTTP request with retries.
func fetchURL(url string) (*http.Response, error) {
	req, err := retryablehttp.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return retryableClient.Do(req)
}

// FetchYAMLFile fetches YAML file at URL and writes it to a temp file.
func FetchYAMLFile(url string, name string) (string, error) {
	resp, err := fetchURL(url)
	if err != nil {
		return "", fmt.Errorf("error fetching yaml file: %w\n", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch yaml file %s: %v\n", url, resp.Status)
	}

	tmpFile, err := os.CreateTemp("", fmt.Sprintf("%s.yaml.*", name))
	if err != nil {
		return "", fmt.Errorf("error creating temporary file: %w\n", err)

	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("error saving file: %w\n", err)
	}

	// log.Printf("File downloaded and saved to temporary file: %s\n", tmpFile.Name())
	return tmpFile.Name(), nil
}
