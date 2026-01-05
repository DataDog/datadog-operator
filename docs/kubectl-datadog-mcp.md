# kubectl-datadog MCP Server

## Overview

The `kubectl datadog mcp` command starts an MCP (Model Context Protocol) server that provides read-only access to DatadogAgent resources in your Kubernetes cluster. The server integrates with AI assistants like Claude Desktop, allowing them to query and understand your Datadog Agent deployments.

The MCP server provides two types of tools:
- **Local tools**: Query DatadogAgent resources from the Kubernetes API
- **Proxy tools**: Automatically discovered and forwarded from the cluster-agent's MCP server for runtime diagnostics

## What is MCP?

The Model Context Protocol (MCP) is an open standard that enables AI assistants to securely connect to external data sources and tools. The `kubectl-datadog` MCP server exposes DatadogAgent resources through a standardized interface that AI assistants can query.

## Features

- **Read-only access**: All operations are safe GET/LIST operations
- **Namespace-aware**: Respects your kubeconfig context and namespace settings
- **Auto-selection**: Most tools automatically select a DatadogAgent when name is omitted (convenient for single-agent clusters)
- **Structured output**: Returns JSON data optimized for AI assistant consumption
- **Local tools**: 5 specialized tools for querying DatadogAgent resources
- **Cluster-agent proxy**: Automatically discovers and exposes cluster-agent MCP tools for runtime diagnostics
- **Graceful degradation**: Local tools continue to work even if cluster-agent proxy is unavailable

## Installation

The MCP server is built into the `kubectl-datadog` plugin. No additional installation is required beyond having the kubectl-datadog plugin available in your PATH.

### Prerequisites

**Required**:
- `kubectl-datadog` plugin installed
- Valid kubeconfig with access to DatadogAgent resources
- Kubernetes cluster with Datadog Operator installed

**Optional (for cluster-agent proxy)**:
- Cluster-agent deployed and running
- Cluster-agent MCP server enabled (`cluster_agent.mcp.enabled=true`)
- RBAC permissions for `pods/list`, `pods/portforward`, and `leases/get` or `configmaps/get`

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

### Configuring the Cluster-Agent Proxy

The cluster-agent proxy is enabled by default and requires no configuration in most cases. However, you can customize its behavior using command-line flags:

**Disable the proxy**:
```bash
kubectl datadog mcp --proxy-cluster-agent=false
```

**Specify a DatadogAgent name** (useful when multiple DatadogAgents exist):
```bash
kubectl datadog mcp --proxy-dda-name=my-datadog-agent
```

**Customize the cluster-agent MCP port** (if using a non-standard configuration):
```bash
kubectl datadog mcp --proxy-port=5000
```

**Customize the cluster-agent MCP endpoint**:
```bash
kubectl datadog mcp --proxy-endpoint=/mcp
```

**Available flags**:
- `--proxy-cluster-agent`: Enable/disable proxy (default: true)
- `--proxy-dda-name`: DatadogAgent name to proxy (default: auto-select first)
- `--proxy-port`: Cluster-agent MCP server port (default: 5000)
- `--proxy-endpoint`: Cluster-agent MCP endpoint path (default: /mcp)

### Debug Logging

To enable debug logging for troubleshooting:

```bash
kubectl datadog mcp --debug
```

Debug logs are written to stderr and include:
- MCP server lifecycle events
- Proxy initialization steps
- Tool discovery and registration
- Port-forward connection status

In Claude Desktop configuration:
```json
{
  "mcpServers": {
    "kubectl-datadog": {
      "command": "kubectl",
      "args": ["datadog", "mcp", "--debug"]
    }
  }
}
```

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

To disable the cluster-agent proxy (local tools only):

```json
{
  "mcpServers": {
    "kubectl-datadog": {
      "command": "kubectl",
      "args": ["datadog", "mcp", "--proxy-cluster-agent=false"]
    }
  }
}
```

To specify a particular DatadogAgent for proxy:

```json
{
  "mcpServers": {
    "kubectl-datadog": {
      "command": "kubectl",
      "args": ["datadog", "mcp", "--proxy-dda-name", "my-datadog-agent"]
    }
  }
}
```

