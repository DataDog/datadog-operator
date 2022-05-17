// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import corev1 "k8s.io/api/core/v1"

// AgentImageConfig defines the agent container image config.
// +kubebuilder:object:generate=true
type AgentImageConfig struct {
	// Define the image to use:
	// Use "gcr.io/datadoghq/agent:latest" for Datadog Agent 7.
	// Use "datadog/dogstatsd:latest" for standalone Datadog Agent DogStatsD 7.
	// Use "gcr.io/datadoghq/cluster-agent:latest" for Datadog Cluster Agent.
	// Use "agent" with the registry and tag configurations for <registry>/agent:<tag>.
	// Use "cluster-agent" with the registry and tag configurations for <registry>/cluster-agent:<tag>.
	Name string `json:"name,omitempty"`

	// Define the image tag to use.
	// To be used if the Name field does not correspond to a full image string.
	// +optional
	Tag string `json:"tag,omitempty"`

	// Define whether the Agent image should support JMX.
	// +optional
	JMXEnabled bool `json:"jmxEnabled,omitempty"`

	// The Kubernetes pull policy:
	// Use Always, Never or IfNotPresent.
	PullPolicy *corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// It is possible to specify Docker registry credentials.
	// See https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod
	// +optional
	PullSecrets *[]corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}
