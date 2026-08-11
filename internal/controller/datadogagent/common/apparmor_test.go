// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestFinalizeAppArmorProfile(t *testing.T) {
	t.Run("keeps annotations before Kubernetes 1.30", func(t *testing.T) {
		podTemplate := appArmorPodTemplate()
		platformInfo := kubernetes.NewPlatformInfoFromVersionMaps(&version.Info{GitVersion: "v1.29.9"}, nil, nil)

		FinalizeAppArmorProfile(&podTemplate, platformInfo)

		assert.Equal(t, "unconfined", podTemplate.Annotations[AppArmorAnnotationKey+"/system-probe"])
		assert.Nil(t, podTemplate.Spec.Containers[0].SecurityContext)
	})

	t.Run("migrates annotations on Kubernetes 1.30 and later", func(t *testing.T) {
		podTemplate := appArmorPodTemplate()
		platformInfo := kubernetes.NewPlatformInfoFromVersionMaps(&version.Info{GitVersion: "v1.30.0"}, nil, nil)

		FinalizeAppArmorProfile(&podTemplate, platformInfo)

		assert.Equal(t, map[string]string{"other": "annotation"}, podTemplate.Annotations)

		systemProbeProfile := requireAppArmorProfile(t, podTemplate.Spec.Containers[0])
		assert.Equal(t, corev1.AppArmorProfileTypeUnconfined, systemProbeProfile.Type)
		assert.Nil(t, systemProbeProfile.LocalhostProfile)

		agentProfile := requireAppArmorProfile(t, podTemplate.Spec.Containers[1])
		assert.Equal(t, corev1.AppArmorProfileTypeRuntimeDefault, agentProfile.Type)
		assert.Nil(t, agentProfile.LocalhostProfile)

		processAgentProfile := requireAppArmorProfile(t, podTemplate.Spec.Containers[2])
		assert.Equal(t, corev1.AppArmorProfileTypeLocalhost, processAgentProfile.Type)
		require.NotNil(t, processAgentProfile.LocalhostProfile)
		assert.Equal(t, "custom-profile", *processAgentProfile.LocalhostProfile)

		initProfile := requireAppArmorProfile(t, podTemplate.Spec.InitContainers[0])
		assert.Equal(t, corev1.AppArmorProfileTypeLocalhost, initProfile.Type)
		require.NotNil(t, initProfile.LocalhostProfile)
		assert.Equal(t, "bare-custom-profile", *initProfile.LocalhostProfile)
	})

	t.Run("preserves an explicit appArmorProfile field", func(t *testing.T) {
		podTemplate := corev1.PodTemplateSpec{
			ObjectMeta: appArmorPodTemplate().ObjectMeta,
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "system-probe",
				SecurityContext: &corev1.SecurityContext{AppArmorProfile: &corev1.AppArmorProfile{
					Type: corev1.AppArmorProfileTypeRuntimeDefault,
				}},
			}}},
		}
		platformInfo := kubernetes.NewPlatformInfoFromVersionMaps(&version.Info{GitVersion: "v1.31.0"}, nil, nil)

		FinalizeAppArmorProfile(&podTemplate, platformInfo)

		profile := requireAppArmorProfile(t, podTemplate.Spec.Containers[0])
		assert.Equal(t, corev1.AppArmorProfileTypeRuntimeDefault, profile.Type)
		_, found := podTemplate.Annotations[AppArmorAnnotationKey+"/system-probe"]
		assert.False(t, found)
	})

	t.Run("maps an empty annotation value to runtime default", func(t *testing.T) {
		podTemplate := corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				AppArmorAnnotationKey + "/agent": "",
			}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent"}}},
		}
		platformInfo := kubernetes.NewPlatformInfoFromVersionMaps(&version.Info{GitVersion: "v1.30.0"}, nil, nil)

		FinalizeAppArmorProfile(&podTemplate, platformInfo)

		profile := requireAppArmorProfile(t, podTemplate.Spec.Containers[0])
		assert.Equal(t, corev1.AppArmorProfileTypeRuntimeDefault, profile.Type)
		assert.Nil(t, profile.LocalhostProfile)
		assert.NotNil(t, podTemplate.Annotations)
		assert.Empty(t, podTemplate.Annotations)
	})
}

func appArmorPodTemplate() corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			AppArmorAnnotationKey + "/system-probe":  "unconfined",
			AppArmorAnnotationKey + "/agent":         "runtime/default",
			AppArmorAnnotationKey + "/process-agent": "localhost/custom-profile",
			AppArmorAnnotationKey + "/init":          "bare-custom-profile",
			AppArmorAnnotationKey + "/absent":        "unconfined",
			"other":                                  "annotation",
		}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "system-probe"},
				{Name: "agent"},
				{Name: "process-agent"},
			},
			InitContainers: []corev1.Container{{Name: "init"}},
		},
	}
}

func requireAppArmorProfile(t *testing.T, container corev1.Container) *corev1.AppArmorProfile {
	t.Helper()
	require.NotNil(t, container.SecurityContext)
	require.NotNil(t, container.SecurityContext.AppArmorProfile)
	return container.SecurityContext.AppArmorProfile
}
