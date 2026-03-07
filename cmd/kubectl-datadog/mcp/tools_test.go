// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// setupTestOptions creates a test options struct with fake clients
func setupTestOptions(objects ...client.Object) *options {
	scheme := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	clientBuilder := ctrlruntimefake.NewClientBuilder().WithScheme(scheme)
	if len(objects) > 0 {
		clientBuilder = clientBuilder.WithObjects(objects...)
	}

	c := clientBuilder.Build()
	clientset := fake.NewSimpleClientset()

	opts := &options{
		IOStreams: genericclioptions.IOStreams{},
	}
	opts.Client = c
	opts.Clientset = clientset
	opts.DiscoveryClient = clientset.Discovery()
	opts.UserNamespace = "test-namespace"

	return opts
}

// createTestServer creates a test MCP server
func createTestServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, nil)
}

func TestGetK8sSchemaOptions(t *testing.T) {
	opts := getK8sSchemaOptions()

	if opts == nil {
		t.Fatal("Expected schema options to be returned")
	}

	if opts.TypeSchemas == nil {
		t.Fatal("Expected TypeSchemas to be set")
	}

	// Check that metav1.Time is mapped correctly
	timeType := reflect.TypeFor[metav1.Time]()
	timeSchema, exists := opts.TypeSchemas[timeType]
	if !exists {
		t.Error("Expected metav1.Time to have a schema")
	}
	if timeSchema.Type != "string" {
		t.Errorf("Expected metav1.Time type to be 'string', got %s", timeSchema.Type)
	}
	if timeSchema.Format != "date-time" {
		t.Errorf("Expected metav1.Time format to be 'date-time', got %s", timeSchema.Format)
	}

	// Check that metav1.MicroTime is mapped correctly
	microTimeType := reflect.TypeFor[metav1.MicroTime]()
	microTimeSchema, exists := opts.TypeSchemas[microTimeType]
	if !exists {
		t.Error("Expected metav1.MicroTime to have a schema")
	}
	if microTimeSchema.Type != "string" {
		t.Errorf("Expected metav1.MicroTime type to be 'string', got %s", microTimeSchema.Type)
	}
	if microTimeSchema.Format != "date-time" {
		t.Errorf("Expected metav1.MicroTime format to be 'date-time', got %s", microTimeSchema.Format)
	}
}

func TestSelectDatadogAgent_WithProvidedName(t *testing.T) {
	opts := setupTestOptions()

	// Test with provided name - should return it directly
	name, errResult := opts.selectDatadogAgent("test-namespace", "my-agent")

	if errResult != nil {
		t.Fatalf("Expected no error result, got: %v", errResult)
	}

	if name != "my-agent" {
		t.Errorf("Expected name 'my-agent', got %s", name)
	}
}

func TestSelectDatadogAgent_AutoSelect_Success(t *testing.T) {
	// Create a DatadogAgent
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent",
			Namespace: "test-namespace",
		},
		Status: v2alpha1.DatadogAgentStatus{
			ClusterAgent: &v2alpha1.DeploymentStatus{},
		},
	}

	opts := setupTestOptions(dda)

	// Test auto-select with empty name
	name, errResult := opts.selectDatadogAgent("test-namespace", "")

	if errResult != nil {
		t.Fatalf("Expected no error result, got: %v", errResult)
	}

	if name != "datadog-agent" {
		t.Errorf("Expected name 'datadog-agent', got %s", name)
	}
}

func TestSelectDatadogAgent_AutoSelect_Failure(t *testing.T) {
	// No DatadogAgent objects
	opts := setupTestOptions()

	// Test auto-select with empty name - should fail
	name, errResult := opts.selectDatadogAgent("test-namespace", "")

	if errResult == nil {
		t.Fatal("Expected error result when no agents exist")
	}

	if name != "" {
		t.Errorf("Expected empty name on error, got %s", name)
	}

	if !errResult.IsError {
		t.Error("Expected IsError to be true")
	}

	if len(errResult.Content) == 0 {
		t.Error("Expected error content to be set")
	}
}

func TestRegisterTools(t *testing.T) {
	tests := []struct {
		name         string
		registerFunc func(*options, *mcp.Server)
	}{
		{
			name: "registerListAgentsTool",
			registerFunc: func(o *options, s *mcp.Server) {
				o.registerListAgentsTool(s)
			},
		},
		{
			name: "registerGetAgentStatusTool",
			registerFunc: func(o *options, s *mcp.Server) {
				o.registerGetAgentStatusTool(s)
			},
		},
		{
			name: "registerDescribeAgentFeaturesTool",
			registerFunc: func(o *options, s *mcp.Server) {
				o.registerDescribeAgentFeaturesTool(s)
			},
		},
		{
			name: "registerDescribeAgentComponentsTool",
			registerFunc: func(o *options, s *mcp.Server) {
				o.registerDescribeAgentComponentsTool(s)
			},
		},
		{
			name: "registerGetClusterAgentLeaderTool",
			registerFunc: func(o *options, s *mcp.Server) {
				o.registerGetClusterAgentLeaderTool(s)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := setupTestOptions()
			server := createTestServer()

			// Register the tool - should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s panicked: %v", tt.name, r)
				}
			}()

			tt.registerFunc(opts, server)
		})
	}
}

func TestAllToolsRegistered(t *testing.T) {
	opts := setupTestOptions()
	server := createTestServer()

	// Register all tools - should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Tool registration panicked: %v", r)
		}
	}()

	opts.registerListAgentsTool(server)
	opts.registerGetAgentStatusTool(server)
	opts.registerDescribeAgentFeaturesTool(server)
	opts.registerDescribeAgentComponentsTool(server)
	opts.registerGetClusterAgentLeaderTool(server)
}
