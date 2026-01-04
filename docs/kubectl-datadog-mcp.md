# kubectl-datadog MCP Server

## Overview

The `kubectl datadog mcp` command starts an MCP (Model Context Protocol) server that provides read-only access to DatadogAgent resources in your Kubernetes cluster. The server integrates with AI assistants like Claude Desktop, allowing them to query and understand your Datadog Agent deployments.

## What is MCP?

The Model Context Protocol (MCP) is an open standard that enables AI assistants to securely connect to external data sources and tools. The `kubectl-datadog` MCP server exposes DatadogAgent resources through a standardized interface that AI assistants can query.

## Features

- **Read-only access**: All operations are safe GET/LIST operations
- **Namespace-aware**: Respects your kubeconfig context and namespace settings
- **Structured output**: Returns JSON data optimized for AI assistant consumption
- **5 specialized tools**: Each tool has a specific, well-defined purpose

## Installation

The MCP server is built into the `kubectl-datadog` plugin. No additional installation is required beyond having the kubectl-datadog plugin available in your PATH.

### Prerequisites

- `kubectl-datadog` plugin installed
- Valid kubeconfig with access to DatadogAgent resources
- Kubernetes cluster with Datadog Operator installed

## Usage

### Starting the MCP Server

To start the MCP server, simply run:

```bash
kubectl datadog mcp
```

The server will start and communicate over stdin/stdout using the MCP protocol. It runs until the client disconnects.

### Specifying a Namespace

You can specify a namespace using the standard kubectl flags:

```bash
kubectl datadog mcp --namespace datadog
```

Or use your kubeconfig's current namespace (default behavior).

## Configuration with Claude Desktop

To use the MCP server with Claude Desktop, add the following to your Claude Desktop configuration file:

