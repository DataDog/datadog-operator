// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package providercaps holds the provider-conditional pod-template mutation
// framework. Both per-feature (feature.ProviderAwareFeature) and global
// (global.ApplyGlobalNodeAgentSpec) consumers declare their mutations as a
// NodeAgentProviderCapabilities map and apply them via
// ApplyNodeAgentProviderCapabilities.
package providercaps

import (
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
)

// PodTemplateManager is the minimal interface ApplyNodeAgentProviderCapabilities
// needs from a pod-template manager. feature.PodTemplateManagers satisfies it
// structurally, so callers pass their existing manager unchanged.
type PodTemplateManager interface {
	PodTemplateSpec() *corev1.PodTemplateSpec
	EnvVar() merger.EnvVarManager
	Volume() merger.VolumeManager
	VolumeMount() merger.VolumeMountManager
}

// VolumeAndMount groups a pod-level volume with a container volume mount.
// Volume is added to the pod spec; Mount is added to each listed container.
type VolumeAndMount struct {
	Volume     corev1.Volume
	Mount      corev1.VolumeMount
	Containers []apicommon.AgentContainerName
}

// EnvVarSet groups an env var with its target containers.
// Empty Containers means the env var is added to all agent containers.
// InitContainers lists init containers that should also receive the env var
// (init containers are not iterated by the all-containers AddEnvVar path).
type EnvVarSet struct {
	EnvVar         corev1.EnvVar
	Containers     []apicommon.AgentContainerName
	InitContainers []apicommon.AgentContainerName
}

// ContainerMountRef identifies a volume mount by volume name and the containers
// it should be stripped from. Empty Containers means strip from all containers.
type ContainerMountRef struct {
	VolumeName string
	Containers []apicommon.AgentContainerName
}

// ProviderCapabilities holds the volumes, env vars, and removals for a
// specific provider entry in a NodeAgentProviderCapabilities map.
type ProviderCapabilities struct {
	Volumes []VolumeAndMount
	EnvVars []EnvVarSet
	// RemoveVolumes strips named volumes (vol + all mounts) before provider additions run.
	RemoveVolumes []string
	// RemoveMounts strips specific container-volume mount pairs before provider additions run.
	RemoveMounts []ContainerMountRef
	// RemoveEnvVars strips env vars by name before provider additions run.
	RemoveEnvVars []string
}

// NodeAgentProviderCapabilities maps a provider string to its capabilities.
// The empty string key "" is the baseline applied to all providers first.
// Provider-specific entries are then applied on top: removals first, additions second.
type NodeAgentProviderCapabilities = map[string]ProviderCapabilities

// ApplyNodeAgentProviderCapabilities applies all provider-conditional mutations.
// The baseline ("") entry is applied first. The provider-specific entry is then
// applied: removals run before additions so a provider can replace a baseline item
// by removing it and re-adding a modified version.
func ApplyNodeAgentProviderCapabilities(mgr PodTemplateManager, provider string, caps NodeAgentProviderCapabilities) {
	if len(caps) == 0 {
		return
	}

	applyAdditions := func(c ProviderCapabilities) {
		addedVolumes := make(map[string]bool)
		for _, vm := range c.Volumes {
			if !addedVolumes[vm.Volume.Name] {
				mgr.Volume().AddVolume(&vm.Volume)
				addedVolumes[vm.Volume.Name] = true
			}
			mgr.VolumeMount().AddVolumeMountToContainers(&vm.Mount, vm.Containers)
		}
		for _, ev := range c.EnvVars {
			if len(ev.Containers) == 0 {
				mgr.EnvVar().AddEnvVar(&ev.EnvVar)
			} else {
				mgr.EnvVar().AddEnvVarToContainers(ev.Containers, &ev.EnvVar)
			}
			for _, ic := range ev.InitContainers {
				mgr.EnvVar().AddEnvVarToInitContainer(ic, &ev.EnvVar)
			}
		}
	}

	applyRemovals := func(c ProviderCapabilities) {
		tmpl := mgr.PodTemplateSpec()
		for _, name := range c.RemoveVolumes {
			stripVolume(tmpl, name)
		}
		for _, ref := range c.RemoveMounts {
			stripMounts(tmpl, ref.VolumeName, ref.Containers)
		}
		for _, name := range c.RemoveEnvVars {
			stripEnvVar(tmpl, name)
		}
	}

	if baseline, ok := caps[""]; ok {
		applyAdditions(baseline)
	}
	if provider != "" {
		if providerCaps, ok := caps[provider]; ok {
			applyRemovals(providerCaps)
			applyAdditions(providerCaps)
		}
	}
}

// stripVolume removes a named volume from the pod spec and all its mounts
// from every container and init container.
func stripVolume(tmpl *corev1.PodTemplateSpec, volumeName string) {
	filtered := tmpl.Spec.Volumes[:0]
	for _, v := range tmpl.Spec.Volumes {
		if v.Name != volumeName {
			filtered = append(filtered, v)
		}
	}
	tmpl.Spec.Volumes = filtered
	stripMounts(tmpl, volumeName, nil)
}

// stripMounts removes the mount for volumeName from the specified containers.
// If containers is nil or empty, the mount is stripped from every container
// and init container.
func stripMounts(tmpl *corev1.PodTemplateSpec, volumeName string, containers []apicommon.AgentContainerName) {
	targetAll := len(containers) == 0
	targetSet := make(map[string]bool, len(containers))
	for _, c := range containers {
		targetSet[string(c)] = true
	}

	for i := range tmpl.Spec.Containers {
		if targetAll || targetSet[tmpl.Spec.Containers[i].Name] {
			tmpl.Spec.Containers[i].VolumeMounts = removeMountByName(tmpl.Spec.Containers[i].VolumeMounts, volumeName)
		}
	}
	for i := range tmpl.Spec.InitContainers {
		if targetAll || targetSet[tmpl.Spec.InitContainers[i].Name] {
			tmpl.Spec.InitContainers[i].VolumeMounts = removeMountByName(tmpl.Spec.InitContainers[i].VolumeMounts, volumeName)
		}
	}
}

// stripEnvVar removes an env var by name from every container and init container.
func stripEnvVar(tmpl *corev1.PodTemplateSpec, name string) {
	for i := range tmpl.Spec.Containers {
		tmpl.Spec.Containers[i].Env = removeEnvVarByName(tmpl.Spec.Containers[i].Env, name)
	}
	for i := range tmpl.Spec.InitContainers {
		tmpl.Spec.InitContainers[i].Env = removeEnvVarByName(tmpl.Spec.InitContainers[i].Env, name)
	}
}

func removeMountByName(mounts []corev1.VolumeMount, name string) []corev1.VolumeMount {
	out := mounts[:0]
	for _, m := range mounts {
		if m.Name != name {
			out = append(out, m)
		}
	}
	return out
}

func removeEnvVarByName(envs []corev1.EnvVar, name string) []corev1.EnvVar {
	out := envs[:0]
	for _, e := range envs {
		if e.Name != name {
			out = append(out, e)
		}
	}
	return out
}
