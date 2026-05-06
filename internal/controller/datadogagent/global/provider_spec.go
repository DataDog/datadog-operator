// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"maps"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	objvolume "github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// containerResourceDefault specifies default resource requests/limits for a named container.
// Applied with "fill if zero" semantics — only written when the container has no existing requests/limits.
type containerResourceDefault struct {
	ContainerName apicommon.AgentContainerName
	Resources     corev1.ResourceRequirements
}

// ProviderAnnotationKey is the annotation on a DDAI that declares the provider string.
// The POC drives provider detection from this annotation rather than node labels.
const ProviderAnnotationKey = "datadoghq.com/provider"

// globalProviderCapabilities extends ProviderCapabilities with fields that are
// global-spec-specific (not available in the feature-level ProviderCapabilities).
type globalProviderCapabilities struct {
	feature.ProviderCapabilities
	ImageRegistry      string
	PodLabels          map[string]string
	PodAnnotations     map[string]string
	ContainerResources []containerResourceDefault
}

// nodeAgentGlobalSpec builds the full provider registry for the node agent pod
// template. The "" key is the baseline applied to all providers; provider-keyed
// entries are applied on top (removals first, then additions).
func nodeAgentGlobalSpec() map[string]globalProviderCapabilities {
	criSocketVol, criSocketMount := objvolume.GetVolumes(
		common.CriSocketVolumeName,
		"/var/run/containerd",
		common.HostCriSocketPathPrefix+"/var/run/containerd",
		true,
	)
	pointerdirVol, pointerdirMount := objvolume.GetVolumes(
		common.RunPathVolumeName,
		"/var/autopilot/addon/datadog",
		common.RunPathVolumeMount,
		false,
	)

	return map[string]globalProviderCapabilities{
		"": {
			ProviderCapabilities: feature.ProviderCapabilities{
				EnvVars: []feature.EnvVarSet{
					{
						EnvVar: corev1.EnvVar{
							Name:  common.DDAuthTokenFilePath,
							Value: filepath.Join(common.AuthVolumePath, "token"),
						},
					},
				},
			},
		},
		kubernetes.GKEAutopilotProvider: {
			ContainerResources: []containerResourceDefault{
				{
					ContainerName: apicommon.CoreAgentContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
				{
					ContainerName: apicommon.TraceAgentContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					},
				},
				{
					ContainerName: apicommon.ProcessAgentContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					},
				},
				{
					ContainerName: apicommon.SystemProbeContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("400Mi"),
						},
					},
				},
			},
			ProviderCapabilities: feature.ProviderCapabilities{
				Volumes: []feature.VolumeAndMount{
					// pointerdir: replace default EmptyDir with Autopilot HostPath
					{
						Volume:     pointerdirVol,
						Mount:      pointerdirMount,
						Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
					},
					// CRI socket: override /var/run with Autopilot containerd path
					{
						Volume: criSocketVol,
						Mount:  criSocketMount,
						Containers: []apicommon.AgentContainerName{
							apicommon.CoreAgentContainerName,
							apicommon.ProcessAgentContainerName,
							apicommon.TraceAgentContainerName,
							apicommon.SecurityAgentContainerName,
							apicommon.AgentDataPlaneContainerName,
						},
					},
				},
				EnvVars: []feature.EnvVarSet{
					{
						EnvVar: corev1.EnvVar{
							Name:  "DD_CLOUD_PROVIDER_METADATA",
							Value: `["gcp"]`,
						},
					},
					{
						EnvVar: corev1.EnvVar{
							Name:  "DD_KUBERNETES_HTTPS_KUBELET_PORT",
							Value: "0",
						},
					},
					{
						EnvVar: corev1.EnvVar{
							Name:  "DD_PROVIDER_KIND",
							Value: kubernetes.GKEAutopilotProvider,
						},
					},
				},
				RemoveVolumes: []string{
					// auth-token is absent on GKE Autopilot (not in the Workload Allowlist).
					common.AuthVolumeName,
					// tmp EmptyDir is not permitted on GKE Autopilot.
					common.TmpVolumeName,
				},
				RemoveMounts: []feature.ContainerMountRef{
					// procdir is not permitted on trace-agent in the Autopilot Workload Allowlist.
					{VolumeName: common.ProcdirVolumeName, Containers: []apicommon.AgentContainerName{apicommon.TraceAgentContainerName}},
					// cgroups is not permitted on trace-agent in the Autopilot Workload Allowlist.
					{VolumeName: common.CgroupsVolumeName, Containers: []apicommon.AgentContainerName{apicommon.TraceAgentContainerName}},
					// system-probe does not mount pointerdir on GKE Autopilot.
					{VolumeName: common.RunPathVolumeName, Containers: []apicommon.AgentContainerName{apicommon.SystemProbeContainerName}},
				},
				RemoveEnvVars: []string{
					// auth-token env var is absent on Autopilot (volume removed above).
					common.DDAuthTokenFilePath,
				},
			},
			ImageRegistry:  "gcr.io/datadoghq",
			PodLabels:      map[string]string{"env.datadoghq.com/kind": kubernetes.GKEAutopilotProvider},
			PodAnnotations: map[string]string{"autopilot.gke.io/no-connect": "true"},
		},
	}
}

