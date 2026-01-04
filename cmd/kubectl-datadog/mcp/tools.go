// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	coordv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

// getK8sSchemaOptions returns ForOptions with custom schemas for Kubernetes types.
// This is needed because k8s types like metav1.Time marshal to strings but the schema
// generator sees them as objects.
func getK8sSchemaOptions() *jsonschema.ForOptions {
	return &jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			// metav1.Time marshals to RFC3339 string
			reflect.TypeFor[metav1.Time](): {
				Type:   "string",
				Format: "date-time",
			},
			// metav1.MicroTime marshals to RFC3339 string with microseconds
			reflect.TypeFor[metav1.MicroTime](): {
				Type:   "string",
				Format: "date-time",
			},
		},
	}
}

// registerListAgentsTool registers the list_datadog_agents tool.
func (o *options) registerListAgentsTool(server *mcp.Server) {
	// Generate output schema with custom K8s type schemas
	outputSchema, err := jsonschema.For[ListAgentsOutput](getK8sSchemaOptions())
	if err != nil {
		panic("failed to generate schema for ListAgentsOutput: " + err.Error())
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:         "list_datadog_agents",
		Description:  "List DatadogAgent resources in the specified namespace or across all namespaces. Returns a summary of each agent including name, namespace, component statuses, and age.",
		OutputSchema: outputSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ListAgentsArgs) (*mcp.CallToolResult, ListAgentsOutput, error) {
		// Determine namespace to use
		namespace := args.Namespace
		if namespace == "" && !args.AllNamespaces {
			namespace = o.UserNamespace
		}

		// List DatadogAgents
		ddList := &v2alpha1.DatadogAgentList{}
		listOpts := &client.ListOptions{}

		if !args.AllNamespaces {
			listOpts.Namespace = namespace
		}

		if err := o.Client.List(ctx, ddList, listOpts); err != nil {
			//nolint:nilerr // MCP SDK pattern: tool errors are returned in CallToolResult, not as Go errors
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: "Failed to list DatadogAgents: " + err.Error(),
					},
				},
			}, ListAgentsOutput{}, nil
		}

		// Build output
		output := ListAgentsOutput{
			Agents: make([]AgentSummary, 0, len(ddList.Items)),
			Count:  len(ddList.Items),
		}

		for i := range ddList.Items {
			agent := &ddList.Items[i]
			summary := AgentSummary{
				Name:      agent.Name,
				Namespace: agent.Namespace,
				Age:       common.GetDurationAsString(agent),
			}

			if agent.Status.Agent != nil {
				summary.AgentStatus = agent.Status.Agent.Status
			}
			if agent.Status.ClusterAgent != nil {
				summary.ClusterAgentStatus = agent.Status.ClusterAgent.Status
			}
			if agent.Status.ClusterChecksRunner != nil {
				summary.ClusterChecksRunnerStatus = agent.Status.ClusterChecksRunner.Status
			}

			output.Agents = append(output.Agents, summary)
		}

		return nil, output, nil
	})
}

// AgentStatusOutput contains status information for a DatadogAgent.
type AgentStatusOutput struct {
	Name                string                     `json:"name"`
	Namespace           string                     `json:"namespace"`
	Conditions          []metav1.Condition         `json:"conditions,omitempty"`
	Agent               *v2alpha1.DaemonSetStatus  `json:"agent,omitempty"`
	ClusterAgent        *v2alpha1.DeploymentStatus `json:"clusterAgent,omitempty"`
	ClusterChecksRunner *v2alpha1.DeploymentStatus `json:"clusterChecksRunner,omitempty"`
}

// registerGetAgentStatusTool registers the get_datadog_agent_status tool.
func (o *options) registerGetAgentStatusTool(server *mcp.Server) {
	// Generate output schema with custom K8s type schemas
	outputSchema, err := jsonschema.For[AgentStatusOutput](getK8sSchemaOptions())
	if err != nil {
		panic("failed to generate schema for AgentStatusOutput: " + err.Error())
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:         "get_datadog_agent_status",
		Description:  "Get runtime status information for a DatadogAgent deployment. Returns detailed status for all components (Agent DaemonSet, Cluster Agent, Cluster Checks Runner) including replica counts, readiness, and conditions.",
		OutputSchema: outputSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetAgentStatusArgs) (*mcp.CallToolResult, AgentStatusOutput, error) {
		namespace := args.Namespace
		if namespace == "" {
			namespace = o.UserNamespace
		}

		agent := &v2alpha1.DatadogAgent{}
		key := client.ObjectKey{
			Namespace: namespace,
			Name:      args.Name,
		}

		if err := o.Client.Get(ctx, key, agent); err != nil {
			//nolint:nilerr // MCP SDK pattern: tool errors are returned in CallToolResult, not as Go errors
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: "Failed to get DatadogAgent: " + err.Error(),
					},
				},
			}, AgentStatusOutput{}, nil
		}

		// Extract status information
		statusInfo := AgentStatusOutput{
			Name:                agent.Name,
			Namespace:           agent.Namespace,
			Conditions:          agent.Status.Conditions,
			Agent:               agent.Status.Agent,
			ClusterAgent:        agent.Status.ClusterAgent,
			ClusterChecksRunner: agent.Status.ClusterChecksRunner,
		}

		return nil, statusInfo, nil
	})
}

