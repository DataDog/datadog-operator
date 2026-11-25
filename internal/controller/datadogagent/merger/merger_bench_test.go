// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
)

func newBenchPodTemplate() *corev1.PodTemplateSpec {
	return &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: string(common.CoreAgentContainerName)},
				{Name: string(common.TraceAgentContainerName)},
				{Name: string(common.ProcessAgentContainerName)},
				{Name: string(common.SecurityAgentContainerName)},
				{Name: string(common.SystemProbeContainerName)},
			},
			InitContainers: []corev1.Container{
				{Name: string(common.InitVolumeContainerName)},
				{Name: string(common.InitConfigContainerName)},
			},
		},
	}
}

func BenchmarkEnvVarManager_Add60EnvVars(b *testing.B) {
	globalEnvs := make([]*corev1.EnvVar, 20)
	for i := range globalEnvs {
		globalEnvs[i] = &corev1.EnvVar{
			Name:  fmt.Sprintf("DD_GLOBAL_VAR_%02d", i),
			Value: fmt.Sprintf("global-val-%02d", i),
		}
	}
	containerEnvs := make([]*corev1.EnvVar, 40)
	for i := range containerEnvs {
		containerEnvs[i] = &corev1.EnvVar{
			Name:  fmt.Sprintf("DD_FEATURE_VAR_%02d", i),
			Value: fmt.Sprintf("feature-val-%02d", i),
		}
	}
	overrideEnvs := make([]*corev1.EnvVar, 10)
	for i := range overrideEnvs {
		overrideEnvs[i] = &corev1.EnvVar{
			Name:  fmt.Sprintf("DD_FEATURE_VAR_%02d", i),
			Value: fmt.Sprintf("override-val-%02d", i),
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		podTmpl := newBenchPodTemplate()
		mgr := NewEnvVarManager(podTmpl)
		for _, e := range globalEnvs {
			mgr.AddEnvVar(e)
		}
		for _, e := range containerEnvs {
			mgr.AddEnvVarToContainer(common.CoreAgentContainerName, e)
		}
		for _, e := range overrideEnvs {
			mgr.AddEnvVarToContainer(common.CoreAgentContainerName, e)
		}
	}
}

func BenchmarkVolumeManager_Add30Volumes(b *testing.B) {
	vols := make([]*corev1.Volume, 30)
	for i := range vols {
		vols[i] = &corev1.Volume{
			Name: fmt.Sprintf("vol-%02d", i),
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/run/datadog/%02d", i),
				},
			},
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		podTmpl := newBenchPodTemplate()
		mgr := NewVolumeManager(podTmpl)
		for _, v := range vols {
			mgr.AddVolume(v)
		}
		// Simulate 5 duplicate/override volumes
		for _, v := range vols[:5] {
			mgr.AddVolume(v)
		}
	}
}

func BenchmarkVolumeMountManager_Add30VolumeMounts(b *testing.B) {
	mounts := make([]*corev1.VolumeMount, 30)
	for i := range mounts {
		mounts[i] = &corev1.VolumeMount{
			Name:      fmt.Sprintf("vol-%02d", i),
			MountPath: fmt.Sprintf("/etc/datadog-agent/vol-%02d", i),
			ReadOnly:  true,
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		podTmpl := newBenchPodTemplate()
		mgr := NewVolumeMountManager(podTmpl)
		for _, m := range mounts[:10] {
			mgr.AddVolumeMount(m)
		}
		for _, m := range mounts[10:] {
			mgr.AddVolumeMountToContainer(m, common.CoreAgentContainerName)
		}
		for _, m := range mounts[:5] {
			mgr.AddVolumeMountToContainer(m, common.CoreAgentContainerName)
		}
	}
}
