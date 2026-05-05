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

// globalNodeAgentSpec holds all provider-conditional mutations for the node
// agent pod template.
type globalNodeAgentSpec struct {
	Volumes        []feature.ProviderRule[feature.VolumeAndMount]
	EnvVars        []feature.ProviderRule[feature.EnvVarSet]
	ImageRegistry  []feature.ProviderRule[string]
	PodLabels      []feature.ProviderRule[map[string]string]
	PodAnnotations []feature.ProviderRule[map[string]string]
	// RemoveVolumes lists volume names to strip entirely (volume + all mounts) for a provider.
	// Applied inside ApplyGlobalNodeAgentSpec, before features run.
	RemoveVolumes []feature.ProviderRule[string]
	// RemoveMounts lists specific container-volume mount pairs to strip for a provider.
	// Applied inside ApplyGlobalNodeAgentSpec, before features run.
	RemoveMounts []feature.ProviderRule[feature.ContainerMountRef]
	// ContainerResources sets default resource requests/limits per container.
	// Applied with "fill if zero" semantics after DDA override runs, so explicit DDA values always win.
	ContainerResources []feature.ProviderRule[containerResourceDefault]
	// CRISocketRoot is the host CRI socket directory; used to fix the init-config mount path.
	CRISocketRoot string
}

