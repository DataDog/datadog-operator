// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// createMCPServer creates and configures the MCP server with all registered tools.
func (o *options) createMCPServer() *mcp.Server {
	// Create server with implementation metadata
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "kubectl-datadog-mcp",
		Version: "1.0.0",
	}, nil)

	// Register all LOCAL tool handlers
	o.registerListAgentsTool(server)
	o.registerGetAgentStatusTool(server)
	o.registerDescribeAgentFeaturesTool(server)
	o.registerDescribeAgentComponentsTool(server)
	o.registerGetClusterAgentLeaderTool(server)

	// Register PROXY tools from cluster-agent (if enabled)
	if o.proxyConfig.Enabled {
		if err := o.registerProxyTools(server); err != nil {
			// Log warning but don't fail - local tools still work
			fmt.Fprintf(o.ErrOut, "Warning: Failed to register cluster-agent proxy tools: %v\n", err)
		}
	} else {
		o.registerGetClusterAgentLeaderTool(server)
	}

	return server
}

// registerProxyTools initializes the proxy manager and registers cluster-agent tools.
func (o *options) registerProxyTools(server *mcp.Server) error {
	// Validate proxy configuration
	if err := o.proxyConfig.Validate(); err != nil {
		return fmt.Errorf("invalid proxy configuration: %w", err)
	}

	// Create proxy manager
	pm := NewProxyManager(o)

	// Initialize connection to cluster-agent (30s timeout)
	ctx, cancel := context.WithTimeout(context.Background(), o.proxyConfig.ConnectionTimeout)
	defer cancel()

	if err := pm.Initialize(ctx, o.proxyConfig.DDAName, o.proxyConfig.Port, o.proxyConfig.Endpoint); err != nil {
		return fmt.Errorf("failed to initialize proxy: %w", err)
	}

	// Register proxy tools
	if err := pm.RegisterProxyTools(server); err != nil {
		pm.Shutdown()
		return fmt.Errorf("failed to register proxy tools: %w", err)
	}

	// Store proxy manager for cleanup
	o.proxyManager = pm

	// Log success
	remoteTools := pm.GetRemoteTools()
	proxyInfo := pm.GetProxyInfo()
	fmt.Fprintf(o.Out, "Registered %d cluster-agent proxy tools from %s (pod: %s)\n",
		len(remoteTools), proxyInfo["datadog_agent"], proxyInfo["pod_name"])

	return nil
}
