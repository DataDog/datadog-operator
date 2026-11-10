// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// ClusterCollector returns the collector of the cluster context: the
// Kubernetes version and distribution, the node count and OS mix, and the
// cluster name and ID.
func ClusterCollector() Collector {
	return Collector{Name: "cluster", Collect: collectCluster}
}

// ClusterID returns the ID of the cluster: the UID of the kube-system
// namespace, as the Operator reports it to Remote Configuration.
func ClusterID(ctx context.Context, reader client.Reader) (string, error) {
	ns := &corev1.Namespace{}
	if err := reader.Get(ctx, types.NamespacedName{Name: "kube-system"}, ns); err != nil {
		return "", fmt.Errorf("unable to get the kube-system namespace: %w", err)
	}
	return string(ns.UID), nil
}

func collectCluster(ctx context.Context, c *Collection) error {
	info := map[string]any{}
	attempts, failures := 2, 0

	var serverVersion *version.Info
	if c.Env.Clientset != nil {
		attempts++
		var err error
		if serverVersion, err = c.Env.Clientset.Discovery().ServerVersion(); err != nil {
			failures++
			c.Warn(fmt.Errorf("unable to get the Kubernetes version: %w", err))
		} else {
			info["kubernetesVersion"] = serverVersion
		}
	}

	if clusterID, err := ClusterID(ctx, c.Env.Reader); err != nil {
		failures++
		c.Warn(err)
	} else {
		info["clusterID"] = clusterID
	}
	if c.Env.ClusterName != "" {
		info["clusterName"] = c.Env.ClusterName
	}

	nodes := &corev1.NodeList{}
	if err := c.Env.Reader.List(ctx, nodes); err != nil {
		failures++
		c.Warn(fmt.Errorf("unable to list nodes: %w", err))
	} else {
		info["nodes"] = summarizeNodes(nodes.Items)
		info["distribution"] = distribution(serverVersion, nodes.Items)
	}

	if failures == attempts {
		return fmt.Errorf("unable to get the cluster context")
	}
	return c.AddYAML("cluster.yaml", info)
}

type nodesSummary struct {
	Count             int            `json:"count"`
	OperatingSystems  map[string]int `json:"operatingSystems"`
	Architectures     map[string]int `json:"architectures"`
	OSImages          map[string]int `json:"osImages"`
	KubeletVersions   map[string]int `json:"kubeletVersions"`
	ContainerRuntimes map[string]int `json:"containerRuntimes"`
	Providers         map[string]int `json:"providers"`
	NotReady          []string       `json:"notReady,omitempty"`
}

func summarizeNodes(nodes []corev1.Node) nodesSummary {
	summary := nodesSummary{
		Count:             len(nodes),
		OperatingSystems:  map[string]int{},
		Architectures:     map[string]int{},
		OSImages:          map[string]int{},
		KubeletVersions:   map[string]int{},
		ContainerRuntimes: map[string]int{},
		Providers:         map[string]int{},
	}
	for i := range nodes {
		nodeInfo := nodes[i].Status.NodeInfo
		summary.OperatingSystems[nodeInfo.OperatingSystem]++
		summary.Architectures[nodeInfo.Architecture]++
		summary.OSImages[nodeInfo.OSImage]++
		summary.KubeletVersions[nodeInfo.KubeletVersion]++
		summary.ContainerRuntimes[nodeInfo.ContainerRuntimeVersion]++
		summary.Providers[kubernetes.ClusterProviderFromNode(&nodes[i])]++
		ready := false
		for _, cond := range nodes[i].Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		if !ready && len(summary.NotReady) < 100 {
			summary.NotReady = append(summary.NotReady, nodes[i].Name)
		}
	}
	return summary
}

// distribution guesses the Kubernetes distribution from the server version
// and the nodes.
func distribution(serverVersion *version.Info, nodes []corev1.Node) string {
	gitVersion := ""
	if serverVersion != nil {
		gitVersion = serverVersion.GitVersion
	}
	switch {
	case strings.Contains(gitVersion, "-eks-"):
		return "eks"
	case strings.Contains(gitVersion, "-gke."):
		return "gke"
	case strings.Contains(gitVersion, "+k3s"):
		return "k3s"
	case strings.Contains(gitVersion, "+rke2"):
		return "rke2"
	}
	for i := range nodes {
		node := &nodes[i]
		if _, ok := node.Labels[kubernetes.OpenShiftProviderLabel]; ok {
			return "openshift"
		}
		providerID := node.Spec.ProviderID
		switch {
		case strings.HasPrefix(providerID, "azure://"):
			return "aks"
		case strings.HasPrefix(providerID, "aws://"):
			return "aws"
		case strings.HasPrefix(providerID, "gce://"):
			return "gce"
		case strings.HasPrefix(providerID, "kind://"):
			return "kind"
		case providerID != "":
			if scheme, _, found := strings.Cut(providerID, "://"); found {
				return scheme
			}
		}
	}
	return "unknown"
}
