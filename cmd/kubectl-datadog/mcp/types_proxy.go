// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"errors"
	"time"
)

// Proxy configuration errors
var (
	ErrInvalidPort              = errors.New("invalid port: must be between 1 and 65535")
	ErrInvalidEndpoint          = errors.New("invalid endpoint: cannot be empty")
	ErrInvalidTimeout           = errors.New("invalid timeout: must be positive")
	ErrInvalidConnectionTimeout = errors.New("invalid connection timeout: must be positive")
)

// ProxyConfig contains configuration for the cluster-agent MCP proxy
type ProxyConfig struct {
	// Enabled indicates whether the proxy is enabled
	Enabled bool

	// DDAName is the DatadogAgent name to proxy
	// If empty, the first DatadogAgent with cluster-agent enabled will be auto-selected
	DDAName string

	// Port is the cluster-agent MCP server port
	// Default: 5000 (cluster-agent metrics port)
	Port int

	// Endpoint is the MCP endpoint path on the cluster-agent
	// Default: "/mcp"
	Endpoint string

	// Timeout is the HTTP timeout for tool calls
	// Default: 120 seconds
	Timeout time.Duration

	// ConnectionTimeout is the timeout for establishing connection to cluster-agent
	// Default: 30 seconds
	ConnectionTimeout time.Duration
}

// DefaultProxyConfig returns a ProxyConfig with default values
func DefaultProxyConfig() *ProxyConfig {
	return &ProxyConfig{
		Enabled:           true,
		DDAName:           "",
		Port:              5000,
		Endpoint:          "/mcp",
		Timeout:           120 * time.Second,
		ConnectionTimeout: 30 * time.Second,
	}
}

// Validate checks if the proxy configuration is valid
func (pc *ProxyConfig) Validate() error {
	if pc.Port <= 0 || pc.Port > 65535 {
		return ErrInvalidPort
	}

	if pc.Endpoint == "" {
		return ErrInvalidEndpoint
	}

	if pc.Timeout <= 0 {
		return ErrInvalidTimeout
	}

	if pc.ConnectionTimeout <= 0 {
		return ErrInvalidConnectionTimeout
	}

	return nil
}
