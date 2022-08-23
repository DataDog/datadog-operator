// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

// DaemonSetStatus defines the observed state of Agent running as DaemonSet.
// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
type DaemonSetStatus struct {
	Desired   int32 `json:"desired"`
	Current   int32 `json:"current"`
	Ready     int32 `json:"ready"`
	Available int32 `json:"available"`
	UpToDate  int32 `json:"upToDate"`

	Status      string       `json:"status,omitempty"`
	State       string       `json:"state,omitempty"`
	LastUpdate  *metav1.Time `json:"lastUpdate,omitempty"`
	CurrentHash string       `json:"currentHash,omitempty"`

	// DaemonsetName corresponds to the name of the created DaemonSet.
	DaemonsetName string `json:"daemonsetName,omitempty"`
}

// DeploymentStatus type representing a Deployment status.
// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
type DeploymentStatus struct {
	// Total number of non-terminated pods targeted by this deployment (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Total number of non-terminated pods targeted by this deployment that have the desired template spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// Total number of ready pods targeted by this deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Total number of available pods (ready for at least minReadySeconds) targeted by this deployment.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Total number of unavailable pods targeted by this deployment. This is the total number of
	// pods that are still required for the deployment to have 100% available capacity. They may
	// either be pods that are running but not yet available or pods that still have not been created.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	LastUpdate  *metav1.Time `json:"lastUpdate,omitempty"`
	CurrentHash string       `json:"currentHash,omitempty"`

	// GeneratedToken corresponds to the generated token if any token was provided in the Credential configuration when ClusterAgent is
	// enabled.
	// +optional
	GeneratedToken string `json:"generatedToken,omitempty"`

	// Status corresponds to the ClusterAgent deployment computed status.
	Status string `json:"status,omitempty"`
	// State corresponds to the ClusterAgent deployment state.
	State string `json:"state,omitempty"`

	// DeploymentName corresponds to the name of the Cluster Agent Deployment.
	DeploymentName string `json:"deploymentName,omitempty"`
}
