// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	apiequality "k8s.io/apimachinery/pkg/api/equality"

	"github.com/DataDog/datadog-operator/pkg/testutils"
)

// CheckPodTemplateFunc define the signature of a check function against a corev1.PodTemplateSpec
type CheckPodTemplateFunc func(t *testing.T, podTemplate *corev1.PodTemplateSpec)

// CheckContainerFunc define the signature of a check function against a corev1.Container
type CheckContainerFunc func(t *testing.T, container *corev1.Container)

// CheckContainerInPodTemplate used to execute a CheckContainerFunc function again a specific container in a corev1.PodTemplateSpec
// object.
func CheckContainerInPodTemplate(containerName string, checkFunc CheckContainerFunc) CheckPodTemplateFunc {
	check := func(t *testing.T, podTemplate *corev1.PodTemplateSpec) {
		for _, container := range podTemplate.Spec.Containers {
			if container.Name == containerName {
				checkFunc(t, &container)
			}
		}
		t.Errorf("Container %s not founded", containerName)
	}
	return check
}

// CheckVolumeIsPresent used to check if a corev1.Volume is present in a corev1.PodTemplateSpec object.
func CheckVolumeIsPresent(volume *corev1.Volume) CheckPodTemplateFunc {
	check := func(t *testing.T, podTemplate *corev1.PodTemplateSpec) {
		for _, volumeIn := range podTemplate.Spec.Volumes {
			if apiequality.Semantic.DeepEqual(&volume, volumeIn) {
				t.Logf("Volume %s founded", volume.Name)
				return
			}
		}
		t.Errorf("Volume %s not founded", volume.Name)
	}
	return check
}

// CheckVolumeIsNotPresent used to check if a corev1.Volume is not present in a corev1.PodTemplateSpec object.
func CheckVolumeIsNotPresent(volume *corev1.Volume) CheckPodTemplateFunc {
	check := func(t *testing.T, podTemplate *corev1.PodTemplateSpec) {
		found := false
		for _, volumeIn := range podTemplate.Spec.Volumes {
			if diff := testutils.CompareKubeResource(&volume, volumeIn); diff == "" {
				found = true
				break
			}
		}
		if found {
			t.Errorf("Volume %s founded", volume.Name)
		} else {
			t.Errorf("Volume %s not founded", volume.Name)
		}
	}
	return check
}

// CheckContainerDeepEqualIsPresent used to check if corev1.Container is equal to a container inside a corev1.PodTemplateSpec object.
func CheckContainerDeepEqualIsPresent(container *corev1.Container) CheckPodTemplateFunc {
	check := func(t *testing.T, podTemplate *corev1.PodTemplateSpec) {
		for _, containerIn := range podTemplate.Spec.Containers {
			if diff := testutils.CompareKubeResource(container, &containerIn); diff != "" {
				t.Logf("Volume %s founded", container.Name)
				return
			}
		}
		t.Errorf("Volume %s not founded", container.Name)
	}
	return check
}

// CheckContainerNameIsPresentFunc used to check if container name is equal to a container name
// present in a corev1.PodTemplateSpec object.
func CheckContainerNameIsPresentFunc(containerName string) CheckPodTemplateFunc {
	check := func(t *testing.T, podTemplate *corev1.PodTemplateSpec) {
		for _, containerIn := range podTemplate.Spec.Containers {
			if containerIn.Name == containerName {
				t.Logf("Volume %s founded", containerIn.Name)
				return
			}
		}
		t.Errorf("Volume %s not founded", containerName)
	}
	return check
}

// CheckEnvVarIsPresent used to check if an corev1.EnvVar is present in a corev1.Container object
func CheckEnvVarIsPresent(envVar *corev1.EnvVar) CheckContainerFunc {
	check := func(t *testing.T, container *corev1.Container) {
		for _, envVarIn := range container.Env {
			if diff := testutils.CompareKubeResource(&envVarIn, envVar); diff == "" {
				t.Logf("EnvVar %s founded in container %s", envVar.Name, container.Name)
				break
			}
		}
		t.Errorf("EnvVar %s not founded in container %s", envVar.Name, container.Name)
	}
	return check
}

// CheckEnvFromIsPresent used to check if an corev1.EnvFromSource is present in a corev1.Container object
func CheckEnvFromIsPresent(envFrom *corev1.EnvFromSource) CheckContainerFunc {
	check := func(t *testing.T, container *corev1.Container) {
		for _, envVarIn := range container.EnvFrom {
			if diff := testutils.CompareKubeResource(&envVarIn, envFrom); diff == "" {
				t.Logf("EnvVar [%s] founded in container %s", envFrom.String(), container.Name)
				break
			}
		}
		t.Errorf("EnvVar [%s] not founded in container %s", envFrom.String(), container.Name)
	}
	return check
}

// CheckVolumeMountIsPresent used to check if an corev1.VolumeMount is present in a corev1.Container object
func CheckVolumeMountIsPresent(volumeMount *corev1.VolumeMount) CheckContainerFunc {
	check := func(t *testing.T, container *corev1.Container) {
		for _, volumeMountIn := range container.VolumeMounts {
			if diff := testutils.CompareKubeResource(&volumeMountIn, volumeMount); diff == "" {
				t.Logf("VolumeMount [%s] founded in container %s", volumeMount.String(), container.Name)
				break
			}
		}
		t.Errorf("EnvVar [%s] not founded in container %s", volumeMount.String(), container.Name)
	}
	return check
}
