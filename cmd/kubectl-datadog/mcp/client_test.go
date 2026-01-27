// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClusterAgentMCPClient(t *testing.T) {
	config := ClusterAgentMCPClientConfig{
		BaseURL:  "http://localhost:5000",
		Endpoint: "/mcp",
		Timeout:  30 * time.Second,
	}

	client, err := NewClusterAgentMCPClient(config)
	if err != nil {
		t.Fatalf("Expected client to be created, got error: %v", err)
	}

	if client == nil {
		t.Fatal("Expected client to be created")
	}

	if client.baseURL != "http://localhost:5000" {
		t.Errorf("Expected baseURL 'http://localhost:5000', got %s", client.baseURL)
	}

	if client.endpoint != "/mcp" {
		t.Errorf("Expected endpoint '/mcp', got %s", client.endpoint)
	}
}

func TestNewClusterAgentMCPClient_Defaults(t *testing.T) {
	config := ClusterAgentMCPClientConfig{
		BaseURL: "http://localhost:5000",
	}

	client, err := NewClusterAgentMCPClient(config)
	if err != nil {
		t.Fatalf("Expected client to be created, got error: %v", err)
	}

	// Check defaults
	if client.endpoint != "/mcp" {
		t.Errorf("Expected default endpoint '/mcp', got %s", client.endpoint)
	}

	if client.httpClient.Timeout != 120*time.Second {
		t.Errorf("Expected default timeout 120s, got %v", client.httpClient.Timeout)
	}
}

func TestNewClusterAgentMCPClient_NoBaseURL(t *testing.T) {
	config := ClusterAgentMCPClientConfig{
		Endpoint: "/mcp",
	}

	_, err := NewClusterAgentMCPClient(config)
	if err == nil {
		t.Fatal("Expected error when baseURL is empty")
	}
}

func TestClusterAgentMCPClient_IsConnected(t *testing.T) {
	config := ClusterAgentMCPClientConfig{
		BaseURL: "http://localhost:5000",
	}

	client, err := NewClusterAgentMCPClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Before connect
	if client.IsConnected() {
		t.Error("Expected client to not be connected before Connect()")
	}
}

func TestClusterAgentMCPClient_ListTools_NotConnected(t *testing.T) {
	config := ClusterAgentMCPClientConfig{
		BaseURL: "http://localhost:5000",
	}

	client, err := NewClusterAgentMCPClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	_, err = client.ListTools(ctx)
	if err == nil {
		t.Error("Expected error when calling ListTools before Connect()")
	}
}

func TestClusterAgentMCPClient_CallTool_NotConnected(t *testing.T) {
	config := ClusterAgentMCPClientConfig{
		BaseURL: "http://localhost:5000",
	}

	client, err := NewClusterAgentMCPClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	_, err = client.CallTool(ctx, "test_tool", map[string]interface{}{})
	if err == nil {
		t.Error("Expected error when calling CallTool before Connect()")
	}
}

func TestClusterAgentMCPClient_Close(t *testing.T) {
	config := ClusterAgentMCPClientConfig{
		BaseURL: "http://localhost:5000",
	}

	client, err := NewClusterAgentMCPClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Close should not error even if not connected
	err = client.Close()
	if err != nil {
		t.Errorf("Expected Close() to succeed even when not connected, got error: %v", err)
	}
}

func TestClusterAgentMCPClient_GetServerCapabilities_NotConnected(t *testing.T) {
	config := ClusterAgentMCPClientConfig{
		BaseURL: "http://localhost:5000",
	}

	client, err := NewClusterAgentMCPClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = client.GetServerCapabilities()
	if err == nil {
		t.Error("Expected error when calling GetServerCapabilities before Connect()")
	}
}

func TestClusterAgentMCPClient_Ping_NotConnected(t *testing.T) {
	config := ClusterAgentMCPClientConfig{
		BaseURL: "http://localhost:5000",
	}

	client, err := NewClusterAgentMCPClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	err = client.Ping(ctx)
	if err == nil {
		t.Error("Expected error when calling Ping before Connect()")
	}
}

// TestClusterAgentMCPClient_Integration tests the client with a mock HTTP server
// This test is skipped by default as it requires a full MCP server implementation
func TestClusterAgentMCPClient_Integration(t *testing.T) {
	t.Skip("Integration test - requires mock MCP server")

	// Create a test HTTP server that simulates cluster-agent MCP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock MCP server responses would go here
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := ClusterAgentMCPClientConfig{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	}

	client, err := NewClusterAgentMCPClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// This would fail without a proper MCP server implementation
	err = client.Connect(ctx)
	if err != nil {
		t.Logf("Connect failed as expected without proper MCP server: %v", err)
	}
}
