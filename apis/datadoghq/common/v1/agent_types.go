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
	// If the name is full image string—`<name>:<tag>` or `<registry>/<name>:<tag>`, then `tag`, `jmxEnabled`,
	// and `global.registry` values are ignored.
	// Otherwise, image string is created by overriding default settings with supplied `name`, `tag`, and `jmxEnabled` values;
	// image string is created using default registry unless `global.registry` is configured.
	Name string `json:"name,omitempty"`

	// Define the image tag to use.
	// To be used if the Name field does not correspond to a full image string.
	// +optional
	Tag string `json:"tag,omitempty"`

	// Define whether the Agent image should support JMX.
	// To be used if the Name field does not correspond to a full image string.
	// +optional
	JMXEnabled bool `json:"jmxEnabled,omitempty"`

	// The Kubernetes pull policy:
	// Use Always, Never, or IfNotPresent.
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
	// Number of desired pods in the DaemonSet.
	Desired int32 `json:"desired"`

	// Number of current pods in the DaemonSet.
	Current int32 `json:"current"`

	// Number of ready pods in the DaemonSet.
	Ready int32 `json:"ready"`

	// Number of available pods in the DaemonSet.
	Available int32 `json:"available"`

	// Number of up to date pods in the DaemonSet.
	UpToDate int32 `json:"upToDate"`

	// LastUpdate is the last time the status was updated.
	LastUpdate *metav1.Time `json:"lastUpdate,omitempty"`

	// CurrentHash is the stored hash of the DaemonSet.
	CurrentHash string `json:"currentHash,omitempty"`

	// Status corresponds to the DaemonSet computed status.
	Status string `json:"status,omitempty"`

	// State corresponds to the DaemonSet state.
	State string `json:"state,omitempty"`

	// DaemonsetName corresponds to the name of the created DaemonSet.
	DaemonsetName string `json:"daemonsetName,omitempty"`
}

// DeploymentStatus type representing a Deployment status.
// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
type DeploymentStatus struct {
	// Total number of non-terminated pods targeted by this Deployment (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Total number of non-terminated pods targeted by this Deployment that have the desired template spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// Total number of ready pods targeted by this Deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Total number of available pods (ready for at least minReadySeconds) targeted by this Deployment.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Total number of unavailable pods targeted by this Deployment. This is the total number of
	// pods that are still required for the Deployment to have 100% available capacity. They may
	// either be pods that are running but not yet available or pods that still have not been created.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// LastUpdate is the last time the status was updated.
	LastUpdate *metav1.Time `json:"lastUpdate,omitempty"`

	// CurrentHash is the stored hash of the Deployment.
	CurrentHash string `json:"currentHash,omitempty"`

	// GeneratedToken corresponds to the generated token if any token was provided in the Credential configuration when ClusterAgent is
	// enabled.
	// +optional
	GeneratedToken string `json:"generatedToken,omitempty"`

	// Status corresponds to the Deployment computed status.
	Status string `json:"status,omitempty"`

	// State corresponds to the Deployment state.
	State string `json:"state,omitempty"`

	// DeploymentName corresponds to the name of the Deployment.
	DeploymentName string `json:"deploymentName,omitempty"`
}
