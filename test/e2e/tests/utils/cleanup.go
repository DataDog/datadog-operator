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
)

var (
	// DatadogAgentGVR is the GroupVersionResource for DatadogAgent CRD
	DatadogAgentGVR = schema.GroupVersionResource{
		Group:    "datadoghq.com",
		Version:  "v2alpha1",
		Resource: "datadogagents",
	}

	// DatadogAgentInternalGVR is the GroupVersionResource for DatadogAgentInternal CRD
	DatadogAgentInternalGVR = schema.GroupVersionResource{
		Group:    "datadoghq.com",
		Version:  "v1alpha1",
		Resource: "datadogagentinternals",
	}
)

// DeleteAllDatadogResources deletes all DatadogAgent and DatadogAgentInternal resources
// in the specified namespace.
// This is useful for cleanup before Pulumi stack destroy to avoid CRD deletion timeout
// caused by finalizers blocking deletion.
func DeleteAllDatadogResources(ctx context.Context, k8sConfig *rest.Config, namespace string) error {
	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Delete DatadogAgents first (they create DatadogAgentInternals)
	if err := deleteAllDatadogAgentsWithClient(ctx, dynamicClient, namespace); err != nil {
		return fmt.Errorf("failed to delete DatadogAgents: %w", err)
	}

	// Delete DatadogAgentInternals (in case any remain)
	if err := deleteAllDatadogAgentInternalsWithClient(ctx, dynamicClient, namespace); err != nil {
		return fmt.Errorf("failed to delete DatadogAgentInternals: %w", err)
	}

	return nil
}

func deleteAllDatadogAgentsWithClient(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	ddaClient := dynamicClient.Resource(DatadogAgentGVR).Namespace(namespace)

	// List all DatadogAgents
	ddaList, err := ddaClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// CRD doesn't exist, nothing to delete
			return nil
		}
		return fmt.Errorf("failed to list DatadogAgents: %w", err)
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
	return waitForDatadogAgentsDeletion(ctx, ddaClient, ddaList.Items)
}

func deleteAllDatadogAgentInternalsWithClient(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	ddaiClient := dynamicClient.Resource(DatadogAgentInternalGVR).Namespace(namespace)

	// List all DatadogAgentInternals
	ddaiList, err := ddaiClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// CRD doesn't exist, nothing to delete
			return nil
		}
		return fmt.Errorf("failed to list DatadogAgentInternals: %w", err)
	}

	// Delete each DatadogAgentInternal
	for _, ddai := range ddaiList.Items {
		name := ddai.GetName()
		err := ddaiClient.Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete DatadogAgentInternal %s: %w", name, err)
		}
	}

	// Wait for all DatadogAgentInternals to be fully deleted
	return waitForDatadogAgentsDeletion(ctx, ddaiClient, ddaiList.Items)
}

// waitForDatadogAgentsDeletion waits for all specified DatadogAgents to be fully deleted
func waitForDatadogAgentsDeletion(ctx context.Context, ddaClient dynamic.ResourceInterface, ddas []unstructured.Unstructured) error {
	pollInterval := 5 * time.Second

	for _, dda := range ddas {
		name := dda.GetName()
		for {
			if _, err := ddaClient.Get(ctx, name, metav1.GetOptions{}); err != nil {
				if errors.IsNotFound(err) {
					// DDA is deleted
					break
				}
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
