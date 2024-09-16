// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils_test

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	apiequality "k8s.io/apimachinery/pkg/api/equality"

	"github.com/DataDog/datadog-operator/pkg/testutils"
)

// ContainerCheckInterface interface used by container checks.
type ContainerCheckInterface interface {
	Check(t *testing.T, container *corev1.Container) error
}

// PodTemplateSpecCheckInterface interface use by PodTemplateSpec checks.
type PodTemplateSpecCheckInterface interface {
	Check(t *testing.T, podTemplate *corev1.PodTemplateSpec) error
}

// CheckPodTemplateFunc define the signature of a check function against a corev1.PodTemplateSpec
type CheckPodTemplateFunc func(t *testing.T, podTemplate *corev1.PodTemplateSpec)

// CheckContainerInPodTemplate used to execute a ContainerCheckInterface.Check(...) function again a specific container in a corev1.PodTemplateSpec
// object.
func CheckContainerInPodTemplate(containerName string, checkFunc ContainerCheckInterface) CheckPodTemplateFunc {
	check := func(t *testing.T, podTemplate *corev1.PodTemplateSpec) {
		for _, container := range podTemplate.Spec.Containers {
			if container.Name == containerName {
				if err := checkFunc.Check(t, &container); err != nil {
					t.Error(err)
				}
			}
		}
		t.Errorf("Container %s not found", containerName)
	}
	return check
}

// CheckVolumeIsPresent used to check if a corev1.Volume is present in a corev1.PodTemplateSpec object.
type CheckVolumeIsPresent struct {
	Volume *corev1.Volume
}

// Check used to check if a corev1.Volume is present in a corev1.PodTemplateSpec object.
func (c *CheckVolumeIsPresent) Check(t *testing.T, podTemplate *corev1.PodTemplateSpec) error {
	for _, volumeIn := range podTemplate.Spec.Volumes {
		if apiequality.Semantic.DeepEqual(c.Volume, &volumeIn) {
			t.Logf("Volume %s found", c.Volume.Name)
			return nil
		}
	}
	return fmt.Errorf("volume %s not found", c.Volume.Name)
}

// CheckVolumeIsNotPresent used to check if a corev1.Volume is not present in a corev1.PodTemplateSpec object.
type CheckVolumeIsNotPresent struct {
	Volume *corev1.Volume
}

// Check used to check if a corev1.Volume is not present in a corev1.PodTemplateSpec object.
func (c *CheckVolumeIsNotPresent) Check(t *testing.T, podTemplate *corev1.PodTemplateSpec) error {
	found := false
	for _, volumeIn := range podTemplate.Spec.Volumes {
		if diff := testutils.CompareKubeResource(c.Volume, &volumeIn); diff == "" {
			found = true
			break
		}
	}
	if found {
		t.Logf("Volume %s found", c.Volume.Name)
		return nil
	}
	return fmt.Errorf("volume %s not found", c.Volume.Name)
}

// CheckContainerDeepEqualIsPresent used to check if corev1.Container is equal to a container inside a corev1.PodTemplateSpec object.
type CheckContainerDeepEqualIsPresent struct {
	Container *corev1.Container
}

// Check used to check if corev1.Container is equal to a container inside a corev1.PodTemplateSpec object.
func (c *CheckContainerDeepEqualIsPresent) Check(t *testing.T, podTemplate *corev1.PodTemplateSpec) error {
	for _, containerIn := range podTemplate.Spec.Containers {
		if diff := testutils.CompareKubeResource(c.Container, &containerIn); diff != "" {
			t.Logf("container %s found", c.Container.Name)
			return nil
		}
	}
	return fmt.Errorf("container %s not found", c.Container.Name)
}

// CheckContainerNameIsPresentFunc used to check if container name is equal to a container name
// present in a corev1.PodTemplateSpec object.
type CheckContainerNameIsPresentFunc struct {
	Name string
}

// Check used to check if container name is equal to a container name
// present in a corev1.PodTemplateSpec object.
func (c *CheckContainerNameIsPresentFunc) Check(t *testing.T, podTemplate *corev1.PodTemplateSpec) error {
	for _, containerIn := range podTemplate.Spec.Containers {
		if containerIn.Name == c.Name {
			t.Logf("container %s found", containerIn.Name)
			return nil
		}
	}
	return fmt.Errorf("container %s not found", c.Name)
}

// CheckEnvVarIsPresent used to check if an corev1.EnvVar is present in a corev1.Container object
type CheckEnvVarIsPresent struct {
	EnvVar *corev1.EnvVar
}

// Check used to check if an corev1.EnvVar is present in a corev1.Container object
func (c *CheckEnvVarIsPresent) Check(t *testing.T, container *corev1.Container) error {
	for _, envVarIn := range container.Env {
		if diff := testutils.CompareKubeResource(&envVarIn, c.EnvVar); diff == "" {
			t.Logf("EnvVar %s found in container %s", c.EnvVar.Name, container.Name)
			return nil
		}
	}
	return fmt.Errorf("EnvVar %s not found in container %s", c.EnvVar.Name, container.Name)
}

// CheckEnvFromIsPresent used to check if an corev1.EnvFromSource is present in a corev1.Container object
type CheckEnvFromIsPresent struct {
	EnvFrom *corev1.EnvFromSource
}

// Check used to check if an corev1.EnvFromSource is present in a corev1.Container object
func (c *CheckEnvFromIsPresent) Check(t *testing.T, container *corev1.Container) error {
	for _, envVarIn := range container.EnvFrom {
		if diff := testutils.CompareKubeResource(&envVarIn, c.EnvFrom); diff == "" {
			t.Logf("EnvVar [%s] found in container %s", c.EnvFrom.String(), container.Name)
			return nil
		}
	}
	return fmt.Errorf("envVar [%s] not found in container %s", c.EnvFrom.String(), container.Name)
}

// CheckVolumeMountIsPresent used to check if an corev1.VolumeMount is present in a corev1.Container object
type CheckVolumeMountIsPresent struct {
	VolumeMount *corev1.VolumeMount
}

// Check used to check if an corev1.VolumeMount is present in a corev1.Container object
func (c *CheckVolumeMountIsPresent) Check(t *testing.T, container *corev1.Container) error {
	for _, volumeMountIn := range container.VolumeMounts {
		if diff := testutils.CompareKubeResource(&volumeMountIn, c.VolumeMount); diff == "" {
			t.Logf("VolumeMount [%s] found in container %s", c.VolumeMount.String(), container.Name)
			return nil
		}
	}
	return fmt.Errorf("volumeMount [%s] not found in container %s", c.VolumeMount.String(), container.Name)
}
