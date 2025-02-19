// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
)

func TestPodTemplateSpec(t *testing.T) {
	ifNotPresent := v1.PullPolicy("IfNotPresent")
	never := v1.PullPolicy("Never")
	pullSecret := []v1.LocalObjectReference{
		{
			Name: "otherPullSecretName",
		},
	}

	tests := []struct {
		name            string
		existingManager func() *fake.PodTemplateManagers
		override        v2alpha1.DatadogAgentComponentOverride
		validateManager func(t *testing.T, manager *fake.PodTemplateManagers)
	}{
		{
			name: "override service account name",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
				manager.PodTemplateSpec().Spec.ServiceAccountName = "old-service-account"
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ServiceAccountName: apiutils.NewPointer("new-service-account"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "new-service-account", manager.PodTemplateSpec().Spec.ServiceAccountName)
			},
		},
		{
			name: "given URI, override image name, tag; don't override a nonAgentContainer",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0", nonAgentName: "someregistry.com/datadog/non-agent:1.1.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name: "custom-agent",
					Tag:  "latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/datadog/custom-agent:latest", nonAgentName: "someregistry.com/datadog/non-agent:1.1.0"},
					t,
				)
			},
		},
		{
			name: "01: given URI, override image name, tag, JMX",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name:       "custom-agent",
					Tag:        "latest",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/datadog/custom-agent:latest-jmx"},
					t,
				)
			},
		},
		{
			name: "02: given URI, override image name, tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name: "custom-agent",
					Tag:  "latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/datadog/custom-agent:latest"},
					t,
				)
			},
		},
		{
			name: "03: given URI, override image tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Tag: "latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/datadog/agent:latest"},
					t,
				)
			},
		},
		{
			name: "04: given URI, override image with name:tag, full name takes precedence",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name:       "agent:9.99.9",
					Tag:        "latest",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "agent:9.99.9"},
					t,
				)
			},
		},
		{
			name: "05: given URI, override image name and JMX, retain tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name:       "custom-agent",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/datadog/custom-agent:7.38.0-jmx"},
					t,
				)
			},
		},
		{
			name: "06: given URI, override image with JMX tag and flag, don't duplicate jmx suffix",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Tag:        "latest-jmx",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/datadog/agent:latest-jmx"},
					t,
				)
			},
		},
		{
			name: "07: given URI with JMX tag, override image with JMX false",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0-jmx"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					JMXEnabled: false,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0"},
					t,
				)
			},
		},
		{
			name: "08: given name:tag, override image with full URI name, ignore tag and JMX, full name takes precedence",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name:       "someregistry.com/datadog/agent:9.99.9",
					Tag:        "latest",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/datadog/agent:9.99.9"},
					t,
				)
			},
		},
		{
			name: "09: given name:tag, override image name, tag, JMX, sets default registry",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name:       "agent",
					Tag:        "latest",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "gcr.io/datadoghq/agent:latest-jmx"},
					t,
				)
			},
		},
		{
			name: "10: given name:tag, override image with name:tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name: "agent:latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "agent:latest"},
					t,
				)
			},
		},
		{
			name: "11: given URI, override image with repo name:tag, full name takes precedence",
			// related to 09 Name precedence.
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name: "repo/agent:latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "repo/agent:latest"},
					t,
				)
			},
		},
		{
			name: "12: given image URI, override with short URI, full name takes precedence",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name: "someregistry.com/agent:latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/agent:latest"},
					t,
				)
			},
		},
		{
			name: "13: given short URI, override with name, tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name: "agent",
					Tag:  "latest",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/agent:latest"},
					t,
				)
			},
		},
		{
			name: "14: given long URI, override with name, tag",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/a/b/c/agent:7.38.0"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name:       "cluster-agent",
					JMXEnabled: true,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(
					manager,
					containerImageOptions{name: "someregistry.com/a/b/c/cluster-agent:7.38.0-jmx"},
					t,
				)
			},
		},
		{
			name: "15: given long URI, override name with slash, overrides name in current image",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(containerImageOptions{name: "someregistry.com/datadog/agent:9.99"}, t)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					Name: "otherregistry.com/agent",
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(manager, containerImageOptions{name: "someregistry.com/datadog/otherregistry.com/agent:9.99"}, t)
			},
		},
		{
			name: "16: override image pull policy",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(
					containerImageOptions{
						name:       "someregistry.com/datadog/agent:9.99",
						pullPolicy: "IfNotPresent",
					},
					t,
				)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					PullPolicy: &never,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(manager, containerImageOptions{
					name:       "someregistry.com/datadog/agent:9.99",
					pullPolicy: "Never",
				}, t)
			},
		},
		{
			name: "17: override image pull policy and pull secrets",
			existingManager: func() *fake.PodTemplateManagers {
				return fakePodTemplateManagersWithImageOverride(
					containerImageOptions{
						name:           "someregistry.com/datadog/agent:9.99",
						pullPolicy:     "Always",
						pullSecretName: "pullSecretName",
					},
					t,
				)
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Image: &v2alpha1.AgentImageConfig{
					PullPolicy:  &ifNotPresent,
					PullSecrets: &pullSecret,
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assertImageConfigValues(manager, containerImageOptions{
					name:           "someregistry.com/datadog/agent:9.99",
					pullPolicy:     "IfNotPresent",
					pullSecretName: "otherPullSecretName",
				}, t)
			},
		},
		{
			name: "add envs",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})

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
					{
						Name: "added-env-valuefrom",
						ValueFrom: &v1.EnvVarSource{
							FieldRef: &v1.ObjectFieldSelector{
								FieldPath: common.FieldPathStatusPodIP,
							},
						},
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
					{
						Name: "added-env-valuefrom",
						ValueFrom: &v1.EnvVarSource{
							FieldRef: &v1.ObjectFieldSelector{
								FieldPath: common.FieldPathStatusPodIP,
							},
						},
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
				return fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				CustomConfigurations: map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig{
					v2alpha1.AgentGeneralConfigFile: {
						ConfigMap: &v2alpha1.ConfigMapConfig{
							Name: "custom-config",
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				found := false
				for _, vol := range manager.VolumeMgr.Volumes {
					if vol.Name == fmt.Sprintf("%s-%s", getDefaultConfigMapName("datadog-agent", string(v2alpha1.AgentGeneralConfigFile)), strings.ToLower(string(v2alpha1.NodeAgentComponentName))) {
						found = true
						break
					}
				}
				assert.True(t, found)
			},
		},
		{
			name: "override confd with configMap",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraConfd: &v2alpha1.MultiCustomConfig{
					ConfigMap: &v2alpha1.ConfigMapConfig{
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
			name: "override confd with configData",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraConfd: &v2alpha1.MultiCustomConfig{
					ConfigDataMap: map[string]string{
						"path_to_file.yaml": "yaml: data",
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
			name: "override checksd with configMap",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraChecksd: &v2alpha1.MultiCustomConfig{
					ConfigMap: &v2alpha1.ConfigMapConfig{
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
			name: "override checksd with configData",
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				ExtraChecksd: &v2alpha1.MultiCustomConfig{
					ConfigDataMap: map[string]string{
						"path_to_file.py": "print('hello')",
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
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{Name: string(apicommon.CoreAgentContainerName)},
							{Name: string(apicommon.ClusterAgentContainerName)},
						},
						InitContainers: []v1.Container{
							{Name: string(apicommon.InitConfigContainerName)},
						},
					},
				})

				manager.EnvVarMgr.AddEnvVarToContainer(
					apicommon.ClusterAgentContainerName,
					&v1.EnvVar{
						Name:  DDLogLevel,
						Value: "info",
					},
				)

				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					apicommon.ClusterAgentContainerName: {
						LogLevel: apiutils.NewPointer("trace"),
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				envSet := false

				for _, env := range manager.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName] {
					if env.Name == DDLogLevel && env.Value == "trace" {
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
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})

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
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
				manager.PodTemplateSpec().Spec.SecurityContext = &v1.PodSecurityContext{
					RunAsUser: apiutils.NewPointer[int64](1234),
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				SecurityContext: &v1.PodSecurityContext{
					RunAsUser: apiutils.NewPointer[int64](5678),
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, int64(5678), *manager.PodTemplateSpec().Spec.SecurityContext.RunAsUser)
			},
		},
		{
			name: "override priority class name",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
				manager.PodTemplateSpec().Spec.PriorityClassName = "old-name"
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				PriorityClassName: apiutils.NewPointer("new-name"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "new-name", manager.PodTemplateSpec().Spec.PriorityClassName)
			},
		},
		{
			name: "override runtime class name",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
				manager.PodTemplateSpec().Spec.RuntimeClassName = apiutils.NewPointer("old-name")
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				RuntimeClassName: apiutils.NewPointer("new-name"),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, "new-name", *manager.PodTemplateSpec().Spec.RuntimeClassName)
			},
		},
		{
			name: "override affinity",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
				manager.PodTemplateSpec().Spec.Affinity = &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "node-label-1",
											Operator: v1.NodeSelectorOpExists,
										},
									},
								},
							},
						},
					},
					PodAffinity: &v1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-label-1": "value-1",
									},
								},
								TopologyKey: v1.LabelHostname,
							},
						},
					},
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-label-2": "value-2",
									},
								},
								TopologyKey: v1.LabelHostname,
							},
						},
					},
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				Affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "node-label-2",
											Operator: v1.NodeSelectorOpExists,
										},
									},
								},
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "node-label-3",
											Operator: v1.NodeSelectorOpExists,
										},
									},
								},
							},
						},
					},
					PodAffinity: &v1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-label-3": "value-3",
									},
								},
								TopologyKey: v1.LabelHostname,
							},
						},
					},
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-label-4": "value-4",
									},
								},
								TopologyKey: v1.LabelHostname,
							},
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				expectedAffinity := &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "node-label-1",
											Operator: v1.NodeSelectorOpExists,
										},
										{
											Key:      "node-label-2",
											Operator: v1.NodeSelectorOpExists,
										},
									},
								},
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "node-label-1",
											Operator: v1.NodeSelectorOpExists,
										},
										{
											Key:      "node-label-3",
											Operator: v1.NodeSelectorOpExists,
										},
									},
								},
							},
						},
					},
					PodAffinity: &v1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-label-1": "value-1",
									},
								},
								TopologyKey: v1.LabelHostname,
							},
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-label-3": "value-3",
									},
								},
								TopologyKey: v1.LabelHostname,
							},
						},
					},
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-label-2": "value-2",
									},
								},
								TopologyKey: v1.LabelHostname,
							},
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"pod-label-4": "value-4",
									},
								},
								TopologyKey: v1.LabelHostname,
							},
						},
					},
				}
				assert.Equal(t, expectedAffinity, manager.PodTemplateSpec().Spec.Affinity)
			},
		},
		{
			name: "override dns policy",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
				manager.PodTemplateSpec().Spec.DNSPolicy = v1.DNSClusterFirst
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				DNSPolicy: newDNSPolicyPointer(v1.DNSClusterFirstWithHostNet),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.Equal(t, v1.DNSClusterFirstWithHostNet, manager.PodTemplateSpec().Spec.DNSPolicy)
			},
		},
		{
			name: "override dns config",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
				manager.PodTemplateSpec().Spec.DNSConfig = &v1.PodDNSConfig{
					Nameservers: []string{
						"10.9.8.7",
					},
					Searches: []string{
						"dns-search-1", "dns-search-2", "dns-search-3",
					},
					Options: []v1.PodDNSConfigOption{
						{
							Name:  "",
							Value: apiutils.NewPointer("value-0"),
						},
						{
							Name:  "",
							Value: apiutils.NewPointer("value-1"),
						},
					},
				}
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				DNSConfig: &v1.PodDNSConfig{
					Nameservers: []string{
						"10.9.8.7", "10.9.8.4",
					},
					Searches: []string{
						"dns-search-1", "dns-search-2",
					},
					Options: []v1.PodDNSConfigOption{
						{
							Name:  "DNSResolver1",
							Value: apiutils.NewPointer("value-2"),
						},
						{
							Name:  "DNSResolver2",
							Value: apiutils.NewPointer("value-3"),
						},
					},
				},
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				expectedConfig := &v1.PodDNSConfig{
					Nameservers: []string{
						"10.9.8.7", "10.9.8.4",
					},
					Searches: []string{
						"dns-search-1", "dns-search-2",
					},
					Options: []v1.PodDNSConfigOption{
						{
							Name:  "DNSResolver1",
							Value: apiutils.NewPointer("value-2"),
						},
						{
							Name:  "DNSResolver2",
							Value: apiutils.NewPointer("value-3"),
						},
					},
				}
				assert.Equal(t, expectedConfig, manager.PodTemplateSpec().Spec.DNSConfig)
			},
		},

		{
			name: "add labels",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
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
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
				manager.PodTemplateSpec().Spec.HostNetwork = false
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				HostNetwork: apiutils.NewPointer(true),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.True(t, manager.PodTemplateSpec().Spec.HostNetwork)
			},
		},
		{
			name: "override host PID",
			existingManager: func() *fake.PodTemplateManagers {
				manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
				manager.PodTemplateSpec().Spec.HostPID = false
				return manager
			},
			override: v2alpha1.DatadogAgentComponentOverride{
				HostPID: apiutils.NewPointer(true),
			},
			validateManager: func(t *testing.T, manager *fake.PodTemplateManagers) {
				assert.True(t, manager.PodTemplateSpec().Spec.HostPID)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := test.existingManager()
			testLogger := zap.New(zap.UseDevMode(true))
			logger := testLogger.WithValues("test", t.Name())

			PodTemplateSpec(logger, manager, &test.override, v2alpha1.NodeAgentComponentName, "datadog-agent")

			test.validateManager(t, manager)
		})
	}
}

