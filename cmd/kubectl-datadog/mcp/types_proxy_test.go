// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"testing"
	"time"
)

func TestDefaultProxyConfig(t *testing.T) {
	config := DefaultProxyConfig()

	if config == nil {
		t.Fatal("Expected default config to be created")
	}

	if !config.Enabled {
		t.Error("Expected proxy to be enabled by default")
	}

	if config.Port != 5000 {
		t.Errorf("Expected default port 5000, got %d", config.Port)
	}

	if config.Endpoint != "/mcp" {
		t.Errorf("Expected default endpoint '/mcp', got %s", config.Endpoint)
	}

	if config.Timeout != 120*time.Second {
		t.Errorf("Expected default timeout 120s, got %v", config.Timeout)
	}

	if config.ConnectionTimeout != 30*time.Second {
		t.Errorf("Expected default connection timeout 30s, got %v", config.ConnectionTimeout)
	}
}

func TestProxyConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *ProxyConfig
		wantError error
	}{
		{
			name:      "valid config",
			config:    DefaultProxyConfig(),
			wantError: nil,
		},
		{
			name: "invalid port - too low",
			config: &ProxyConfig{
				Port:              0,
				Endpoint:          "/mcp",
				Timeout:           30 * time.Second,
				ConnectionTimeout: 30 * time.Second,
			},
			wantError: ErrInvalidPort,
		},
		{
			name: "invalid port - too high",
			config: &ProxyConfig{
				Port:              65536,
				Endpoint:          "/mcp",
				Timeout:           30 * time.Second,
				ConnectionTimeout: 30 * time.Second,
			},
			wantError: ErrInvalidPort,
		},
		{
			name: "invalid endpoint - empty",
			config: &ProxyConfig{
				Port:              5000,
				Endpoint:          "",
				Timeout:           30 * time.Second,
				ConnectionTimeout: 30 * time.Second,
			},
			wantError: ErrInvalidEndpoint,
		},
		{
			name: "invalid timeout - zero",
			config: &ProxyConfig{
				Port:              5000,
				Endpoint:          "/mcp",
				Timeout:           0,
				ConnectionTimeout: 30 * time.Second,
			},
			wantError: ErrInvalidTimeout,
		},
		{
			name: "invalid timeout - negative",
			config: &ProxyConfig{
				Port:              5000,
				Endpoint:          "/mcp",
				Timeout:           -1 * time.Second,
				ConnectionTimeout: 30 * time.Second,
			},
			wantError: ErrInvalidTimeout,
		},
		{
			name: "invalid connection timeout - zero",
			config: &ProxyConfig{
				Port:              5000,
				Endpoint:          "/mcp",
				Timeout:           30 * time.Second,
				ConnectionTimeout: 0,
			},
			wantError: ErrInvalidConnectionTimeout,
		},
		{
			name: "invalid connection timeout - negative",
			config: &ProxyConfig{
				Port:              5000,
				Endpoint:          "/mcp",
				Timeout:           30 * time.Second,
				ConnectionTimeout: -1 * time.Second,
			},
			wantError: ErrInvalidConnectionTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