// AgentFeaturesOutput contains feature configuration for a DatadogAgent.
type AgentFeaturesOutput struct {
	Name      string                    `json:"name"`
	Namespace string                    `json:"namespace"`
	Features  *v2alpha1.DatadogFeatures `json:"features,omitempty"`
}

// registerDescribeAgentFeaturesTool registers the describe_datadog_agent_features tool.
func (o *options) registerDescribeAgentFeaturesTool(server *mcp.Server) {
	// Generate output schema with custom K8s type schemas
	outputSchema, err := jsonschema.For[AgentFeaturesOutput](getK8sSchemaOptions())
	if err != nil {
		panic("failed to generate schema for AgentFeaturesOutput: " + err.Error())
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:         "describe_datadog_agent_features",
		Description:  "Get the enabled features and their configuration for a DatadogAgent. Shows which monitoring features are active (APM, Logs, NPM, Security, etc.) and their specific settings.",
		OutputSchema: outputSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DescribeAgentFeaturesArgs) (*mcp.CallToolResult, AgentFeaturesOutput, error) {
		namespace := args.Namespace
		if namespace == "" {
			namespace = o.UserNamespace
		}

		agent := &v2alpha1.DatadogAgent{}
		key := client.ObjectKey{
			Namespace: namespace,
			Name:      args.Name,
		}

		if err := o.Client.Get(ctx, key, agent); err != nil {
			//nolint:nilerr // MCP SDK pattern: tool errors are returned in CallToolResult, not as Go errors
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: "Failed to get DatadogAgent: " + err.Error(),
					},
				},
			}, AgentFeaturesOutput{}, nil
		}

		// Extract feature configuration
		featuresInfo := AgentFeaturesOutput{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Features:  agent.Spec.Features,
		}

		return nil, featuresInfo, nil
	})
}

// AgentComponentsOutput contains component configuration for a DatadogAgent.
type AgentComponentsOutput struct {
	Name         string                                                             `json:"name"`
	Namespace    string                                                             `json:"namespace"`
	Overrides    map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride `json:"overrides,omitempty"`
	GlobalConfig *v2alpha1.GlobalConfig                                             `json:"globalConfig,omitempty"`
}

// registerDescribeAgentComponentsTool registers the describe_datadog_agent_components tool.
func (o *options) registerDescribeAgentComponentsTool(server *mcp.Server) {
	// Generate output schema with custom K8s type schemas
	outputSchema, err := jsonschema.For[AgentComponentsOutput](getK8sSchemaOptions())
	if err != nil {
		panic("failed to generate schema for AgentComponentsOutput: " + err.Error())
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:         "describe_datadog_agent_components",
		Description:  "Get component overrides and global configuration for a DatadogAgent. Shows customizations for NodeAgent, ClusterAgent, and ClusterChecksRunner components including container overrides, resource limits, and environment variables.",
		OutputSchema: outputSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DescribeAgentComponentsArgs) (*mcp.CallToolResult, AgentComponentsOutput, error) {
		namespace := args.Namespace
		if namespace == "" {
			namespace = o.UserNamespace
		}

		agent := &v2alpha1.DatadogAgent{}
		key := client.ObjectKey{
			Namespace: namespace,
			Name:      args.Name,
		}

		if err := o.Client.Get(ctx, key, agent); err != nil {
			//nolint:nilerr // MCP SDK pattern: tool errors are returned in CallToolResult, not as Go errors
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: "Failed to get DatadogAgent: " + err.Error(),
					},
				},
			}, AgentComponentsOutput{}, nil
		}

		// Extract component configuration
		componentsInfo := AgentComponentsOutput{
			Name:         agent.Name,
			Namespace:    agent.Namespace,
			Overrides:    agent.Spec.Override,
			GlobalConfig: agent.Spec.Global,
		}

		return nil, componentsInfo, nil
	})
}

// leaderResponse is used for unmarshaling ConfigMap leader annotation
type leaderResponse struct {
	HolderIdentity string `json:"holderIdentity"`
}