type containerImageOptions struct {
	name           string
	pullPolicy     string
	pullSecretName string

	nonAgentName string
}

// In practice, image string registry will be derived either from global.registry setting or the default.
// func fakePodTemplateManagersWithImageOverride(image string, t *testing.T) *fake.PodTemplateManagers {
func fakePodTemplateManagersWithImageOverride(imageOptions containerImageOptions, t *testing.T) *fake.PodTemplateManagers {
	manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})

	basicContainer := v1.Container{Name: "agent", Image: imageOptions.name}
	if imageOptions.pullPolicy != "" {
		basicContainer.ImagePullPolicy = v1.PullPolicy(imageOptions.pullPolicy)
	}

	manager.PodTemplateSpec().Spec.InitContainers = []v1.Container{basicContainer}
	manager.PodTemplateSpec().Spec.Containers = []v1.Container{basicContainer}

	// This represents a container that doesn't use the agent image
	if imageOptions.nonAgentName != "" {
		basicNonAgentContainer := v1.Container{Name: "nonAgent", Image: imageOptions.nonAgentName}
		manager.PodTemplateSpec().Spec.Containers = append(manager.PodTemplateSpec().Spec.Containers, basicNonAgentContainer)
	}

	if imageOptions.pullSecretName != "" {
		manager.PodTemplateSpec().Spec.ImagePullSecrets = []v1.LocalObjectReference{{Name: imageOptions.pullSecretName}}
	}

	return manager
}

// Assert all image config parameters are as expected
func assertImageConfigValues(manager *fake.PodTemplateManagers, imageOptions containerImageOptions, t *testing.T) {
	allContainers := append(
		manager.PodTemplateSpec().Spec.Containers, manager.PodTemplateSpec().Spec.InitContainers...,
	)

	for _, container := range allContainers {
		if container.Name == "agent" {
			assert.Equal(t, imageOptions.name, container.Image)
		} else {
			// This represents a container that doesn't use the agent image
			assert.Equal(t, imageOptions.nonAgentName, container.Image)
		}
	}

	if imageOptions.pullPolicy != "" {
		imagePullPolicy := v1.PullPolicy(imageOptions.pullPolicy)
		for _, container := range allContainers {
			assert.Equal(t, imagePullPolicy, container.ImagePullPolicy)
		}
	}

	if imageOptions.pullSecretName != "" {
		imageSecrets := []v1.LocalObjectReference{{Name: imageOptions.pullSecretName}}
		assert.Equal(t, imageSecrets, manager.PodTemplateSpec().Spec.ImagePullSecrets)
	}
}

func newDNSPolicyPointer(s v1.DNSPolicy) *v1.DNSPolicy {
	return &s
}
