// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"slices"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// ProviderRule wraps any config item with provider inclusion/exclusion lists.
// Empty IncludeProviders means the rule applies to all providers.
// ExcludeProviders takes priority over IncludeProviders.
type ProviderRule[T any] struct {
	Item             T
	IncludeProviders []string
	ExcludeProviders []string
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
type EnvVarSet struct {
	EnvVar     corev1.EnvVar
	Containers []apicommon.AgentContainerName
}

// ContainerMountRef identifies a volume mount by volume name and the containers
// it should be stripped from. Empty Containers means strip from all containers.
type ContainerMountRef struct {
	VolumeName string
	Containers []apicommon.AgentContainerName
}

// NodeAgentProviderCapabilities holds provider-conditional volumes and env vars
// that a feature contributes to the node agent pod template.
type NodeAgentProviderCapabilities struct {
	Volumes []ProviderRule[VolumeAndMount]
	EnvVars []ProviderRule[EnvVarSet]
	// RemoveVolumes lists volume names to remove entirely (volume + all mounts).
	RemoveVolumes []ProviderRule[string]
	// RemoveMounts lists specific container-volume mount pairs to strip.
	RemoveMounts []ProviderRule[ContainerMountRef]
	// OverrideVolumes replaces named volumes in the pod spec post-feature.
	// Only the volume source is swapped; existing mounts are unaffected since
	// they reference volumes by name.
	OverrideVolumes []ProviderRule[corev1.Volume]
}

// ProviderAwareFeature is an optional interface for features that vary behaviour
// by provider. Features that have no provider-specific variation do not need
// to implement it.
type ProviderAwareFeature interface {
	Feature
	NodeAgentProviderCapabilities() NodeAgentProviderCapabilities
}

// ShouldApply returns true when the rule should be applied for the given provider.
func ShouldApply(provider string, include, exclude []string) bool {
	if slices.Contains(exclude, provider) {
		return false
	}
	if len(include) == 0 {
		return true
	}
	return slices.Contains(include, provider)
}

// ApplyNodeAgentProviderCapabilities applies all provider-conditional mutations
// from a NodeAgentProviderCapabilities in order: additions, then removals, then
// volume source overrides. Call this after ManageNodeAgent so that volumes added
// by the feature are in scope for removal and override.
func ApplyNodeAgentProviderCapabilities(mgr PodTemplateManagers, provider string, caps NodeAgentProviderCapabilities) {
	addedVolumes := make(map[string]bool)
	for _, rule := range caps.Volumes {
		if ShouldApply(provider, rule.IncludeProviders, rule.ExcludeProviders) {
			if !addedVolumes[rule.Item.Volume.Name] {
				mgr.Volume().AddVolume(&rule.Item.Volume)
				addedVolumes[rule.Item.Volume.Name] = true
			}
			mgr.VolumeMount().AddVolumeMountToContainers(&rule.Item.Mount, rule.Item.Containers)
		}
	}
	for _, rule := range caps.EnvVars {
		if ShouldApply(provider, rule.IncludeProviders, rule.ExcludeProviders) {
			if len(rule.Item.Containers) == 0 {
				mgr.EnvVar().AddEnvVar(&rule.Item.EnvVar)
			} else {
				mgr.EnvVar().AddEnvVarToContainers(rule.Item.Containers, &rule.Item.EnvVar)
			}
		}
	}
	tmpl := mgr.PodTemplateSpec()
	for _, rule := range caps.RemoveVolumes {
		if ShouldApply(provider, rule.IncludeProviders, rule.ExcludeProviders) {
			stripVolume(tmpl, rule.Item)
		}
	}
	for _, rule := range caps.RemoveMounts {
		if ShouldApply(provider, rule.IncludeProviders, rule.ExcludeProviders) {
			stripMounts(tmpl, rule.Item.VolumeName, rule.Item.Containers)
		}
	}
	for _, rule := range caps.OverrideVolumes {
		if ShouldApply(provider, rule.IncludeProviders, rule.ExcludeProviders) {
			for i := range tmpl.Spec.Volumes {
				if tmpl.Spec.Volumes[i].Name == rule.Item.Name {
					tmpl.Spec.Volumes[i] = rule.Item
					break
				}
			}
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
	// nil means all containers
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

func removeMountByName(mounts []corev1.VolumeMount, name string) []corev1.VolumeMount {
	out := mounts[:0]
	for _, m := range mounts {
		if m.Name != name {
			out = append(out, m)
		}
	}
	return out
}
