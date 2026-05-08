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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// cloudInitInstanceIDPath is the host path EKS-EC2 nodes expose containing the
// EC2 instance-id, used by the agent to derive a stable hostname.
const cloudInitInstanceIDPath = "/var/lib/cloud/data/instance-id"

// cloudInitInstanceIDVolumeName is the pod-level volume name for the cloud-init
// instance-id file.
const cloudInitInstanceIDVolumeName = "cloudinit-instance-id-file"

// NodeAgentProviderSpec is the provider-keyed capabilities map for the node
// agent pod template. The "" baseline applies to all providers; provider-keyed
// entries are applied on top (removals first, then additions).
//
// Mirrors the Helm chart's _provider-specific_ pod-template additions (see
// charts/datadog/templates/_containers-common-env.yaml and
// _container-cloudinit-volumemounts.yaml).
var NodeAgentProviderSpec = providercaps.NodeAgentProviderCapabilities{
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
				},
			},
		},
	},
}
