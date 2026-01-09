// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mcp

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/clusteragent/leader"
)

const (
	// ClusterAgentComponentLabel is the label used to identify cluster-agent pods
	ClusterAgentComponentLabel = "agent.datadoghq.com/component=cluster-agent"

	// DatadogAgentNameLabel is the label used to identify which DatadogAgent a pod belongs to
	DatadogAgentNameLabel = "agent.datadoghq.com/name"
)

// ClusterAgentDiscovery handles finding and connecting to cluster-agent pods
type ClusterAgentDiscovery struct {
	client          client.Client
	clientset       kubernetes.Interface
	discoveryClient discovery.DiscoveryInterface
	namespace       string
}

// NewClusterAgentDiscovery creates a new cluster-agent discovery instance
func NewClusterAgentDiscovery(
	c client.Client,
	clientset kubernetes.Interface,
	discoveryClient discovery.DiscoveryInterface,
	namespace string,
) *ClusterAgentDiscovery {
	return &ClusterAgentDiscovery{
		client:          c,
		clientset:       clientset,
		discoveryClient: discoveryClient,
		namespace:       namespace,
	}
}

// DiscoverLeaderPod finds the cluster-agent leader pod for a given DatadogAgent
// Returns the pod name, namespace, and any error encountered
func (d *ClusterAgentDiscovery) DiscoverLeaderPod(ddaName string) (string, string, error) {
	// Build the leader election object name
	leaderObjName := fmt.Sprintf("%s-leader-election", ddaName)
	objKey := client.ObjectKey{Namespace: d.namespace, Name: leaderObjName}

	// Check if Lease API is supported
	useLease, err := leader.IsLeaseSupported(d.discoveryClient)
	if err != nil {
		return "", "", fmt.Errorf("unable to check if lease is supported: %w", err)
	}

	var leaderName string

	// Try to get leader from Lease if supported
	if useLease {
		leaderName, err = leader.GetLeaderFromLease(d.client, objKey)
		if err == nil {
			return leaderName, d.namespace, nil
		}
		// If Lease lookup failed, try ConfigMap fallback
	}

	// Fall back to ConfigMap
	leaderName, err = leader.GetLeaderFromConfigMap(d.client, objKey)
	if err != nil {
		return "", "", fmt.Errorf("unable to get leader from lease or configmap: %w", err)
	}

	return leaderName, d.namespace, nil
}

// GetClusterAgentPod finds any running cluster-agent pod for a given DatadogAgent
// This is a fallback if leader election doesn't work
func (d *ClusterAgentDiscovery) GetClusterAgentPod(ddaName string) (*corev1.Pod, error) {
	// Build label selector
	labelSelector := ClusterAgentComponentLabel
	if ddaName != "" {
		labelSelector = fmt.Sprintf("%s,%s=%s", ClusterAgentComponentLabel, DatadogAgentNameLabel, ddaName)
	}

	// List pods with the cluster-agent component label
	pods, err := d.clientset.CoreV1().Pods(d.namespace).List(
		context.TODO(),
		metav1.ListOptions{
			LabelSelector: labelSelector,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster-agent pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no cluster-agent pods found in namespace %s", d.namespace)
	}

	// Find first Running pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase == corev1.PodRunning {
			return pod, nil
		}
	}

	return nil, fmt.Errorf("no running cluster-agent pods found in namespace %s", d.namespace)
}

// SelectDatadogAgent chooses which DatadogAgent to proxy
// If ddaName is specified, returns it
// Otherwise, lists all DatadogAgents and picks the first one with cluster-agent enabled
func (d *ClusterAgentDiscovery) SelectDatadogAgent(ddaName string) (string, error) {
	// If name specified, use it
	if ddaName != "" {
		// Verify it exists and has cluster-agent enabled
		dda := &v2alpha1.DatadogAgent{}
		key := client.ObjectKey{Namespace: d.namespace, Name: ddaName}
		if err := d.client.Get(context.TODO(), key, dda); err != nil {
			return "", fmt.Errorf("DatadogAgent %s not found: %w", ddaName, err)
		}

		// Check if cluster-agent is enabled
		if !d.hasClusterAgentEnabled(dda) {
			return "", fmt.Errorf("DatadogAgent %s does not have cluster-agent enabled", ddaName)
		}

		return ddaName, nil
	}

	// Auto-select: list all DatadogAgents in namespace
	ddaList := &v2alpha1.DatadogAgentList{}
	listOpts := &client.ListOptions{Namespace: d.namespace}
	if err := d.client.List(context.TODO(), ddaList, listOpts); err != nil {
		return "", fmt.Errorf("failed to list DatadogAgents: %w", err)
	}

	if len(ddaList.Items) == 0 {
		return "", fmt.Errorf("no DatadogAgent resources found in namespace %s", d.namespace)
	}

	// Find first DatadogAgent with cluster-agent enabled
	for i := range ddaList.Items {
		dda := &ddaList.Items[i]
		if d.hasClusterAgentEnabled(dda) {
			if len(ddaList.Items) > 1 {
				// Warn if multiple DatadogAgents exist
				return dda.Name, fmt.Errorf("multiple DatadogAgents found, auto-selecting %s (use --proxy-dda-name to specify)", dda.Name)
			}
			return dda.Name, nil
		}
	}

	return "", fmt.Errorf("no DatadogAgent with cluster-agent enabled found in namespace %s", d.namespace)
}

// hasClusterAgentEnabled checks if a DatadogAgent has the cluster-agent component enabled
func (d *ClusterAgentDiscovery) hasClusterAgentEnabled(dda *v2alpha1.DatadogAgent) bool {
	// Check if cluster-agent is enabled in status
	if dda.Status.ClusterAgent != nil {
		return true
	}

	// Check if cluster-agent is configured in spec
	// In v2alpha1, cluster-agent is enabled by default unless explicitly disabled
	if dda.Spec.Override != nil {
		if override, ok := dda.Spec.Override[v2alpha1.ClusterAgentComponentName]; ok {
			if override.Disabled != nil && *override.Disabled {
				return false
			}
		}
	}

	// Default: cluster-agent is enabled
	return true
}
