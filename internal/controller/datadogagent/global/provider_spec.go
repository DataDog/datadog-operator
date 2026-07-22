// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file is the declarative manifest of provider-conditional global
// mutations applied to the node agent pod template. It contains constants,
// type definitions, and the spec map. Behaviour (applier, helpers) lives in
// provider_apply.go.

package global

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// cloudInitInstanceIDPath is the host path EKS-EC2 nodes expose containing the
// EC2 instance-id, used by the agent to derive a stable hostname.
const cloudInitInstanceIDPath = "/var/lib/cloud/data/instance-id"

// cloudInitInstanceIDVolumeName is the pod-level volume name for the cloud-init
// instance-id file.
const cloudInitInstanceIDVolumeName = "cloudinit-instance-id-file"

// GKE Autopilot node-agent constants. These mirror the values the (now removed)
// experimental autopilot override applied imperatively.
const (
	// ddKubeletUseAPIServer makes the Agent discover pods via the API server
	// instead of the kubelet, which is not reachable on Autopilot nodes.
	ddKubeletUseAPIServer = "DD_KUBELET_USE_API_SERVER"
	// ddCloudProviderMetadata restricts host-alias collection to GCP metadata.
	ddCloudProviderMetadata = "DD_CLOUD_PROVIDER_METADATA"
	// autopilotInitVolumeName is the init container whose args must be rewritten.
	autopilotInitVolumeName = "init-volume"
)

// NodeAgentProviderSpec is the provider-keyed capabilities map for the node
// agent pod template. The "" baseline applies to all providers; provider-keyed
// entries are applied on top (removals first, then additions).
//
// Mirrors the Helm chart's _provider-specific_ pod-template additions (see
// charts/datadog/templates/_containers-common-env.yaml and
// _container-cloudinit-volumemounts.yaml).
var NodeAgentProviderSpec = providercaps.ProviderCapabilityMap{
	kubernetes.EKSEC2UseHostnameFromFileProvider: {
		// DD_HOSTNAME_FILE points the agent at the EC2 instance-id so it
		// derives a stable hostname even when the kubelet hostname differs.
		// Helm includes this in containers-common-env, which renders into
		// every main container AND every init container; mirror that.
		EnvVars: []providercaps.EnvVarSet{
			{
				EnvVar: corev1.EnvVar{
					Name:  "DD_HOSTNAME_FILE",
					Value: cloudInitInstanceIDPath,
				},
				InitContainers: []apicommon.AgentContainerName{
					apicommon.InitConfigContainerName,
				},
			},
		},
		// HostPath mount of the instance-id file. Helm adds this on every
		// agent-side main container; enumerate the same set.
		Volumes: []providercaps.VolumeAndMount{
			{
				Volume: corev1.Volume{
					Name: cloudInitInstanceIDVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: cloudInitInstanceIDPath,
							Type: ptr.To(corev1.HostPathFile),
						},
					},
				},
				Mount: corev1.VolumeMount{
					Name:      cloudInitInstanceIDVolumeName,
					MountPath: cloudInitInstanceIDPath,
					ReadOnly:  true,
				},
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
					apicommon.TraceAgentContainerName,
					apicommon.ProcessAgentContainerName,
					apicommon.SystemProbeContainerName,
					apicommon.SecurityAgentContainerName,
					apicommon.AgentDataPlaneContainerName,
					apicommon.OtelAgent,
					apicommon.UnprivilegedSingleAgentContainerName,
				},
			},
		},
	},

	// GKE Autopilot. The WorkloadAllowlist forbids most hostPath volumes and the
	// kubelet endpoint, so the node agent must drop several default volumes/mounts
	// and discover pods via the API server. On Autopilot the dogstatsd and APM
	// features disable UDS (see their Configure), so the only sockets present are
	// the default-builder ones removed here; nothing feature-specific is added.
	// Mutations that the declarative model can't express (pod label, init-volume
	// args, run-path hostPath remap, seccomp read-only, trace/process commands)
	// live in applyAutopilotGlobalExtras in provider_apply.go.
	kubernetes.GKEAutopilotProvider: {
		EnvVars: []providercaps.EnvVarSet{
			{EnvVar: corev1.EnvVar{Name: ddKubeletUseAPIServer, Value: "true"}},
			{EnvVar: corev1.EnvVar{Name: ddCloudProviderMetadata, Value: `["gcp"]`}},
			{EnvVar: corev1.EnvVar{Name: DDProviderKind, Value: kubernetes.GKEAutopilotProvider}},
		},
		// Volumes absent from the Autopilot WorkloadAllowlist. RemoveVolumes
		// strips the volume and its mounts from every container/init container.
		// The dogstatsd socket is colocated in the dogstatsd feature (it owns that
		// volume's provider variation); see dogstatsd.NodeAgentProviderCapabilities.
		RemoveVolumes: []string{
			common.AuthVolumeName,
			common.CriSocketVolumeName,
		},
		// proc and cgroups volumes are kept (other containers need them) but their
		// mounts are not permitted on the trace-agent.
		RemoveMounts: []providercaps.ContainerMountRef{
			{VolumeName: common.ProcdirVolumeName, Containers: []apicommon.AgentContainerName{apicommon.TraceAgentContainerName}},
			{VolumeName: common.CgroupsVolumeName, Containers: []apicommon.AgentContainerName{apicommon.TraceAgentContainerName}},
		},
		// The auth token file path env var is meaningless once the auth volume is gone.
		RemoveEnvVars: []string{common.DDAuthTokenFilePath},
	},
}

// ClusterAgentProviderSpec is the provider-keyed capabilities map for the Cluster
// Agent pod template. Helm's _components-common-env.yaml emits DD_CLOUD_PROVIDER_METADATA
// on all components that include that template; DCA includes it but does NOT include
// the provider-env helper that emits DD_PROVIDER_KIND.
var ClusterAgentProviderSpec = providercaps.ProviderCapabilityMap{
	kubernetes.GKEAutopilotProvider: {
		EnvVars: []providercaps.EnvVarSet{
			{EnvVar: corev1.EnvVar{Name: ddCloudProviderMetadata, Value: `["gcp"]`}},
		},
	},
}

// ClusterChecksRunnerProviderSpec is the provider-keyed capabilities map for the
// Cluster Checks Runner pod template. CCR includes both _components-common-env.yaml
// (DD_CLOUD_PROVIDER_METADATA) and the provider-env helper (DD_PROVIDER_KIND).
var ClusterChecksRunnerProviderSpec = providercaps.ProviderCapabilityMap{
	kubernetes.GKEAutopilotProvider: {
		EnvVars: []providercaps.EnvVarSet{
			{EnvVar: corev1.EnvVar{Name: ddCloudProviderMetadata, Value: `["gcp"]`}},
			{EnvVar: corev1.EnvVar{Name: DDProviderKind, Value: kubernetes.GKEAutopilotProvider}},
		},
	},
}

// nodeAgentProviderPodLabels holds provider-conditional pod-template labels.
// providercaps has no label support (labels are a pod-level, not per-container,
// concern), so they are applied separately in applyAutopilotGlobalExtras.
var nodeAgentProviderPodLabels = map[string]map[string]string{
	kubernetes.GKEAutopilotProvider: {
		// Prevent the agent DaemonSet from being mutated by the admission controller.
		"admission.datadoghq.com/enabled": "false",
		// Tag the pod with its cloud-provider identity, matching Helm's provider-labels.
		"env.datadoghq.com/kind": kubernetes.GKEAutopilotProvider,
	},
}
