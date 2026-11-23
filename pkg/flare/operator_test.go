// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectHealthEndpoints(t *testing.T) {
	collector := func(get HealthGetter) Collector {
		return Collector{Name: "health", Collect: func(ctx context.Context, c *Collection) error {
			return CollectHealthEndpoints(ctx, c, get)
		}}
	}
	files, manifest := runCollectors(t, Env{}, collector(func(_ context.Context, endpoint string) (string, []byte, error) {
		if endpoint == "/readyz?verbose" {
			return "500 Internal Server Error", []byte("[-]ping failed"), nil
		}
		return "200 OK", []byte("[+]ping ok"), nil
	}))
	assert.Equal(t, StatusOK, manifest.Collectors[0].Status)
	assert.Contains(t, files["operator/health.txt"], "GET /healthz?verbose: 200 OK\n[+]ping ok")
	assert.Contains(t, files["operator/health.txt"], "GET /readyz?verbose: 500 Internal Server Error\n[-]ping failed")

	files, manifest = runCollectors(t, Env{}, collector(func(_ context.Context, endpoint string) (string, []byte, error) {
		if endpoint == "/readyz?verbose" {
			return "", nil, errors.New("connection refused")
		}
		return "200 OK", nil, nil
	}))
	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)
	assert.Contains(t, files, "operator/health.txt")

	// The Operator registers no readiness checks: /readyz is a 404.
	files, manifest = runCollectors(t, Env{}, collector(func(_ context.Context, endpoint string) (string, []byte, error) {
		if endpoint == "/readyz?verbose" {
			return "404 Not Found", []byte("404 page not found"), nil
		}
		return "200 OK", []byte("healthz check passed"), nil
	}))
	assert.Equal(t, StatusOK, manifest.Collectors[0].Status)
	assert.Contains(t, files["operator/health.txt"], "GET /readyz?verbose: 404 Not Found (no checks are registered for this endpoint)")
	assert.NotContains(t, files["operator/health.txt"], "404 page not found")

	_, manifest = runCollectors(t, Env{}, collector(func(context.Context, string) (string, []byte, error) {
		return "", nil, errors.New("connection refused")
	}))
	assert.Equal(t, StatusError, manifest.Collectors[0].Status)
}

func TestRedaction(t *testing.T) {
	assert.Equal(t, redacted, RedactFlagValue("secretBackendConfig", "x"))
	assert.Equal(t, redacted, RedactFlagValue("apiKey", "x"))
	assert.Equal(t, "", RedactFlagValue("apiKey", ""))
	assert.Equal(t, "debug", RedactFlagValue("loglevel", "debug"))
	assert.Equal(t, redacted, RedactEnvValue("DD_API_KEY_FILE", "/x"))
	assert.Equal(t, "datadoghq.com", RedactEnvValue("DD_SITE", "datadoghq.com"))
	assert.True(t, IsCollectedEnv("DD_SITE"))
	assert.False(t, IsCollectedEnv("HOME"))
}