// registerGetClusterAgentLeaderTool registers the get_cluster_agent_leader tool.
func (o *options) registerGetClusterAgentLeaderTool(server *mcp.Server) {
	// Generate output schema with custom K8s type schemas
	outputSchema, err := jsonschema.For[ClusterAgentLeaderOutput](getK8sSchemaOptions())
	if err != nil {
		panic("failed to generate schema for ClusterAgentLeaderOutput: " + err.Error())
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:         "get_cluster_agent_leader",
		Description:  "Get the cluster-agent leader pod name for a DatadogAgent. Returns the pod name that currently holds the leader election lock and the election method used (Lease or ConfigMap).",
		OutputSchema: outputSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetClusterAgentLeaderArgs) (*mcp.CallToolResult, ClusterAgentLeaderOutput, error) {
		namespace := args.Namespace
		if namespace == "" {
			namespace = o.UserNamespace
		}

		// Leader object name is {DatadogAgentName}-leader-election
		leaderObjName := fmt.Sprintf("%s-leader-election", args.Name)
		objKey := client.ObjectKey{Namespace: namespace, Name: leaderObjName}

		var leaderName string
		var electionMethod string

		// Check if Lease is supported
		useLease, err := o.isLeaseSupported()
		if err != nil {
			//nolint:nilerr // MCP SDK pattern: tool errors are returned in CallToolResult, not as Go errors
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf("Failed to check if Lease is supported: %s", err.Error()),
					},
				},
			}, ClusterAgentLeaderOutput{}, nil
		}

		// Try to get leader from Lease if supported
		if useLease {
			leaderName, err = o.getLeaderFromLease(ctx, objKey)
			if err == nil {
				electionMethod = "Lease"
			}
		}

		// Fall back to ConfigMap if Lease is not supported or failed
		if !useLease || err != nil {
			leaderName, err = o.getLeaderFromConfigMap(ctx, objKey)
			if err != nil {
				//nolint:nilerr // MCP SDK pattern: tool errors are returned in CallToolResult, not as Go errors
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{
							Text: fmt.Sprintf("Failed to get cluster-agent leader: %s", err.Error()),
						},
					},
				}, ClusterAgentLeaderOutput{}, nil
			}
			electionMethod = "ConfigMap"
		}

		output := ClusterAgentLeaderOutput{
			Name:           args.Name,
			Namespace:      namespace,
			LeaderPodName:  leaderName,
			ElectionMethod: electionMethod,
		}

		return nil, output, nil
	})
}

// getLeaderFromLease retrieves the leader identity from a Lease object
func (o *options) getLeaderFromLease(ctx context.Context, objKey client.ObjectKey) (string, error) {
	lease := &coordv1.Lease{}
	err := o.Client.Get(ctx, objKey, lease)
	if err != nil && apierrors.IsNotFound(err) {
		return "", fmt.Errorf("lease %s/%s not found", objKey.Namespace, objKey.Name)
	} else if err != nil {
		return "", fmt.Errorf("unable to get leader election lease: %w", err)
	}

	// Get the info from the lease
	if lease.Spec.HolderIdentity == nil {
		return "", fmt.Errorf("lease %s/%s does not have a holder identity", objKey.Namespace, objKey.Name)
	}

	return *lease.Spec.HolderIdentity, nil
}

// getLeaderFromConfigMap retrieves the leader identity from a ConfigMap annotation
func (o *options) getLeaderFromConfigMap(ctx context.Context, objKey client.ObjectKey) (string, error) {
	// Get the config map holding the leader identity
	cm := &corev1.ConfigMap{}
	err := o.Client.Get(ctx, objKey, cm)
	if err != nil && apierrors.IsNotFound(err) {
		return "", fmt.Errorf("config map %s/%s not found", objKey.Namespace, objKey.Name)
	} else if err != nil {
		return "", fmt.Errorf("unable to get leader election config map: %w", err)
	}

	// Get leader from annotations
	annotations := cm.GetAnnotations()
	leaderInfo, found := annotations["control-plane.alpha.kubernetes.io/leader"]
	if !found {
		return "", fmt.Errorf("couldn't find leader annotation on %s/%s config map", objKey.Namespace, objKey.Name)
	}

	resp := leaderResponse{}
	if err := json.Unmarshal([]byte(leaderInfo), &resp); err != nil {
		return "", fmt.Errorf("couldn't unmarshal leader annotation: %w", err)
	}

	return resp.HolderIdentity, nil
}

// isLeaseSupported checks if the Kubernetes cluster supports Lease resources
func (o *options) isLeaseSupported() (bool, error) {
	apiGroupList, err := o.DiscoveryClient.ServerGroups()
	if err != nil {
		return false, fmt.Errorf("unable to discover APIGroups, err:%w", err)
	}

	groupVersions := metav1.ExtractGroupVersions(apiGroupList)
	for _, grv := range groupVersions {
		if grv == "coordination.k8s.io/v1" || grv == "coordination.k8s.io/v1beta1" {
			return true, nil
		}
	}

	return false, nil
}