**macOS/Linux**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "kubectl-datadog": {
      "command": "kubectl",
      "args": ["datadog", "mcp"]
    }
  }
}
```

If you want to target a specific namespace:

```json
{
  "mcpServers": {
    "kubectl-datadog": {
      "command": "kubectl",
      "args": ["datadog", "mcp", "--namespace", "datadog"]
    }
  }
}
```

After updating the configuration, restart Claude Desktop for the changes to take effect.

## Available Tools

The MCP server exposes 5 tools that AI assistants can use to query DatadogAgent resources:

### 1. list_datadog_agents

**Purpose**: List all DatadogAgent resources in a namespace or across all namespaces.

**Parameters**:
- `namespace` (optional): Kubernetes namespace to list agents from. Empty means current namespace.
- `allNamespaces` (optional): If true, list agents from all namespaces.

**Returns**: Summary of each agent including:
- Name and namespace
- Agent DaemonSet status
- Cluster Agent status
- Cluster Checks Runner status
- Age

**Example prompt for Claude**:
> "List all DatadogAgent resources in my cluster"

### 2. get_datadog_agent

**Purpose**: Get the complete configuration of a specific DatadogAgent resource.

**Parameters**:
- `name` (required): Name of the DatadogAgent resource
- `namespace` (optional): Kubernetes namespace. Empty means current namespace.

**Returns**: Full DatadogAgent CRD including:
- Global configuration
- Feature settings
- Component overrides
- Status information

**Example prompt for Claude**:
> "Show me the full configuration of the datadog-agent resource"

### 3. get_datadog_agent_status

**Purpose**: Get runtime status information for a DatadogAgent deployment.

**Parameters**:
- `name` (required): Name of the DatadogAgent resource
- `namespace` (optional): Kubernetes namespace. Empty means current namespace.

**Returns**: Detailed status for all components:
- Agent DaemonSet: desired, current, ready, available, up-to-date counts
- Cluster Agent: replica counts, deployment name
- Cluster Checks Runner: replica counts, deployment name
- Conditions: reconciliation status

**Example prompt for Claude**:
> "What's the status of my datadog-agent deployment?"

### 4. describe_datadog_agent_features

**Purpose**: View enabled features and their configuration.

**Parameters**:
- `name` (required): Name of the DatadogAgent resource
- `namespace` (optional): Kubernetes namespace. Empty means current namespace.

**Returns**: Feature configuration including:
- APM settings
- Log collection settings
- Network Performance Monitoring (NPM)
- Security features (CSPM, CWS, etc.)
- Process monitoring
- And 40+ other monitoring features

**Example prompt for Claude**:
> "What monitoring features are enabled on datadog-agent?"

### 5. describe_datadog_agent_components

**Purpose**: Get component overrides and global configuration.

**Parameters**:
- `name` (required): Name of the DatadogAgent resource
- `namespace` (optional): Kubernetes namespace. Empty means current namespace.

**Returns**: Component customizations including:
- NodeAgent (DaemonSet) overrides
- ClusterAgent (Deployment) overrides
- ClusterChecksRunner (Deployment) overrides
- Global configuration settings
- Container resource limits
- Environment variables
- Volume mounts

**Example prompt for Claude**:
> "Show me the component overrides for datadog-agent"

## Example Interactions

Here are some example interactions you can have with Claude once the MCP server is configured:

**Troubleshooting deployment issues**:
> "My datadog-agent pods aren't starting. Can you check the status and tell me what might be wrong?"

**Understanding configuration**:
> "I want to enable APM. Is it currently enabled on my datadog-agent? If not, what would I need to add to the configuration?"

**Comparing configurations**:
> "List all DatadogAgent resources and compare their feature configurations"

**Capacity planning**:
> "What resource limits are set on the datadog-agent components? Are they appropriate for a large cluster?"

## Security Considerations

- **Read-only**: The MCP server only performs GET and LIST operations. It cannot modify DatadogAgent resources.
- **RBAC**: The server uses your kubeconfig credentials and respects Kubernetes RBAC. Ensure your kubeconfig has appropriate read permissions.
- **Local only**: The stdio transport means the server only accepts connections from processes on the same machine (like Claude Desktop).
- **No authentication**: The server does not implement additional authentication beyond what kubectl provides.

## Troubleshooting

### MCP server not appearing in Claude Desktop

1. Verify your `claude_desktop_config.json` syntax is valid JSON
2. Ensure the `kubectl` command is in your PATH
3. Restart Claude Desktop after configuration changes
4. Check Claude Desktop logs for error messages

### "Failed to list DatadogAgents" errors

1. Verify your kubeconfig is valid: `kubectl config view`
2. Check you have permissions: `kubectl auth can-i list datadogagents`
3. Verify the Datadog Operator CRDs are installed: `kubectl get crd datadogagents.datadoghq.com`
4. Check the namespace exists and you have access

### Server disconnects immediately

1. Ensure kubectl-datadog is properly installed: `kubectl datadog --help`
2. Try running the command manually: `kubectl datadog mcp` (press Ctrl+C to exit)
3. Check for Go module errors or missing dependencies

### Claude doesn't seem to see my DatadogAgents

1. Verify agents exist: `kubectl get datadogagents -A`
2. Check the namespace Claude is querying (it uses your kubeconfig context)
3. Try explicitly specifying `allNamespaces: true` in your query

## Technical Details

### Transport

The MCP server uses **stdio transport** (stdin/stdout) for communication. This is the standard transport for local MCP servers and is compatible with Claude Desktop and other MCP clients.

### Protocol

The server implements the Model Context Protocol specification, using JSON-RPC 2.0 for message framing. Messages are newline-delimited JSON.

### Data Format

All tool responses return structured JSON data. The MCP SDK automatically serializes Go types to JSON schemas that AI assistants can understand.

## Development

### Running Tests

```bash
go test ./cmd/kubectl-datadog/mcp/...
```

### Building

```bash
make kubectl-datadog
```

### Debugging

To see the MCP protocol messages, you can use the MCP Inspector:

```bash
npx @modelcontextprotocol/inspector kubectl datadog mcp
```

This opens a web UI where you can interactively test the MCP tools.

## Related Documentation

- [Model Context Protocol Specification](https://github.com/modelcontextprotocol)
- [DatadogAgent CRD Documentation](configuration.v2alpha1.md)
- [kubectl-datadog Plugin Documentation](kubectl-datadog.md)
- [Datadog Operator Documentation](../README.md)

## Support

For issues or questions:
- File an issue: https://github.com/DataDog/datadog-operator/issues
- Datadog Community Slack: #kubernetes

## License

Apache License 2.0 - see LICENSE file for details.
