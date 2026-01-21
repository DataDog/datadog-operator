// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	// DatadogAgentGVR is the GroupVersionResource for DatadogAgent CRD
	DatadogAgentGVR = schema.GroupVersionResource{
		Group:    "datadoghq.com",
		Version:  "v2alpha1",
		Resource: "datadogagents",
	}
)

// DeleteAllDatadogAgentsWithKubeConfig deletes all DatadogAgent resources in the specified namespace
// using a kubeconfig string to create the client.
// This is useful for cleanup before Pulumi stack destroy to avoid CRD deletion timeout
// caused by finalizers blocking deletion.
func DeleteAllDatadogAgentsWithKubeConfig(ctx context.Context, kubeConfig string, namespace string, timeout time.Duration) error {
	// Parse kubeconfig string to create rest.Config
	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfig))
	if err != nil {
		return fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	return DeleteAllDatadogAgentsWithConfig(ctx, restConfig, namespace, timeout)
}

// DeleteAllDatadogAgentsWithConfig deletes all DatadogAgent resources in the specified namespace
// and waits for them to be fully deleted (including finalizer processing).
// This is useful for cleanup before Pulumi stack destroy to avoid CRD deletion timeout
// caused by finalizers blocking deletion.
func DeleteAllDatadogAgentsWithConfig(ctx context.Context, restConfig *rest.Config, namespace string, timeout time.Duration) error {
	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return deleteAllDatadogAgentsWithClient(ctx, dynamicClient, namespace, timeout)
}

// DeleteAllDatadogAgents deletes all DatadogAgent resources in the specified namespace
// using a dynamic client and waits for them to be fully deleted.
func DeleteAllDatadogAgents(ctx context.Context, dynamicClient dynamic.Interface, namespace string, timeout time.Duration) error {
	return deleteAllDatadogAgentsWithClient(ctx, dynamicClient, namespace, timeout)
}

func deleteAllDatadogAgentsWithClient(ctx context.Context, dynamicClient dynamic.Interface, namespace string, timeout time.Duration) error {
	ddaClient := dynamicClient.Resource(DatadogAgentGVR).Namespace(namespace)

	// List all DatadogAgents
	ddaList, err := ddaClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// CRD doesn't exist, nothing to delete
			return nil
		}
		// If we get a "no matches" error, it means the CRD isn't installed
		if errors.IsNotFound(err) || isNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("failed to list DatadogAgents: %w", err)
	}

	if len(ddaList.Items) == 0 {
		return nil
	}

	// Delete each DatadogAgent
	for _, dda := range ddaList.Items {
		name := dda.GetName()
		err := ddaClient.Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete DatadogAgent %s: %w", name, err)
		}
	}

	// Wait for all DatadogAgents to be fully deleted
	return waitForDatadogAgentsDeletion(ctx, ddaClient, ddaList.Items, timeout)
}

// isNoMatchError checks if the error indicates the resource type doesn't exist
func isNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	// Check for "no matches for kind" error which happens when CRD doesn't exist
	return errors.IsNotFound(err) ||
		(err.Error() != "" && (contains(err.Error(), "no matches for kind") ||
			contains(err.Error(), "the server could not find the requested resource")))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// waitForDatadogAgentsDeletion waits for all specified DatadogAgents to be fully deleted
func waitForDatadogAgentsDeletion(ctx context.Context, ddaClient dynamic.ResourceInterface, ddas []unstructured.Unstructured, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 5 * time.Second

	for _, dda := range ddas {
		name := dda.GetName()
		for {
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for DatadogAgent %s to be deleted", name)
			}

			_, err := ddaClient.Get(ctx, name, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				// DDA is deleted
				break
			}
			if err != nil {
				return fmt.Errorf("error checking DatadogAgent %s: %w", name, err)
			}

			// DDA still exists, wait and retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
		}
	}

	return nil
}
