// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/rest"
)

// ProxyManager orchestrates cluster-agent tool proxying
type ProxyManager struct {
	options       *options
	discovery     *ClusterAgentDiscovery
	portForwarder *PortForwarder
	mcpClient     *ClusterAgentMCPClient
	remoteTools   []*mcp.Tool
	ddaName       string
	podName       string
}

// NewProxyManager creates a new proxy manager
func NewProxyManager(opts *options) *ProxyManager {
	return &ProxyManager{
		options: opts,
	}
}

// Initialize discovers cluster-agent, sets up port-forward, and fetches tools
func (pm *ProxyManager) Initialize(ctx context.Context, ddaName string, port int, endpoint string) error {
	// Step 1: Create discovery instance
	pm.discovery = NewClusterAgentDiscovery(
		pm.options.Client,
		pm.options.Clientset,
		pm.options.DiscoveryClient,
		pm.options.UserNamespace,
	)

	// Step 2: Select DatadogAgent
	selectedDDAName, err := pm.discovery.SelectDatadogAgent(ddaName)
	if err != nil {
		return fmt.Errorf("failed to select DatadogAgent: %w", err)
	}
	pm.ddaName = selectedDDAName

	// Step 3: Discover leader pod
	leaderPodName, namespace, err := pm.discovery.DiscoverLeaderPod(pm.ddaName)
	if err != nil {
		// Fallback: try to find any running cluster-agent pod
		pod, fallbackErr := pm.discovery.GetClusterAgentPod(pm.ddaName)
		if fallbackErr != nil {
			return fmt.Errorf("failed to discover cluster-agent pod (leader: %w, fallback: %w)", err, fallbackErr)
		}
		leaderPodName = pod.Name
		namespace = pod.Namespace
	}
	pm.podName = leaderPodName

	// Step 4: Create port forwarder
	var restConfig *rest.Config
	restConfig, err = pm.options.ConfigFlags.ToRawKubeConfigLoader().ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get rest config: %w", err)
	}

	pm.portForwarder = NewPortForwarder(PortForwarderConfig{
		Clientset:  pm.options.Clientset,
		RestConfig: restConfig,
		Namespace:  namespace,
		PodName:    leaderPodName,
		RemotePort: port,
	})

	// Step 5: Start port forwarding
	err = pm.portForwarder.Start()
	if err != nil {
		return fmt.Errorf("failed to start port forward: %w", err)
	}

	// Step 6: Wait for port forward to be ready
	err = pm.portForwarder.WaitReady(30 * time.Second)
	if err != nil {
		pm.portForwarder.Stop()
		return fmt.Errorf("port forward failed to become ready: %w", err)
	}

	// Step 7: Create MCP HTTP client
	pm.mcpClient, err = NewClusterAgentMCPClient(ClusterAgentMCPClientConfig{
		BaseURL:  pm.portForwarder.BaseURL(),
		Endpoint: endpoint,
		Timeout:  120 * time.Second,
	})
	if err != nil {
		pm.portForwarder.Stop()
		return fmt.Errorf("failed to create MCP client: %w", err)
	}

	// Step 8: Connect to cluster-agent MCP server
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err = pm.mcpClient.Connect(connectCtx)
	if err != nil {
		pm.portForwarder.Stop()
		return fmt.Errorf("failed to connect to cluster-agent MCP server: %w", err)
	}

	// Step 9: Fetch tools from cluster-agent
	listCtx, listCancel := context.WithTimeout(ctx, 30*time.Second)
	defer listCancel()

	tools, err := pm.mcpClient.ListTools(listCtx)
	if err != nil {
		pm.Shutdown()
		return fmt.Errorf("failed to list tools from cluster-agent: %w", err)
	}

	pm.remoteTools = tools

	return nil
}

// RegisterProxyTools registers all discovered tools with the MCP server
func (pm *ProxyManager) RegisterProxyTools(server *mcp.Server) error {
	if pm.remoteTools == nil {
		return fmt.Errorf("no remote tools available, call Initialize() first")
	}

	for _, tool := range pm.remoteTools {
		// Prefix tool name to avoid conflicts with local tools
		proxiedName := fmt.Sprintf("cluster_agent_%s", tool.Name)

		// Create a copy of the tool with the prefixed name
		proxiedTool := &mcp.Tool{
			Name:        proxiedName,
			Description: fmt.Sprintf("[Cluster-Agent %s] %s", pm.ddaName, tool.Description),
			// Note: InputSchema is already part of the tool from ListTools
		}

		// Create proxy handler for this tool
		handler := pm.createProxyToolHandler(tool.Name)

		// Register the tool with the server
		// We use the generic AddTool function with map[string]interface{} for flexibility
		mcp.AddTool(server, proxiedTool, handler)
	}

	return nil
}

// createProxyToolHandler creates a handler that forwards tool calls to cluster-agent
func (pm *ProxyManager) createProxyToolHandler(originalToolName string) func(context.Context, *mcp.CallToolRequest, map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args map[string]interface{}) (*mcp.CallToolResult, interface{}, error) {
		// Forward the tool call to cluster-agent
		result, err := pm.mcpClient.CallTool(ctx, originalToolName, args)
		if err != nil {
			// Return error as MCP CallToolResult
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf("Failed to call cluster-agent tool %s: %v", originalToolName, err),
					},
				},
			}, nil, nil
		}

		// Return the result from cluster-agent
		return result, nil, nil
	}
}

// Shutdown gracefully closes all connections
func (pm *ProxyManager) Shutdown() {
	if pm.mcpClient != nil {
		_ = pm.mcpClient.Close()
	}
	if pm.portForwarder != nil {
		pm.portForwarder.Stop()
	}
}

// GetRemoteTools returns the list of tools fetched from cluster-agent
func (pm *ProxyManager) GetRemoteTools() []*mcp.Tool {
	return pm.remoteTools
}

// GetProxyInfo returns information about the proxy configuration
func (pm *ProxyManager) GetProxyInfo() map[string]string {
	info := map[string]string{
		"datadog_agent": pm.ddaName,
		"pod_name":      pm.podName,
		"namespace":     pm.options.UserNamespace,
	}

	if pm.portForwarder != nil {
		info["local_port"] = fmt.Sprintf("%d", pm.portForwarder.LocalPort())
	}

	if pm.mcpClient != nil && pm.mcpClient.IsConnected() {
		info["connected"] = "true"
	} else {
		info["connected"] = "false"
	}

	return info
}

// HealthCheck performs a health check on the proxy connection
func (pm *ProxyManager) HealthCheck(ctx context.Context) error {
	if pm.mcpClient == nil {
		return fmt.Errorf("MCP client not initialized")
	}

	if !pm.mcpClient.IsConnected() {
		return fmt.Errorf("MCP client not connected")
	}

	// Check port forwarder
	if pm.portForwarder != nil {
		if err := pm.portForwarder.GetError(); err != nil {
			return fmt.Errorf("port forward error: %w", err)
		}
	}

	// Ping the cluster-agent
	if err := pm.mcpClient.Ping(ctx); err != nil {
		return fmt.Errorf("cluster-agent ping failed: %w", err)
	}

	return nil
}
