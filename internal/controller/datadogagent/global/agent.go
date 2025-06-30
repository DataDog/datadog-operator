// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func applyNodeAgentResources(manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent, singleContainerStrategyEnabled bool, provider string) {
	config := dda.Spec.Global

	// Kubelet injection for Instrospection for AKS
	_, providerLabel := kubernetes.GetProviderLabelKeyValue(provider)

	if providerLabel == kubernetes.AKSRoleType {

		// Handle "tlsVerify: false"
		if config.Kubelet != nil && config.Kubelet.TLSVerify != nil && !*config.Kubelet.TLSVerify {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDKubeletTLSVerify,
				Value: apiutils.BoolToString(config.Kubelet.TLSVerify),
			})
		} else {
			// Configure the kubelet host
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name: common.DDKubeletHost,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			})

			// Configure the kubelet CA path
			if config.Kubelet == nil || config.Kubelet.HostCAPath == "" {
				const aksKubeletCAPath = "/etc/kubernetes/certs/kubeletserver.crt"
				agentCAPath := common.KubeletAgentCAPath

				kubeletVol, kubeletVolMount := volume.GetVolumes(kubeletCAVolumeName, aksKubeletCAPath, agentCAPath, true)
				if singleContainerStrategyEnabled {
					manager.VolumeMount().AddVolumeMountToContainers(
						&kubeletVolMount,
						[]apicommon.AgentContainerName{
							apicommon.UnprivilegedSingleAgentContainerName,
						},
					)
					manager.Volume().AddVolume(&kubeletVol)
				} else {
					manager.VolumeMount().AddVolumeMountToContainers(
						&kubeletVolMount,
						[]apicommon.AgentContainerName{
							apicommon.CoreAgentContainerName,
							apicommon.ProcessAgentContainerName,
							apicommon.TraceAgentContainerName,
							apicommon.SecurityAgentContainerName,
							apicommon.AgentDataPlaneContainerName,
						},
					)
					manager.Volume().AddVolume(&kubeletVol)
				}

				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:  DDKubeletCAPath,
					Value: agentCAPath,
				})
			}
		}
	}

	// Kubelet contains the kubelet configuration parameters.
	// The environment variable `DD_KUBERNETES_KUBELET_HOST` defaults to `status.hostIP` if not overriden.
	if config.Kubelet != nil {
		if config.Kubelet.Host != nil {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:      common.DDKubeletHost,
				ValueFrom: config.Kubelet.Host,
			})
		}
		if config.Kubelet.TLSVerify != nil && !(providerLabel == kubernetes.AKSRoleType && !*config.Kubelet.TLSVerify) {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDKubeletTLSVerify,
				Value: apiutils.BoolToString(config.Kubelet.TLSVerify),
			})
		}
		if config.Kubelet.HostCAPath != "" {
			var agentCAPath string
			// If the user configures a Kubelet CA certificate, it is mounted in AgentCAPath.
			// The default mount value is `/var/run/host-kubelet-ca.crt`, which can be overriden by the user-provided parameter.
			if config.Kubelet.AgentCAPath != "" {
				agentCAPath = config.Kubelet.AgentCAPath
			} else {
				agentCAPath = common.KubeletAgentCAPath
			}
			kubeletVol, kubeletVolMount := volume.GetVolumes(kubeletCAVolumeName, config.Kubelet.HostCAPath, agentCAPath, true)
			if singleContainerStrategyEnabled {
				manager.VolumeMount().AddVolumeMountToContainers(
					&kubeletVolMount,
					[]apicommon.AgentContainerName{
						apicommon.UnprivilegedSingleAgentContainerName,
					},
				)
				manager.Volume().AddVolume(&kubeletVol)
			} else {
				manager.VolumeMount().AddVolumeMountToContainers(
					&kubeletVolMount,
					[]apicommon.AgentContainerName{
						apicommon.CoreAgentContainerName,
						apicommon.ProcessAgentContainerName,
						apicommon.TraceAgentContainerName,
						apicommon.SecurityAgentContainerName,
						apicommon.AgentDataPlaneContainerName,
					},
				)
				manager.Volume().AddVolume(&kubeletVol)
			}
			// If the HostCAPath is overridden, set the environment variable `DD_KUBELET_CLIENT_CA`. The default value in the Agent is `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt`.
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDKubeletCAPath,
				Value: agentCAPath,
			})
		}
		// Configure checks tag cardinality if provided
		if config.ChecksTagCardinality != nil {
			// The value validation happens at the Agent level - if the lower(string) is not `low`, `orchestrator` or `high`, the Agent defaults to `low`.
			// Ref: https://github.com/DataDog/datadog-agent/blob/1d08a6a9783fe271ea3813ddf9abf60244abdf2c/comp/core/tagger/taggerimpl/tagger.go#L173-L177
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDChecksTagCardinality,
				Value: *config.ChecksTagCardinality,
			})
		}
	}

	var runtimeVol corev1.Volume
	var runtimeVolMount corev1.VolumeMount
	// Path to the docker runtime socket.
	if config.DockerSocketPath != nil {
		dockerMountPath := filepath.Join(common.HostCriSocketPathPrefix, *config.DockerSocketPath)
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  DockerHost,
			Value: "unix://" + dockerMountPath,
		})
		runtimeVol, runtimeVolMount = volume.GetVolumes(common.CriSocketVolumeName, *config.DockerSocketPath, dockerMountPath, true)
	} else if config.CriSocketPath != nil {
		// Path to the container runtime socket (if different from Docker).
		criSocketMountPath := filepath.Join(common.HostCriSocketPathPrefix, *config.CriSocketPath)
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  DDCriSocketPath,
			Value: criSocketMountPath,
		})
		runtimeVol, runtimeVolMount = volume.GetVolumes(common.CriSocketVolumeName, *config.CriSocketPath, criSocketMountPath, true)
	}
	if runtimeVol.Name != "" && runtimeVolMount.Name != "" {
		if singleContainerStrategyEnabled {
			manager.VolumeMount().AddVolumeMountToContainers(
				&runtimeVolMount,
				[]apicommon.AgentContainerName{
					apicommon.UnprivilegedSingleAgentContainerName,
				},
			)
			manager.Volume().AddVolume(&runtimeVol)
		} else {
			manager.VolumeMount().AddVolumeMountToContainers(
				&runtimeVolMount,
				[]apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
					apicommon.ProcessAgentContainerName,
					apicommon.TraceAgentContainerName,
					apicommon.SecurityAgentContainerName,
					apicommon.AgentDataPlaneContainerName,
				},
			)
			manager.Volume().AddVolume(&runtimeVol)
		}
	}

}
