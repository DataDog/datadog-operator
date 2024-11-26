// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"k8s.io/apimachinery/pkg/util/intstr"
)

// The deployment strategy to use to replace existing pods with new ones.
// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
type UpdateStrategy struct {
	// Type can be "RollingUpdate" or "OnDelete" for DaemonSets and "RollingUpdate"
	// or "Recreate" for Deployments
	Type string `json:"type,omitempty"`
	// Configure the rolling update strategy of the Deployment or DaemonSet.
	RollingUpdate *RollingUpdate `json:"rollingUpdate,omitempty"`
}

// RollingUpdate describes how to replace existing pods with new ones.
// +k8s:openapi-gen=true
// +kubebuilder:object:generate=true
type RollingUpdate struct {
	// The maximum number of pods that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%).
	// Refer to the Kubernetes API documentation for additional details..
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// MaxSurge behaves differently based on the Kubernetes resource. Refer to the
	// Kubernetes API documentation for additional details.
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

// AgentContainerName is the name of a container inside an Agent component
type AgentContainerName string

const (
	// InitVolumeContainerName is the name of the Init Volume init container
	InitVolumeContainerName AgentContainerName = "init-volume"
	// InitConfigContainerName is the name of the Init config Volume init container
	InitConfigContainerName AgentContainerName = "init-config"
	// SeccompSetupContainerName is the name of the Seccomp Setup init container
	SeccompSetupContainerName AgentContainerName = "seccomp-setup"

	// UnprivilegedSingleAgentContainerName is the name of a container which may run
	// any combination of Core, Trace and Process Agent processes in a single container.
	UnprivilegedSingleAgentContainerName AgentContainerName = "unprivileged-single-agent"
	// CoreAgentContainerName is the name of the Core Agent container
	CoreAgentContainerName AgentContainerName = "agent"
	// TraceAgentContainerName is the name of the Trace Agent container
	TraceAgentContainerName AgentContainerName = "trace-agent"
	// ProcessAgentContainerName is the name of the Process Agent container
	ProcessAgentContainerName AgentContainerName = "process-agent"
	// SecurityAgentContainerName is the name of the Security Agent container
	SecurityAgentContainerName AgentContainerName = "security-agent"
	// SystemProbeContainerName is the name of the System Probe container
	SystemProbeContainerName AgentContainerName = "system-probe"
	// OtelAgent is the name of the OTel container
	OtelAgent AgentContainerName = "otel-agent"
	// AgentDataPlaneContainerName is the name of the Agent Data Plane container
	AgentDataPlaneContainerName AgentContainerName = "agent-data-plane"
	// AllContainers is used internally to reference all containers in the pod
	AllContainers AgentContainerName = "all"
	// ClusterAgentContainerName is the name of the Cluster Agent container
	ClusterAgentContainerName AgentContainerName = "cluster-agent"
	// ClusterChecksRunnersContainerName is the name of the Agent container in Cluster Checks Runners
	ClusterChecksRunnersContainerName AgentContainerName = "agent"

	// FIPSProxyContainerName is the name of the FIPS Proxy container
	FIPSProxyContainerName AgentContainerName = "fips-proxy"
)
