// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"reflect"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/pkg/constants"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestContainer(t *testing.T) {
	agentContainer := &corev1.Container{
		Name: string(apicommon.CoreAgentContainerName),
	}
	initVolContainer := &corev1.Container{
		Name: string(apicommon.InitVolumeContainerName),
	}
	initConfigContainer := &corev1.Container{
		Name: string(apicommon.InitConfigContainerName),
	}
	systemProbeContainer := &corev1.Container{
		Name: string(apicommon.SystemProbeContainerName),
	}
	tests := []struct {
		name            string
		containerName   apicommon.AgentContainerName
		existingManager func() *fake.PodTemplateManagers
		override        v2alpha1.DatadogAgentGenericContainer
		validateManager func(t *testing.T, manager *fake.PodTemplateManagers, containerName string)
	}{
		{
			name:          "override container name",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Name: apiutils.NewStringPointer("my-container-name"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, "my-container-name", func(container corev1.Container) bool {
					return true
				})
			},
		},
		{
			name:          "override log level",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				LogLevel: apiutils.NewStringPointer("debug"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				envs := manager.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
				expectedEnvs := []*corev1.EnvVar{
					{
						Name:  constants.DDLogLevel,
						Value: "debug",
					},
				}
				assert.Equal(t, expectedEnvs, envs)
			},
		},
		{
			name:          "add envs",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
				manager.EnvVar().AddEnvVarToContainer(
					apicommon.CoreAgentContainerName,
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
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				envs := manager.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
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
			name:          "add volume mounts",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
				manager.VolumeMount().AddVolumeMountToContainer(
					&corev1.VolumeMount{
						Name: "existing-volume-mount",
					},
					apicommon.CoreAgentContainerName,
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
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				mounts := manager.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
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
			name:          "override resources - when there are none defined",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
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
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						corev1.ResourceRequirements{
							Limits: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
							},
							Requests: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU: *resource.NewQuantity(1, resource.DecimalSI),
							},
						},
						container.Resources)
				})
			},
		},
		{
			name:          "override resources - when there are some defined",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: string(apicommon.CoreAgentContainerName),
								Resources: corev1.ResourceRequirements{
									Limits: map[corev1.ResourceName]resource.Quantity{
										corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI), // Not overridden, should be kept
										corev1.ResourceMemory: *resource.NewQuantity(2048, resource.DecimalSI),
									},
									Requests: map[corev1.ResourceName]resource.Quantity{
										corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
										corev1.ResourceMemory: *resource.NewQuantity(1024, resource.DecimalSI), // Not overridden, should be kept
									},
								},
							},
						},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Resources: &corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: *resource.NewQuantity(4096, resource.DecimalSI),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						corev1.ResourceRequirements{
							Limits: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),    // Not overridden
								corev1.ResourceMemory: *resource.NewQuantity(4096, resource.DecimalSI), // Overridden
							},
							Requests: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),    // Overridden
								corev1.ResourceMemory: *resource.NewQuantity(1024, resource.DecimalSI), // Not overridden
							},
						},
						container.Resources)
				})
			},
		},
		{
			name:          "override resources - when the override specifies a 0",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: string(apicommon.CoreAgentContainerName),
								Resources: corev1.ResourceRequirements{
									Limits: map[corev1.ResourceName]resource.Quantity{
										corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),    // Not overridden, should be kept
										corev1.ResourceMemory: *resource.NewQuantity(2048, resource.DecimalSI), // Not overridden, should be kept
									},
									Requests: map[corev1.ResourceName]resource.Quantity{
										corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
										corev1.ResourceMemory: *resource.NewQuantity(1024, resource.DecimalSI), // Not overridden, should be kept
									},
								},
							},
						},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Resources: &corev1.ResourceRequirements{
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU: *resource.NewQuantity(0, resource.DecimalSI),
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						corev1.ResourceRequirements{
							Limits: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),    // Not overridden
								corev1.ResourceMemory: *resource.NewQuantity(2048, resource.DecimalSI), // Not overridden
							},
							Requests: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU:    *resource.NewQuantity(0, resource.DecimalSI),    // Overridden
								corev1.ResourceMemory: *resource.NewQuantity(1024, resource.DecimalSI), // Not overridden
							},
						},
						container.Resources)
				})
			},
		},
		{
			name:          "override command",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Command: []string{"test-agent", "start"},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual([]string{"test-agent", "start"}, container.Command)
				})
			},
		},
		{
			name:          "override args",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Args: []string{"arg1", "val1"},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual([]string{"arg1", "val1"}, container.Args)
				})
			},
		},
		{
			name:          "override health port",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				HealthPort: apiutils.NewInt32Pointer(1234),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				envs := manager.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
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
			name:          "override readiness probe with default HTTPGet",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
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
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.Probe{
							InitialDelaySeconds: 10,
							TimeoutSeconds:      5,
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							FailureThreshold:    5,
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/ready",
									Port: intstr.IntOrString{
										IntVal: 5555,
									},
								},
							},
						},
						container.ReadinessProbe)
				})
			},
		},
		{
			name:          "override readiness probe with non-HTTPGet handler",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				ReadinessProbe: &corev1.Probe{
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{
							Command: []string{"echo", "foo", "bar"},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.Probe{
							InitialDelaySeconds: 10,
							TimeoutSeconds:      5,
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							FailureThreshold:    5,
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"echo", "foo", "bar"},
								},
							},
						},
						container.ReadinessProbe)
				})
			},
		},
		{
			name:          "override readiness probe",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				ReadinessProbe: &corev1.Probe{
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/some/path",
							Port: intstr.IntOrString{
								IntVal: 1234,
							},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/some/path",
									Port: intstr.IntOrString{
										IntVal: 1234,
									},
								},
							},
							InitialDelaySeconds: 10,
							TimeoutSeconds:      5,
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							FailureThreshold:    5,
						},
						container.ReadinessProbe)
				})
			},
		},
		{
			name:          "override liveness probe with default HTTPGet",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
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
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.Probe{
							InitialDelaySeconds: 10,
							TimeoutSeconds:      5,
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							FailureThreshold:    5,
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/live",
									Port: intstr.IntOrString{
										IntVal: 5555,
									},
								},
							},
						},
						container.LivenessProbe)
				})
			},
		},
		{
			name:          "override liveness probe with non-HTTPGet handler",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				LivenessProbe: &corev1.Probe{
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{
							Command: []string{"echo", "foo", "bar"},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.Probe{
							InitialDelaySeconds: 10,
							TimeoutSeconds:      5,
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							FailureThreshold:    5,
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"echo", "foo", "bar"},
								},
							},
						},
						container.LivenessProbe)
				})
			},
		},
		{
			name:          "override liveness probe",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				LivenessProbe: &corev1.Probe{
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/some/path",
							Port: intstr.IntOrString{
								IntVal: 1234,
							},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.Probe{
							InitialDelaySeconds: 10,
							TimeoutSeconds:      5,
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							FailureThreshold:    5,
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/some/path",
									Port: intstr.IntOrString{
										IntVal: 1234,
									},
								},
							},
						},
						container.LivenessProbe)
				})
			},
		},
		{
			name:          "override startup probe with default HTTPGet",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				StartupProbe: &corev1.Probe{
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.Probe{
							InitialDelaySeconds: 10,
							TimeoutSeconds:      5,
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							FailureThreshold:    5,
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/startup",
									Port: intstr.IntOrString{
										IntVal: 5555,
									},
								},
							},
						},
						container.StartupProbe)
				})
			},
		},
		{
			name:          "override startup probe with non-HTTPGet handler",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				StartupProbe: &corev1.Probe{
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{
							Command: []string{"echo", "foo", "bar"},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.Probe{
							InitialDelaySeconds: 10,
							TimeoutSeconds:      5,
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							FailureThreshold:    5,
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"echo", "foo", "bar"},
								},
							},
						},
						container.StartupProbe)
				})
			},
		},
		{
			name:          "override startup probe",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				StartupProbe: &corev1.Probe{
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/some/path",
							Port: intstr.IntOrString{
								IntVal: 1234,
							},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.Probe{
							InitialDelaySeconds: 10,
							TimeoutSeconds:      5,
							PeriodSeconds:       30,
							SuccessThreshold:    1,
							FailureThreshold:    5,
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/some/path",
									Port: intstr.IntOrString{
										IntVal: 1234,
									},
								},
							},
						},
						container.StartupProbe)
				})
			},
		},
		{
			name:          "override security context",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser: apiutils.NewInt64Pointer(12345),
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.Containers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.SecurityContext{
							RunAsUser: apiutils.NewInt64Pointer(12345),
						},
						container.SecurityContext)
				})
			},
		},
		{
			name:          "override seccomp root path",
			containerName: apicommon.SystemProbeContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*systemProbeContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				SeccompConfig: &v2alpha1.SeccompConfig{
					CustomRootPath: apiutils.NewStringPointer("seccomp/path"),
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				expectedVolumes := []*corev1.Volume{
					{
						Name: "seccomp-root",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "seccomp/path",
							},
						},
					},
				}
				assert.Equal(t, expectedVolumes, manager.VolumeMgr.Volumes)
			},
		},
		{
			name:          "override seccomp profile",
			containerName: apicommon.SystemProbeContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*systemProbeContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				SeccompConfig: &v2alpha1.SeccompConfig{
					CustomProfile: &v2alpha1.CustomConfig{
						ConfigMap: &v2alpha1.ConfigMapConfig{
							Name: "custom-seccomp-profile",
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				expectedVolumes := []*corev1.Volume{
					{
						Name: "datadog-agent-security",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "custom-seccomp-profile",
								},
							},
						},
					},
				}
				assert.Equal(t, expectedVolumes, manager.VolumeMgr.Volumes)
			},
		},
		{
			name:          "override app armor profile",
			containerName: apicommon.CoreAgentContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				AppArmorProfileName: apiutils.NewStringPointer("my-app-armor-profile"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				annotation := fmt.Sprintf("%s/%s", v2alpha1.AppArmorAnnotationKey, apicommon.CoreAgentContainerName)
				assert.Equal(t, "my-app-armor-profile", manager.AnnotationMgr.Annotations[annotation])
			},
		},
		{
			name:          "override initContainer name",
			containerName: apicommon.InitVolumeContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{*initVolContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Name: apiutils.NewStringPointer("my-initContainer-name"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.InitContainers, "my-initContainer-name", func(container corev1.Container) bool {
					return true
				})
			},
		},
		{
			name:          "add initContainer envs",
			containerName: apicommon.InitVolumeContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{*initVolContainer},
					},
				})
				manager.EnvVar().AddEnvVarToInitContainer(
					apicommon.InitVolumeContainerName,
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
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				envs := manager.EnvVarMgr.EnvVarsByC[apicommon.InitVolumeContainerName]
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
			name:          "add initContainer volume mounts",
			containerName: apicommon.InitVolumeContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{*initVolContainer},
					},
				})
				manager.VolumeMount().AddVolumeMountToInitContainer(
					&corev1.VolumeMount{
						Name: "existing-init-container-volume-mount",
					},
					apicommon.InitVolumeContainerName,
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
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				mounts := manager.VolumeMountMgr.VolumeMountsByC[apicommon.InitVolumeContainerName]
				expectedMounts := []*corev1.VolumeMount{
					{
						Name: "existing-init-container-volume-mount",
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
			name:          "override initContainer resources",
			containerName: apicommon.InitConfigContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{*initConfigContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				Resources: &corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.InitContainers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						corev1.ResourceRequirements{
							Limits: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU:    *resource.NewQuantity(2, resource.DecimalSI),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Requests: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
						container.Resources)
				})
			},
		},
		{
			name:          "override initContainer security context",
			containerName: apicommon.InitConfigContainerName,
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{*initConfigContainer},
					},
				})
			},
			override: v2alpha1.DatadogAgentGenericContainer{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser: apiutils.NewInt64Pointer(12345),
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers, containerName string) {
				assertContainerMatch(t, manager.PodTemplateSpec().Spec.InitContainers, containerName, func(container corev1.Container) bool {
					return reflect.DeepEqual(
						&corev1.SecurityContext{
							RunAsUser: apiutils.NewInt64Pointer(12345),
						},
						container.SecurityContext)
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := test.existingManager()
			Container(test.containerName, manager, &test.override)

			test.validateManager(t, manager, string(test.containerName))
		})
	}
}

func assertContainerMatch(t *testing.T, containerList []corev1.Container, matchName string, matcher func(container corev1.Container) bool) {
	found := false
	for _, container := range containerList {
		if container.Name == matchName && matcher(container) {
			found = true
			break
		}
	}
	assert.Truef(t, found, "Expected value for container `%s` was not found.", matchName)
}
