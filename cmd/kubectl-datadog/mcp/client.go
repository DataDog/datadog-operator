// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ClusterAgentMCPClient communicates with the cluster-agent MCP server via HTTP
type ClusterAgentMCPClient struct {
	baseURL    string
	endpoint   string
	httpClient *http.Client
	mcpClient  *mcp.Client
	session    *mcp.ClientSession
}

// ClusterAgentMCPClientConfig contains configuration for creating a ClusterAgentMCPClient
type ClusterAgentMCPClientConfig struct {
	BaseURL    string        // e.g., "http://localhost:12345"
	Endpoint   string        // e.g., "/mcp"
	Timeout    time.Duration // HTTP timeout for tool calls
	MaxRetries int           // Maximum reconnection retries
}

// NewClusterAgentMCPClient creates a new MCP HTTP client for cluster-agent communication
func NewClusterAgentMCPClient(config ClusterAgentMCPClientConfig) (*ClusterAgentMCPClient, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	if config.Endpoint == "" {
		config.Endpoint = "/mcp"
	}

	if config.Timeout == 0 {
		config.Timeout = 120 * time.Second
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: config.Timeout,
	}

	// Create MCP client
	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "kubectl-datadog-proxy",
		Version: "1.0.0",
	}, nil)

	return &ClusterAgentMCPClient{
		baseURL:    config.BaseURL,
		endpoint:   config.Endpoint,
		httpClient: httpClient,
		mcpClient:  mcpClient,
	}, nil
}

// Connect establishes a connection to the cluster-agent MCP server
func (c *ClusterAgentMCPClient) Connect(ctx context.Context) error {
	// Build full endpoint URL
	fullURL := c.baseURL + c.endpoint

	// Create streamable HTTP transport
	transport := &mcp.StreamableClientTransport{
		Endpoint:   fullURL,
		HTTPClient: c.httpClient,
		MaxRetries: 3,
	}

	// Connect to the cluster-agent MCP server
	session, err := c.mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to cluster-agent MCP server: %w", err)
	}

	c.session = session
	return nil
}

// ListTools fetches the list of available tools from the cluster-agent
func (c *ClusterAgentMCPClient) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	if c.session == nil {
		return nil, fmt.Errorf("client not connected, call Connect() first")
	}

	// List tools from the cluster-agent
	result, err := c.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return result.Tools, nil
}

// CallTool executes a tool on the cluster-agent
func (c *ClusterAgentMCPClient) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if c.session == nil {
		return nil, fmt.Errorf("client not connected, call Connect() first")
	}

	// Call the tool on the cluster-agent
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call tool %s: %w", name, err)
	}

	return result, nil
}

// Close closes the connection to the cluster-agent MCP server
func (c *ClusterAgentMCPClient) Close() error {
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

// IsConnected returns true if the client is connected to the cluster-agent
func (c *ClusterAgentMCPClient) IsConnected() bool {
	return c.session != nil
}

// GetServerCapabilities returns capabilities of the connected MCP server
func (c *ClusterAgentMCPClient) GetServerCapabilities() (*mcp.ServerCapabilities, error) {
	if c.session == nil {
		return nil, fmt.Errorf("client not connected")
	}

	// Server capabilities are available in the InitializeResult
	initResult := c.session.InitializeResult()
	if initResult == nil {
		return nil, fmt.Errorf("server not initialized")
	}

	return initResult.Capabilities, nil
}

// Ping sends a ping request to verify the connection is alive
func (c *ClusterAgentMCPClient) Ping(ctx context.Context) error {
	if c.session == nil {
		return fmt.Errorf("client not connected")
	}

	// Use a simple operation like listing tools to verify connectivity
	_, err := c.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}

// Reconnect attempts to reconnect to the cluster-agent MCP server
func (c *ClusterAgentMCPClient) Reconnect(ctx context.Context) error {
	// Close existing connection if any
	if c.session != nil {
		_ = c.session.Close()
		c.session = nil
	}

	// Attempt to reconnect
	return c.Connect(ctx)
}