After updating the configuration, restart Claude Desktop for the changes to take effect.

## Available Tools

### Local Tools

The MCP server exposes 5 local tools that query DatadogAgent resources from the Kubernetes API.

**Auto-selection**: Tools that query a specific DatadogAgent (tools 2-5) support auto-selection. When the `name` parameter is omitted, the tool automatically selects the first DatadogAgent found in the namespace. This is convenient for clusters with a single DatadogAgent. If multiple agents exist, the first one is chosen and a warning is included in the error message.

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

### 2. get_datadog_agent_status

**Purpose**: Get runtime status information for a DatadogAgent deployment.

**Parameters**:
- `name` (optional): Name of the DatadogAgent resource. If not specified, auto-selects the first DatadogAgent found in the namespace.
- `namespace` (optional): Kubernetes namespace. Empty means current namespace.

**Returns**: Detailed status for all components:
- Agent DaemonSet: desired, current, ready, available, up-to-date counts
- Cluster Agent: replica counts, deployment name
- Cluster Checks Runner: replica counts, deployment name
- Conditions: reconciliation status

**Auto-selection**: When `name` is omitted, the tool automatically selects the first DatadogAgent in the namespace. If multiple agents exist, the first one is chosen and a warning is included.

**Example prompts for Claude**:
> "What's the status of my datadog-agent deployment?"
> "Show me the DatadogAgent status" (uses auto-selection)

### 3. describe_datadog_agent_features

**Purpose**: View enabled features and their configuration.

**Parameters**:
- `name` (optional): Name of the DatadogAgent resource. If not specified, auto-selects the first DatadogAgent found in the namespace.
- `namespace` (optional): Kubernetes namespace. Empty means current namespace.

**Returns**: Feature configuration including:
- APM settings
- Log collection settings
- Network Performance Monitoring (NPM)
- Security features (CSPM, CWS, etc.)
- Process monitoring
- And 40+ other monitoring features

**Auto-selection**: When `name` is omitted, the tool automatically selects the first DatadogAgent in the namespace. If multiple agents exist, the first one is chosen and a warning is included.

**Example prompts for Claude**:
> "What monitoring features are enabled on datadog-agent?"
> "Show me the enabled features" (uses auto-selection)

### 4. describe_datadog_agent_components

**Purpose**: Get component overrides and global configuration.

**Parameters**:
- `name` (optional): Name of the DatadogAgent resource. If not specified, auto-selects the first DatadogAgent found in the namespace.
- `namespace` (optional): Kubernetes namespace. Empty means current namespace.

**Returns**: Component customizations including:
- NodeAgent (DaemonSet) overrides
- ClusterAgent (Deployment) overrides
- ClusterChecksRunner (Deployment) overrides
- Global configuration settings
- Container resource limits
- Environment variables
- Volume mounts

**Auto-selection**: When `name` is omitted, the tool automatically selects the first DatadogAgent in the namespace. If multiple agents exist, the first one is chosen and a warning is included.

**Example prompts for Claude**:
> "Show me the component overrides for datadog-agent"
> "What component customizations are configured?" (uses auto-selection)

### 5. get_cluster_agent_leader

**Purpose**: Get the cluster-agent leader pod name for a DatadogAgent.

**Parameters**:
- `name` (optional): Name of the DatadogAgent resource. If not specified, auto-selects the first DatadogAgent found in the namespace.
- `namespace` (optional): Kubernetes namespace. Empty means current namespace.

**Returns**: Leader election information including:
- DatadogAgent name
- Namespace
- Leader pod name
- Election method (Lease or ConfigMap)

**Auto-selection**: When `name` is omitted, the tool automatically selects the first DatadogAgent in the namespace. If multiple agents exist, the first one is chosen and a warning is included.

**Example prompts for Claude**:
> "Which cluster-agent pod is the leader?"
> "Show me the cluster-agent leader for datadog-agent"
> "What's the leader election method being used?" (uses auto-selection)

### Cluster-Agent Proxy Tools

