// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	corev1 "k8s.io/api/core/v1"
)

// BuilderOptions corresponds to Builders options.
type BuilderOptions struct {
	AllowOverride bool
}

// VolumeBuilder used to generate a list of Volume.
type VolumeBuilder struct {
	options BuilderOptions
	volumes []corev1.Volume
}

// VolumeMountBuilder used to generate a list of VolumeMount.
type VolumeMountBuilder struct {
	options      BuilderOptions
	volumeMounts []corev1.VolumeMount
}

// EnvVarBuilder used to generate a list of EnvVar.
type EnvVarBuilder struct {
	options BuilderOptions
	envVars []corev1.EnvVar
}

// DefaultBuilderOptions returns a default BuilderOptions instance.
func DefaultBuilderOptions() BuilderOptions {
	return BuilderOptions{
		AllowOverride: true,
	}
}

// NewVolumeBuilder returns a new VolumeBuilder instance.
func NewVolumeBuilder(volumes []corev1.Volume, opts *BuilderOptions) *VolumeBuilder {
	builder := &VolumeBuilder{
		volumes: volumes,
		options: DefaultBuilderOptions(),
	}
	if opts != nil {
		builder.options = *opts
	}

	return builder
}

// NewVolumeMountBuilder returns a new VolumeMountBuilder instance.
func NewVolumeMountBuilder(volumeMounts []corev1.VolumeMount, opts *BuilderOptions) *VolumeMountBuilder {
	builder := &VolumeMountBuilder{
		volumeMounts: volumeMounts,
		options:      DefaultBuilderOptions(),
	}
	if opts != nil {
		builder.options = *opts
	}

	return builder
}

// NewEnvVarsBuilder returns a new EnvVarsBuilder instance.
func NewEnvVarsBuilder(envVars []corev1.EnvVar, opts *BuilderOptions) *EnvVarBuilder {
	builder := &EnvVarBuilder{
		envVars: envVars,
		options: DefaultBuilderOptions(),
	}
	if opts != nil {
		builder.options = *opts
	}

	return builder
}

// Add used to add an EnvVar to the EnvVarBuilder.
func (b *EnvVarBuilder) Add(iEnvVar *corev1.EnvVar) *EnvVarBuilder {
	found := false
	for id, envVar := range b.envVars {
		if envVar.Name == iEnvVar.Name {
			found = true
			if b.options.AllowOverride {
				b.envVars[id] = *iEnvVar
			}
		}
	}
	if !found {
		b.envVars = append(b.envVars, *iEnvVar)
	}

	return b
}

// Remove used to remove an EnvVar to the EnvVarBuilder.
func (b *EnvVarBuilder) Remove(volumeName string) *EnvVarBuilder {
	for id, vol := range b.envVars {
		if vol.Name == volumeName {
			// remove the volume
			copy(b.envVars[id:], b.envVars[id+1:])
			b.envVars = b.envVars[:len(b.envVars)-1]
		}
	}
	return b
}

// Build return the generated EnvVar list.
func (b *EnvVarBuilder) Build() []corev1.EnvVar {
	return b.envVars
}

// Add used to add an Volume to the VolumeBuilder.
func (b *VolumeBuilder) Add(iVolume *corev1.Volume) *VolumeBuilder {
	found := false
	for id, vol := range b.volumes {
		if vol.Name == iVolume.Name {
			found = true
			if b.options.AllowOverride {
				b.volumes[id] = *iVolume
			}
		}
	}
	if !found {
		b.volumes = append(b.volumes, *iVolume)
	}

	return b
}

// Remove used to remove an Volume from the VolumeBuilder.
func (b *VolumeBuilder) Remove(volumeName string) *VolumeBuilder {
	for id, vol := range b.volumes {
		if vol.Name == volumeName {
			// remove the volume
			copy(b.volumes[id:], b.volumes[id+1:])
			b.volumes = b.volumes[:len(b.volumes)-1]
		}
	}
	return b
}

// Build used to generate a list of Volume.
func (b *VolumeBuilder) Build() []corev1.Volume {
	return b.volumes
}

// Add used to add an VolumeMount to the VolumeMountBuilder.
func (b *VolumeMountBuilder) Add(iVolumeMount *corev1.VolumeMount) *VolumeMountBuilder {
	found := false
	for id, vol := range b.volumeMounts {
		if vol.Name == iVolumeMount.Name {
			found = true
			if b.options.AllowOverride {
				b.volumeMounts[id] = *iVolumeMount
			}
		}
	}
	if !found {
		b.volumeMounts = append(b.volumeMounts, *iVolumeMount)
	}

	return b
}

// Remove used to remove an VolumeMount from the VolumeMountBuilder.
func (b *VolumeMountBuilder) Remove(volumeName string) *VolumeMountBuilder {
	for id, vol := range b.volumeMounts {
		if vol.Name == volumeName {
			// remove the volume
			copy(b.volumeMounts[id:], b.volumeMounts[id+1:])
			b.volumeMounts = b.volumeMounts[:len(b.volumeMounts)-1]
		}
	}
	return b
}

// Build used to generate a list of VolumeMount
func (b *VolumeMountBuilder) Build() []corev1.VolumeMount {
	return b.volumeMounts
}
