// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodTemplateSpec(t *testing.T) {
	tests := []struct {
		name            string
		existingManager func() *fake.PodTemplateManagers
		override        v2alpha1.DatadogAgentComponentOverride
		validateManager func(t *testing.T, manager *fake.PodTemplateManagers)
	}{
		{
			name: "override service account name",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.ServiceAccountName = "old-service-account"
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ServiceAccountName: apiutils.NewStringPointer("new-service-account"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "new-service-account", manager.PodTemplateSpec().Spec.ServiceAccountName)
			},
		},
		{
			name: "override image",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.InitContainers = []v1.Container{
					{
						Image: "docker.io/datadog/agent:7.38.0",
					},
				}
				manager.PodTemplateSpec().Spec.Containers = []v1.Container{
					{
						Image: "docker.io/datadog/agent:7.38.0",
					},
					{
						Image: "docker.io/datadog/agent:7.38.0",
					},
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &commonv1.AgentImageConfig{
					Name:       "custom-agent",
					Tag:        "latest",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				allContainers := append(
					manager.PodTemplateSpec().Spec.Containers, manager.PodTemplateSpec().Spec.InitContainers...,
				)

				for _, container := range allContainers {
					assert.Equal(t, "docker.io/datadog/custom-agent:latest-jmx", container.Image)
				}
			},
		},
		{
			name: "add envs",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)

				manager.EnvVar().AddEnvVar(&v1.EnvVar{
					Name:  "existing-env",
					Value: "123",
				})

				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Env: []v1.EnvVar{
					{
						Name:  "added-env",
						Value: "456",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				expectedEnvs := []*v1.EnvVar{
					{
						Name:  "existing-env",
						Value: "123",
					},
					{
						Name:  "added-env",
						Value: "456",
					},
				}

				for _, envs := range manager.EnvVarMgr.EnvVarsByC {
					assert.Equal(t, expectedEnvs, envs)
				}
			},
		},
		{
			// Note: this test is for the node agent (hardcoded in t.Run).
			name: "add custom configs",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{
					v2alpha1.AgentGeneralConfigFile: {
						ConfigMap: &commonv1.ConfigMapConfig{
							Name: "custom-config",
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				found := false
				for _, vol := range manager.VolumeMgr.Volumes {
					if vol.Name == common.AgentCustomConfigVolumeName {
						found = true
						break
					}
				}
				assert.True(t, found)
			},
		},
		{
			name: "override confd",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraConfd: &v2alpha1.CustomConfig{
					ConfigMap: &commonv1.ConfigMapConfig{
						Name: "extra-confd",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				found := false
				for _, vol := range manager.VolumeMgr.Volumes {
					if vol.Name == common.ConfdVolumeName {
						found = true
						break
					}
				}
				assert.True(t, found)
			},
		},
		{
			name: "override checksd",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraChecksd: &v2alpha1.CustomConfig{
					ConfigMap: &commonv1.ConfigMapConfig{
						Name: "extra-checksd",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				found := false
				for _, vol := range manager.VolumeMgr.Volumes {
					if vol.Name == common.ChecksdVolumeName {
						found = true
						break
					}
				}
				assert.True(t, found)
			},
		},
		{
			// This test is pretty simple because "container_test.go" already tests overriding containers
			name: "override containers",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)

				manager.EnvVarMgr.AddEnvVarToContainer(
					commonv1.ClusterAgentContainerName,
					&v1.EnvVar{
						Name:  common.DDLogLevel,
						Value: "info",
					},
				)

				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Containers: map[commonv1.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					commonv1.ClusterAgentContainerName: {
						LogLevel: apiutils.NewStringPointer("trace"),
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				envSet := false

				for _, env := range manager.EnvVarMgr.EnvVarsByC[commonv1.ClusterAgentContainerName] {
					if env.Name == common.DDLogLevel && env.Value == "trace" {
						envSet = true
						break
					}
				}

				assert.True(t, envSet)
			},
		},
		{
			name: "add volumes",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)

				manager.Volume().AddVolume(&v1.Volume{
					Name: "existing-volume",
				})

				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Volumes: []v1.Volume{
					{
						Name: "added-volume",
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				expectedVolumes := []*v1.Volume{
					{
						Name: "existing-volume",
					},
					{
						Name: "added-volume",
					},
				}

				assert.Equal(t, expectedVolumes, manager.VolumeMgr.Volumes)
			},
		},
		{
			name: "override security context",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.SecurityContext = &v1.PodSecurityContext{
					RunAsUser: apiutils.NewInt64Pointer(1234),
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				SecurityContext: &v1.PodSecurityContext{
					RunAsUser: apiutils.NewInt64Pointer(5678),
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, int64(5678), *manager.PodTemplateSpec().Spec.SecurityContext.RunAsUser)
			},
		},
		{
			name: "override priority class name",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.PriorityClassName = "old-name"
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				PriorityClassName: apiutils.NewStringPointer("new-name"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "new-name", manager.PodTemplateSpec().Spec.PriorityClassName)
			},
		},
		{
			name: "override affinity",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.Affinity = &v1.Affinity{
					PodAntiAffinity: &v1.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
							{
								Weight: 50,
								PodAffinityTerm: v1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"old-label": "123",
										},
									},
									TopologyKey: "kubernetes.io/hostname",
								},
							},
						},
					},
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Affinity: &v1.Affinity{
					PodAntiAffinity: &v1.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
							{
								Weight: 50,
								PodAffinityTerm: v1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"new-label": "456", // Changed
										},
									},
									TopologyKey: "kubernetes.io/hostname",
								},
							},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t,
					map[string]string{"new-label": "456"},
					manager.PodTemplateSpec().Spec.Affinity.PodAntiAffinity.
						PreferredDuringSchedulingIgnoredDuringExecution[0].
						PodAffinityTerm.LabelSelector.MatchLabels)
			},
		},
		{
			name: "add labels",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Labels = map[string]string{
					"existing-label": "123",
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Labels: map[string]string{
					"existing-label": "456",
					"new-label":      "789",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				expectedLabels := map[string]string{
					"existing-label": "456",
					"new-label":      "789",
				}

				assert.Equal(t, expectedLabels, manager.PodTemplateSpec().Labels)
			},
		},
		{
			name: "override host network",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.HostNetwork = false
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				HostNetwork: apiutils.NewBoolPointer(true),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.True(t, manager.PodTemplateSpec().Spec.HostNetwork)
			},
		},
		{
			name: "override host PID",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t)
				manager.PodTemplateSpec().Spec.HostPID = false
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				HostPID: apiutils.NewBoolPointer(true),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.True(t, manager.PodTemplateSpec().Spec.HostPID)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := test.existingManager()

			PodTemplateSpec(manager, &test.override, v2alpha1.NodeAgentComponentName, "datadog-agent")

			test.validateManager(t, manager)
		})
	}
}