// ApplyGlobalNodeAgentSpec applies all provider-conditional mutations for the
// node agent pod template. Call this once, before features run.
func ApplyGlobalNodeAgentSpec(mgr feature.PodTemplateManagers, provider string) {
	spec := nodeAgentGlobalSpec()

	// Extract the common ProviderCapabilities for each entry and delegate to
	// the feature-level applier (handles baseline + provider removals/additions).
	featureCaps := make(feature.NodeAgentProviderCapabilities, len(spec))
	for k, v := range spec {
		featureCaps[k] = v.ProviderCapabilities
	}
	feature.ApplyNodeAgentProviderCapabilities(mgr, provider, featureCaps)

	// Apply provider-specific global fields.
	if provider != "" {
		if providerCaps, ok := spec[provider]; ok {
			for _, r := range providerCaps.ContainerResources {
				applyDefaultContainerResources(mgr.PodTemplateSpec(), r)
			}
			if providerCaps.ImageRegistry != "" {
				overrideImageRegistry(mgr, providerCaps.ImageRegistry)
			}
			if providerCaps.PodLabels != nil {
				if mgr.PodTemplateSpec().Labels == nil {
					mgr.PodTemplateSpec().Labels = map[string]string{}
				}
				maps.Copy(mgr.PodTemplateSpec().Labels, providerCaps.PodLabels)
			}
			if providerCaps.PodAnnotations != nil {
				if mgr.PodTemplateSpec().Annotations == nil {
					mgr.PodTemplateSpec().Annotations = map[string]string{}
				}
				maps.Copy(mgr.PodTemplateSpec().Annotations, providerCaps.PodAnnotations)
			}
		}
	}

	// Imperative overrides that cannot be expressed as ProviderCapabilities entries.
	if provider == kubernetes.GKEAutopilotProvider {
		applyAutopilotInitContainerOverrides(mgr, "/var/run/containerd")
		applyAutopilotContainerCommandOverrides(mgr)
	}
}

// applyDefaultContainerResources sets resource requests/limits on a container only when
// the container is present and has no existing requests or limits set.
func applyDefaultContainerResources(tmpl *corev1.PodTemplateSpec, d containerResourceDefault) {
	for i := range tmpl.Spec.Containers {
		if tmpl.Spec.Containers[i].Name != string(d.ContainerName) {
			continue
		}
		c := &tmpl.Spec.Containers[i]
		if c.Resources.Requests == nil && d.Resources.Requests != nil {
			c.Resources.Requests = d.Resources.Requests.DeepCopy()
		}
		if c.Resources.Limits == nil && d.Resources.Limits != nil {
			c.Resources.Limits = d.Resources.Limits.DeepCopy()
		}
		return
	}
}

// applyAutopilotInitContainerOverrides fixes init-volume args and the init-config
// CRI socket mount path for GKE Autopilot.
func applyAutopilotInitContainerOverrides(mgr feature.PodTemplateManagers, criSocketRoot string) {
	criMountPath := common.HostCriSocketPathPrefix + criSocketRoot
	for i := range mgr.PodTemplateSpec().Spec.InitContainers {
		switch mgr.PodTemplateSpec().Spec.InitContainers[i].Name {
		case "init-volume":
			// Autopilot allowlist requires no -vn flags.
			mgr.PodTemplateSpec().Spec.InitContainers[i].Args = []string{"cp -r /etc/datadog-agent /opt"}
		case string(apicommon.InitConfigContainerName):
			// Remap the CRI socket mount to the Autopilot-specific path.
			for j := range mgr.PodTemplateSpec().Spec.InitContainers[i].VolumeMounts {
				if mgr.PodTemplateSpec().Spec.InitContainers[i].VolumeMounts[j].Name == common.CriSocketVolumeName {
					mgr.PodTemplateSpec().Spec.InitContainers[i].VolumeMounts[j].MountPath = criMountPath
				}
			}
		case "seccomp-setup":
			// Autopilot allowlist requires the seccomp-security ConfigMap mount to be read-only.
			for j := range mgr.PodTemplateSpec().Spec.InitContainers[i].VolumeMounts {
				if mgr.PodTemplateSpec().Spec.InitContainers[i].VolumeMounts[j].Name == common.SeccompSecurityVolumeName {
					mgr.PodTemplateSpec().Spec.InitContainers[i].VolumeMounts[j].ReadOnly = true
				}
			}
		}
	}
}

// applyAutopilotContainerCommandOverrides sets the Autopilot-allowlisted commands
// for trace-agent and process-agent, and removes the startup probe from core-agent
// (startup probes are unsupported on Autopilot).
func applyAutopilotContainerCommandOverrides(mgr feature.PodTemplateManagers) {
	for i := range mgr.PodTemplateSpec().Spec.Containers {
		switch mgr.PodTemplateSpec().Spec.Containers[i].Name {
		case string(apicommon.CoreAgentContainerName):
			mgr.PodTemplateSpec().Spec.Containers[i].StartupProbe = nil
		case string(apicommon.TraceAgentContainerName):
			mgr.PodTemplateSpec().Spec.Containers[i].Command = []string{"trace-agent", "-config=/etc/datadog-agent/datadog.yaml"}
		case string(apicommon.ProcessAgentContainerName):
			mgr.PodTemplateSpec().Spec.Containers[i].Command = []string{"process-agent", "-config=/etc/datadog-agent/datadog.yaml"}
		}
	}
}

func overrideImageRegistry(mgr feature.PodTemplateManagers, registry string) {
	if registry == "" {
		return
	}
	for i, c := range mgr.PodTemplateSpec().Spec.Containers {
		if c.Image == "" {
			continue
		}
		mgr.PodTemplateSpec().Spec.Containers[i].Image = images.FromString(c.Image).WithRegistry(registry).ToString()
	}
	for i, c := range mgr.PodTemplateSpec().Spec.InitContainers {
		if c.Image == "" {
			continue
		}
		mgr.PodTemplateSpec().Spec.InitContainers[i].Image = images.FromString(c.Image).WithRegistry(registry).ToString()
	}
}
