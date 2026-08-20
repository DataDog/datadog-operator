// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecksrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

func Test_getDefaultServiceAccountName(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}

	assert.Equal(t, "my-datadog-agent-cluster-checks-runner", getDefaultServiceAccountName(&dda))
}

func Test_getPodDisruptionBudget(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}
	testpdb := GetClusterChecksRunnerPodDisruptionBudget(&dda, false).(*policyv1.PodDisruptionBudget)
	assert.Equal(t, "my-datadog-agent-cluster-checks-runner-pdb", testpdb.Name)
	assert.Equal(t, intstr.FromInt(pdbMaxUnavailableInstances), *testpdb.Spec.MaxUnavailable)
	assert.Nil(t, testpdb.Spec.MinAvailable)
}

func Test_copyCheckAssetsInitContainer(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}

	podSpec := NewDefaultClusterChecksRunnerPodTemplateSpec(dda.GetObjectMeta(), &dda.Spec).Spec

	// Locate the init container that seeds the conf.d overlay with packaged
	// check assets (SNMP profiles, etc.) so cluster checks can use them.
	var initContainer *corev1.Container
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].Name == initCopyCheckAssetsContainerName {
			initContainer = &podSpec.InitContainers[i]
			break
		}
	}

	if !assert.NotNil(t, initContainer, "expected a %q init container", initCopyCheckAssetsContainerName) {
		return
	}

	// It must mount remove-corechecks at the scratch path and must NOT overlay
	// /etc/datadog-agent or its conf.d, otherwise it could not read the image's
	// packaged conf.d assets.
	assert.Len(t, initContainer.VolumeMounts, 1)
	assert.Equal(t, common.RmCorechecksVolumeName, initContainer.VolumeMounts[0].Name)
	assert.Equal(t, common.RmCorechecksConfdInitPath, initContainer.VolumeMounts[0].MountPath)
	for _, vm := range initContainer.VolumeMounts {
		assert.NotEqual(t, common.ConfigVolumePath, vm.MountPath)
		assert.NotEqual(t, common.ConfigVolumePath+"/conf.d", vm.MountPath)
	}

	// The command copies the image conf.d into the overlay and drops default
	// check configs while preserving packaged sub-directory assets.
	require.Len(t, initContainer.Args, 1)
	assert.Contains(t, initContainer.Args[0], "cp -a /etc/datadog-agent/conf.d/.")
	assert.Contains(t, initContainer.Args[0], common.RmCorechecksConfdInitPath)
	assert.Contains(t, initContainer.Args[0], "*.yaml.default")
}

func TestDefaultEnvVarsJMXUseContainerSupport(t *testing.T) {
	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want bool
	}{
		{
			name: "DatadogAgent without override does not add JMX env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			},
			want: false,
		},
		{
			name: "CCR override without image does not add JMX env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {},
					},
				},
			},
			want: false,
		},
		{
			name: "Cluster Agent JMX image does not add CCR env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							Image: &v2alpha1.AgentImageConfig{JMXEnabled: true},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "Node Agent JMX image does not add CCR env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Image: &v2alpha1.AgentImageConfig{Tag: "7.80.2-jmx"},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "CCR JMX image flag adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {
							Image: &v2alpha1.AgentImageConfig{JMXEnabled: true},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "CCR JMX image adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {
							Image: &v2alpha1.AgentImageConfig{Tag: "7.80.2-jmx"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "CCR image name with JMX suffix adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {
							Image: &v2alpha1.AgentImageConfig{Name: "agent:7.80.2-jmx"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "CCR full image adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {
							Image: &v2alpha1.AgentImageConfig{Tag: "7.80.2-full"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "CCR full image name adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {
							Image: &v2alpha1.AgentImageConfig{Name: "agent:7.80.2-full"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "CCR FIPS full image adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {
							Image: &v2alpha1.AgentImageConfig{Tag: "7.80.2-fips-full"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "CCR FIPS full image name adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {
							Image: &v2alpha1.AgentImageConfig{Name: "agent:7.80.2-fips-full"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "tagged CCR image name without JMX suffix ignores JMX fields",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {
							Image: &v2alpha1.AgentImageConfig{
								Name:       "agent:7.80.2",
								Tag:        "7.80.2-jmx",
								JMXEnabled: true,
							},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := NewDefaultClusterChecksRunnerDeployment(tt.dda.GetObjectMeta(), &tt.dda.Spec)
			assertJMXUseContainerSupportEnv(t, deployment.Spec.Template.Spec.Containers[0].Env, tt.want)
		})
	}
}

func assertJMXUseContainerSupportEnv(t *testing.T, envVars []corev1.EnvVar, want bool) {
	t.Helper()

	count := 0
	for _, envVar := range envVars {
		if envVar.Name != common.DDJMXUseContainerSupport {
			continue
		}
		count++
		assert.Equal(t, "true", envVar.Value)
		assert.Nil(t, envVar.ValueFrom)
	}

	if want {
		assert.Equal(t, 1, count)
	} else {
		assert.Zero(t, count)
	}
}
