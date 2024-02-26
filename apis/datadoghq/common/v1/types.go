// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import corev1 "k8s.io/api/core/v1"

// SecretConfig contains a secret name and an included key.
// +kubebuilder:object:generate=true
type SecretConfig struct {
	// SecretName is the name of the secret.
	SecretName string `json:"secretName"`

	// KeyName is the key of the secret to use.
	// +optional
	KeyName string `json:"keyName,omitempty"`
}

// ConfigMapConfig contains ConfigMap information used to store a configuration file.
// +kubebuilder:object:generate=true
type ConfigMapConfig struct {
	// Name is the name of the ConfigMap.
	Name string `json:"name,omitempty"`

	// Items maps a ConfigMap data `key` to a file `path` mount.
	// +listType=map
	// +listMapKey=key
	// +optional
	Items []corev1.KeyToPath `json:"items,omitempty"`
}

// CustomConfig allows one to put custom configurations for the agent.
// +kubebuilder:object:generate=true
type CustomConfig struct {
	// ConfigData corresponds to the configuration file content.
	// +optional
	ConfigData *string
	// Enable to specify a reference to an already existing ConfigMap.
	// +optional
	ConfigMap *ConfigMapConfig
}

// KubeletConfig contains the kubelet configuration parameters.
// +kubebuilder:object:generate=true
type KubeletConfig struct {
	// Host overrides the host used to contact kubelet API (default to status.hostIP).
	// +optional
	Host *corev1.EnvVarSource `json:"host,omitempty"`

	// TLSVerify toggles kubelet TLS verification.
	// Default: true
	// +optional
	TLSVerify *bool `json:"tlsVerify,omitempty"`

	// HostCAPath is the host path where the kubelet CA certificate is stored.
	// +optional
	HostCAPath string `json:"hostCAPath,omitempty"`

	// AgentCAPath is the container path where the kubelet CA certificate is stored.
	// Default: '/var/run/host-kubelet-ca.crt' if hostCAPath is set, else '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt'
	// +optional
	AgentCAPath string `json:"agentCAPath,omitempty"`
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
	// AllContainers is used internally to reference all containers in the pod
	AllContainers AgentContainerName = "all"
	// ClusterAgentContainerName is the name of the Cluster Agent container
	ClusterAgentContainerName AgentContainerName = "cluster-agent"

	// ClusterChecksRunnersContainerName is the name of the Agent container in Cluster Checks Runners
	ClusterChecksRunnersContainerName AgentContainerName = "agent"

	// FIPSProxyContainerName is the name of the FIPS Proxy container
	FIPSProxyContainerName AgentContainerName = "fips-proxy"
)