// nodeAgentGlobalSpec builds the globalNodeAgentSpec for the given provider.
func nodeAgentGlobalSpec(provider string) globalNodeAgentSpec {
	criSocketRoot := common.RuntimeDirVolumePath
	hostDataRoot := "/var/lib/datadog-agent"
	if provider == kubernetes.GKEAutopilotProvider {
		criSocketRoot = "/var/run/containerd"
		hostDataRoot = "/var/autopilot/addon/datadog"
	}

	// CRI socket: HostPath replaces the default /var/run volume on Autopilot
	criSocketVol, criSocketMount := objvolume.GetVolumes(
		common.CriSocketVolumeName,
		criSocketRoot,
		common.HostCriSocketPathPrefix+criSocketRoot,
		true,
	)

	// pointerdir: on Autopilot replace the default EmptyDir with a HostPath
	pointerdirVol, pointerdirMount := objvolume.GetVolumes(
		common.RunPathVolumeName,
		hostDataRoot,
		common.RunPathVolumeMount,
		false,
	)

	spec := globalNodeAgentSpec{
		CRISocketRoot: criSocketRoot,
		Volumes: []feature.ProviderRule[feature.VolumeAndMount]{
			// pointerdir: override EmptyDir with HostPath on Autopilot
			{
				Item: feature.VolumeAndMount{
					Volume:     pointerdirVol,
					Mount:      pointerdirMount,
					Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
				},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
			// CRI socket: override default /var/run path on Autopilot
			{
				Item: feature.VolumeAndMount{
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
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
		},
		EnvVars: []feature.ProviderRule[feature.EnvVarSet]{
			// DD_AUTH_TOKEN_FILE_PATH: all containers except Autopilot
			{
				Item: feature.EnvVarSet{
					EnvVar: corev1.EnvVar{
						Name:  common.DDAuthTokenFilePath,
						Value: filepath.Join(common.AuthVolumePath, "token"),
					},
				},
				ExcludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
			// GKE Autopilot: cloud provider metadata env var
			{
				Item: feature.EnvVarSet{
					EnvVar: corev1.EnvVar{
						Name:  "DD_CLOUD_PROVIDER_METADATA",
						Value: `["gcp"]`,
					},
				},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
			// GKE Autopilot: disable HTTPS kubelet port (Autopilot uses HTTP)
			{
				Item: feature.EnvVarSet{
					EnvVar: corev1.EnvVar{
						Name:  "DD_KUBERNETES_HTTPS_KUBELET_PORT",
						Value: "0",
					},
				},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
			// GKE Autopilot: provider kind tag consumed by the agent
			{
				Item: feature.EnvVarSet{
					EnvVar: corev1.EnvVar{
						Name:  "DD_PROVIDER_KIND",
						Value: kubernetes.GKEAutopilotProvider,
					},
				},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
		},
		ImageRegistry: []feature.ProviderRule[string]{
			{
				Item:             "gcr.io/datadoghq",
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
		},
		PodLabels: []feature.ProviderRule[map[string]string]{
			{
				Item:             map[string]string{"env.datadoghq.com/kind": kubernetes.GKEAutopilotProvider},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
		},
		PodAnnotations: []feature.ProviderRule[map[string]string]{
			{
				Item:             map[string]string{"autopilot.gke.io/no-connect": "true"},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
		},
		// RemoveVolumes and RemoveMounts strip base-template volumes before features run.
		RemoveVolumes: []feature.ProviderRule[string]{
			// auth-token volume is absent on GKE Autopilot (not in the Workload Allowlist).
			{Item: common.AuthVolumeName, IncludeProviders: []string{kubernetes.GKEAutopilotProvider}},
			// tmp EmptyDir is not permitted on GKE Autopilot (not in the Workload Allowlist).
			{Item: common.TmpVolumeName, IncludeProviders: []string{kubernetes.GKEAutopilotProvider}},
		},
		RemoveMounts: []feature.ProviderRule[feature.ContainerMountRef]{
			// procdir is not permitted on trace-agent in the Autopilot Workload Allowlist.
			{
				Item:             feature.ContainerMountRef{VolumeName: common.ProcdirVolumeName, Containers: []apicommon.AgentContainerName{apicommon.TraceAgentContainerName}},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
			// cgroups is not permitted on trace-agent in the Autopilot Workload Allowlist.
			{
				Item:             feature.ContainerMountRef{VolumeName: common.CgroupsVolumeName, Containers: []apicommon.AgentContainerName{apicommon.TraceAgentContainerName}},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
			// system-probe does not mount pointerdir on GKE Autopilot.
			{
				Item:             feature.ContainerMountRef{VolumeName: common.RunPathVolumeName, Containers: []apicommon.AgentContainerName{apicommon.SystemProbeContainerName}},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
		},
		ContainerResources: []feature.ProviderRule[containerResourceDefault]{
			{
				Item: containerResourceDefault{
					ContainerName: apicommon.CoreAgentContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
			{
				Item: containerResourceDefault{
					ContainerName: apicommon.TraceAgentContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					},
				},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
			{
				Item: containerResourceDefault{
					ContainerName: apicommon.ProcessAgentContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("200Mi"),
						},
					},
				},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
			{
				Item: containerResourceDefault{
					ContainerName: apicommon.SystemProbeContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("400Mi"),
						},
					},
				},
				IncludeProviders: []string{kubernetes.GKEAutopilotProvider},
			},
		},
	}

	return spec
}

// ApplyGlobalNodeAgentSpec applies all provider-conditional mutations for the
// node agent pod template. Call this once, before features run.
func ApplyGlobalNodeAgentSpec(mgr feature.PodTemplateManagers, provider string) {
	spec := nodeAgentGlobalSpec(provider)
	feature.ApplyNodeAgentProviderCapabilities(mgr, provider, feature.NodeAgentProviderCapabilities{
		Volumes:       spec.Volumes,
		EnvVars:       spec.EnvVars,
		RemoveVolumes: spec.RemoveVolumes,
		RemoveMounts:  spec.RemoveMounts,
	})

	for _, rule := range spec.ImageRegistry {
		if feature.ShouldApply(provider, rule.IncludeProviders, rule.ExcludeProviders) {
			overrideImageRegistry(mgr, rule.Item)
		}
	}

	for _, rule := range spec.PodLabels {
		if feature.ShouldApply(provider, rule.IncludeProviders, rule.ExcludeProviders) {
			if mgr.PodTemplateSpec().Labels == nil {
				mgr.PodTemplateSpec().Labels = map[string]string{}
			}
			maps.Copy(mgr.PodTemplateSpec().Labels, rule.Item)
		}
	}

	for _, rule := range spec.PodAnnotations {
		if feature.ShouldApply(provider, rule.IncludeProviders, rule.ExcludeProviders) {
			if mgr.PodTemplateSpec().Annotations == nil {
				mgr.PodTemplateSpec().Annotations = map[string]string{}
			}
			maps.Copy(mgr.PodTemplateSpec().Annotations, rule.Item)
		}
	}

	for _, rule := range spec.ContainerResources {
		if feature.ShouldApply(provider, rule.IncludeProviders, rule.ExcludeProviders) {
			applyDefaultContainerResources(mgr.PodTemplateSpec(), rule.Item)
		}
	}

	// Autopilot imperative overrides that cannot be expressed as ProviderRules.
	if provider == kubernetes.GKEAutopilotProvider {
		applyAutopilotInitContainerOverrides(mgr, spec.CRISocketRoot)
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