The MCP server automatically discovers and proxies tools from the cluster-agent's MCP server (if enabled). These tools provide runtime diagnostics and operational insights directly from the running cluster-agent.

**How it works**:
1. The kubectl-datadog MCP server discovers the cluster-agent leader pod
2. Establishes a port-forward connection to the cluster-agent's MCP server (port 5000)
3. Fetches the list of available tools from the cluster-agent
4. Registers proxy handlers with the `cluster_agent_` prefix to avoid conflicts

**Tool naming**: All cluster-agent tools are prefixed with `cluster_agent_`. For example, if the cluster-agent exposes a `get_leader` tool, it becomes `cluster_agent_get_leader`.

**Configuration**: Cluster-agent proxy is enabled by default. You can disable it with `--proxy-cluster-agent=false`.

**Requirements**:
- Cluster-agent must be deployed and running
- Cluster-agent MCP server must be enabled (`cluster_agent.mcp.enabled=true` in datadog-agent configuration)
- kubectl-datadog must have permissions to list pods and create port-forwards

**Example cluster-agent tools** (availability depends on cluster-agent version):
- `cluster_agent_get_leader`: Get cluster-agent leader election information
- Additional tools may be added by the cluster-agent in future versions

**Graceful degradation**: If the cluster-agent proxy fails to initialize (e.g., cluster-agent not found, MCP server not enabled), a warning is logged and only local tools remain available. This ensures the MCP server continues to function for DatadogAgent resource queries.

## Example Interactions

Here are some example interactions you can have with Claude once the MCP server is configured:

### Local Tool Interactions

**Troubleshooting deployment issues**:
> "My datadog-agent pods aren't starting. Can you check the status and tell me what might be wrong?"

**Understanding configuration**:
> "I want to enable APM. Is it currently enabled on my datadog-agent? If not, what would I need to add to the configuration?"

**Comparing configurations**:
> "List all DatadogAgent resources and compare their feature configurations"

**Capacity planning**:
> "What resource limits are set on the datadog-agent components? Are they appropriate for a large cluster?"

### Cluster-Agent Proxy Tool Interactions

**Leader election diagnostics**:
> "Which cluster-agent pod is currently the leader? Use the cluster_agent_get_leader tool."

**Runtime diagnostics** (tools vary by cluster-agent version):
> "Can you check the cluster-agent runtime status using the available cluster-agent tools?"

**Combined queries**:
> "Show me the DatadogAgent configuration and then check which cluster-agent pod is the leader"

## Security Considerations

- **Read-only**: The MCP server only performs GET and LIST operations. It cannot modify DatadogAgent resources.
- **RBAC**: The server uses your kubeconfig credentials and respects Kubernetes RBAC. Ensure your kubeconfig has appropriate read permissions.
- **Local only**: The stdio transport means the server only accepts connections from processes on the same machine (like Claude Desktop).
- **No authentication**: The server does not implement additional authentication beyond what kubectl provides.
- **Port-forward security**: The cluster-agent proxy uses Kubernetes port-forward, which creates a secure tunnel authenticated by your kubeconfig. The connection stays within your local machine and does not expose cluster-agent ports externally.
- **Proxy permissions**: The proxy requires additional RBAC permissions:
  - `pods/list`: To discover cluster-agent pods
  - `pods/portforward`: To establish port-forward connections
  - `leases/get` or `configmaps/get`: For leader election detection
- **Tool isolation**: Cluster-agent tools are prefixed with `cluster_agent_` to prevent naming conflicts with local tools.

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

### "Warning: Failed to register cluster-agent proxy tools"

This warning appears when the cluster-agent proxy cannot be initialized. This is not a critical error - local tools will continue to work. Common causes:

1. **Cluster-agent not running**: Verify cluster-agent pods exist:
   ```bash
   kubectl get pods -l agent.datadoghq.com/component=cluster-agent
   ```

2. **Cluster-agent MCP server not enabled**: Check if the cluster-agent has MCP enabled:
   ```bash
   kubectl logs -l agent.datadoghq.com/component=cluster-agent | grep -i mcp
   ```
   The cluster-agent must be configured with `cluster_agent.mcp.enabled=true`.

