// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// createMCPServer creates and configures the MCP server with all registered tools.
func (o *options) createMCPServer() *mcp.Server {
	// Create server with implementation metadata
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "kubectl-datadog-mcp",
		Version: "1.0.0",
	}, nil)

	// Register all tool handlers
	o.registerListAgentsTool(server)
	o.registerGetAgentStatusTool(server)
	o.registerDescribeAgentFeaturesTool(server)
	o.registerDescribeAgentComponentsTool(server)
	o.registerGetClusterAgentLeaderTool(server)

	return server
}
