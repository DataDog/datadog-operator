// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestContainer(t *testing.T) {
	containerName := commonv1.CoreAgentContainerName

	tests := []struct {
		name            string
		existingManager func() *fake.PodTemplateManagers
		override        v2alpha1.DatadogAgentGenericContainer
		validateManager func(t *testing.T, manager *fake.PodTemplateManagers)
	}{
		{
			name: "override container name",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Name: apiutils.NewStringPointer("my-container-name"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				for _, container := range manager.PodTemplateSpec().Spec.Containers {
					if container.Name == string(commonv1.CoreAgentContainerName) {
						assert.Equal(t, "my-container-name", container.Name)
					}
				}
			},
		},
		{
			name: "override log level",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				LogLevel: apiutils.NewStringPointer("debug"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				envs := manager.EnvVarMgr.EnvVarsByC[commonv1.CoreAgentContainerName]
				expectedEnvs := []*corev1.EnvVar{
					{
						Name:  common.DDLogLevel,
						Value: "debug",
					},
				}
				assert.Equal(t, expectedEnvs, envs)
			},
		},
		{
			name: "add envs",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.EnvVar().AddEnvVarToContainer(
					commonv1.CoreAgentContainerName,
					&corev1.EnvVar{
						Name:  "existing-env",
						Value: "some-val",
					},
				)
				return manager
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Env: []corev1.EnvVar{
					{
						Name:  "added-env-1",
						Value: "1",
					},
					{
						Name:  "added-env-2",
						Value: "2",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				envs := manager.EnvVarMgr.EnvVarsByC[commonv1.CoreAgentContainerName]
				expectedEnvs := []*corev1.EnvVar{
					{
						Name:  "existing-env",
						Value: "some-val",
					},
					{
						Name:  "added-env-1",
						Value: "1",
					},
					{
						Name:  "added-env-2",
						Value: "2",
					},
				}
				assert.Equal(t, expectedEnvs, envs)
			},
		},
		{
			name: "add volume mounts",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.VolumeMount().AddVolumeMountToContainer(
					&corev1.VolumeMount{
						Name: "existing-volume-mount",
					},
					commonv1.CoreAgentContainerName,
				)
				return manager
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				VolumeMounts: []corev1.VolumeMount{
					{
						Name: "added-volume-mount-1",
					},
					{
						Name: "added-volume-mount-2",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				mounts := manager.VolumeMountMgr.VolumeMountsByC[commonv1.CoreAgentContainerName]
				expectedMounts := []*corev1.VolumeMount{
					{
						Name: "existing-volume-mount",
					},
					{
						Name: "added-volume-mount-1",
					},
					{
						Name: "added-volume-mount-2",
					},
				}
				assert.Equal(t, expectedMounts, mounts)
			},
		},
		{
			name: "override resources",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Resources: &corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU: *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				for _, container := range manager.PodTemplateSpec().Spec.Containers {
					if container.Name == string(commonv1.CoreAgentContainerName) {
						assert.Equal(
							t,
							&corev1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
								},
								Requests: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU: *resource.NewQuantity(1, resource.DecimalSI),
								},
							},
							container.Resources)
					}
				}
			},
		},
		{
			name: "override command",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Command: []string{"test-agent", "start"},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				for _, container := range manager.PodTemplateSpec().Spec.Containers {
					if container.Name == string(commonv1.CoreAgentContainerName) {
						assert.Equal(t, []string{"test-agent", "start"}, container.Command)
					}
				}
			},
		},
		{
			name: "override args",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Args: []string{"arg1", "val1"},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				for _, container := range manager.PodTemplateSpec().Spec.Containers {
					if container.Name == string(commonv1.CoreAgentContainerName) {
						assert.Equal(t, []string{"arg1", "val1"}, container.Args)
					}
				}
			},
		},
		{
			name: "override health port",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				HealthPort: apiutils.NewInt32Pointer(1234),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				envs := manager.EnvVarMgr.EnvVarsByC[commonv1.CoreAgentContainerName]
				expectedEnvs := []*corev1.EnvVar{
					{
						Name:  common.DDHealthPort,
						Value: "1234",
					},
				}
				assert.Equal(t, expectedEnvs, envs)
			},
		},
		{
			name: "override readiness probe",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				ReadinessProbe: &corev1.Probe{
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				for _, container := range manager.PodTemplateSpec().Spec.Containers {
					if container.Name == string(commonv1.CoreAgentContainerName) {
						assert.Equal(
							t,
							&corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								PeriodSeconds:       30,
								SuccessThreshold:    1,
								FailureThreshold:    5,
							},
							container.ReadinessProbe,
						)
					}
				}
			},
		},
		{
			name: "override liveness probe",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				LivenessProbe: &corev1.Probe{
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				for _, container := range manager.PodTemplateSpec().Spec.Containers {
					if container.Name == string(commonv1.CoreAgentContainerName) {
						assert.Equal(
							t,
							&corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								PeriodSeconds:       30,
								SuccessThreshold:    1,
								FailureThreshold:    5,
							},
							container.LivenessProbe,
						)
					}
				}
			},
		},
		{
			name: "override security context",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser: apiutils.NewInt64Pointer(12345),
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				for _, container := range manager.PodTemplateSpec().Spec.Containers {
					if container.Name == string(commonv1.CoreAgentContainerName) {
						assert.Equal(
							t,
							&corev1.SecurityContext{
								RunAsUser: apiutils.NewInt64Pointer(12345),
							},
							container.SecurityContext,
						)
					}
				}
			},
		},
		{
			name: "override app armor profile",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				AppArmorProfileName: apiutils.NewStringPointer("my-app-armor-profile"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				annotation := fmt.Sprintf("%s/%s", common.AppArmorAnnotationKey, commonv1.CoreAgentContainerName)
				assert.Equal(t, "my-app-armor-profile", manager.AnnotationMgr.Annotations[annotation])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := test.existingManager()

			Container(containerName, manager, &test.override)

			test.validateManager(t, manager)
		})
	}
}