3. **Multiple DatadogAgents exist**: Specify which one to use:
   ```bash
   kubectl datadog mcp --proxy-dda-name=your-datadog-agent
   ```

4. **Port-forward permissions**: Verify you have permissions to create port-forwards:
   ```bash
   kubectl auth can-i create pods/portforward
   ```

5. **Cluster-agent not on port 5000**: If using a custom port, specify it:
   ```bash
   kubectl datadog mcp --proxy-port=<custom-port>
   ```

To disable the proxy and suppress warnings:
```bash
kubectl datadog mcp --proxy-cluster-agent=false
```

### Cluster-agent proxy tools not appearing

1. **Check proxy is enabled**: Ensure `--proxy-cluster-agent=true` (default)
2. **Verify cluster-agent MCP server**: The cluster-agent must have its MCP server enabled
3. **Check DatadogAgent version**: Older versions may not support the MCP server
4. **Review startup messages**: The MCP server logs "Registered N cluster-agent proxy tools" on successful initialization

## Technical Details

### Transport

The MCP server uses **stdio transport** (stdin/stdout) for communication. This is the standard transport for local MCP servers and is compatible with Claude Desktop and other MCP clients.

### Protocol

The server implements the Model Context Protocol specification, using JSON-RPC 2.0 for message framing. Messages are newline-delimited JSON.

### Data Format

All tool responses return structured JSON data. The MCP SDK automatically serializes Go types to JSON schemas that AI assistants can understand.

### Cluster-Agent Proxy Architecture

The cluster-agent proxy uses the following architecture:

```
┌─────────────────┐
│  Claude Desktop │
│   (MCP Client)  │
└────────┬────────┘
         │ stdio (JSON-RPC 2.0)
         ▼
┌─────────────────────────────────────────────────────────┐
│  kubectl-datadog MCP Server (stdio transport)           │
│  ┌───────────────────────────────────────────────────┐  │
│  │ Local Tools (5 tools)                             │  │
│  │ • list_datadog_agents                             │  │
│  │ • get_datadog_agent_status (auto-select)          │  │
│  │ • describe_datadog_agent_features (auto-select).  │  │
│  │ • describe_datadog_agent_components (auto-select) │  │
│  │ • get_cluster_agent_leader (auto-select)          │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ Proxy Manager                                     │  │
│  │ • Discovery: Find cluster-agent leader pod        │  │
│  │ • Port-forward: SPDY to pod port 5000             │  │
│  │ • MCP Client: HTTP streaming to /mcp              │  │
│  │ • Tool registration: Prefix with cluster_agent_.  │  │
│  └──────────────┬────────────────────────────────────┘  │
└─────────────────┼───────────────────────────────────────┘
                  │ Port-forward (SPDY)
                  │ HTTP POST to /mcp
                  ▼
┌──────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                  │
│  ┌────────────────────────────────────────────────┐  │
│  │  Cluster-Agent Leader Pod                      │  │
│  │  ┌──────────────────────────────────────────┐  │  │
│  │  │  Cluster-Agent MCP Server                │  │  │
│  │  │  • Port: 5000 (metrics port)             │  │  │
│  │  │  • Endpoint: /mcp                        │  │  │
│  │  │  • Transport: HTTP with streaming        │  │  │
│  │  │  • Config: cluster_agent.mcp.enabled     │  │  │
│  │  └──────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

**Proxy initialization flow**:
1. **Discovery**: Find DatadogAgent resource (auto-select if not specified)
2. **Leader detection**: Identify cluster-agent leader pod using Lease resources or ConfigMap annotations
3. **Port-forward**: Establish SPDY connection to pod port 5000 using Kubernetes port-forward API
4. **MCP connection**: Connect to cluster-agent's HTTP MCP server at `/mcp` endpoint
5. **Tool discovery**: Call `tools/list` to fetch available tools from cluster-agent
6. **Registration**: Register proxy handlers for each tool with `cluster_agent_` prefix
7. **Request forwarding**: Forward tool calls to cluster-agent and return responses

**Graceful degradation**: If any step fails, the error is logged as a warning and the server continues with only local tools available.

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
