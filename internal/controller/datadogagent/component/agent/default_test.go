package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func findVolume(volumes []corev1.Volume, name string) *corev1.Volume {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}

func TestVolumesForAgent(t *testing.T) {
	tests := []struct {
		name                string
		dda                 metav1.Object
		requiredContainers  []apicommon.AgentContainerName
		expectedSeccompName string
		expectedInstallName string
	}{
		{
			name: "foo DDA",
			dda: &metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				Labels:    map[string]string{},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "foo-system-probe-seccomp",
			expectedInstallName: "foo-install-info",
		},
		{
			name: "profile DDAI",
			dda: &metav1.ObjectMeta{
				Name:      "my-profile",
				Namespace: "default",
				Labels: map[string]string{
					constants.ProfileLabelKey:          "my-profile",
					apicommon.DatadogAgentNameLabelKey: "foo",
				},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "foo-system-probe-seccomp",
			expectedInstallName: "foo-install-info",
		},
		{
			name: "foo DDAI (same name as original DDA, no profile label)",
			dda: &metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				Labels: map[string]string{
					apicommon.DatadogAgentNameLabelKey: "foo",
				},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "foo-system-probe-seccomp",
			expectedInstallName: "foo-install-info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volumes := volumesForAgent(tt.dda, tt.requiredContainers)

			installVol := findVolume(volumes, common.InstallInfoVolumeName)
			assert.NotNil(t, installVol, "install-info volume should exist")
			assert.Equal(t, tt.expectedInstallName, installVol.ConfigMap.Name)

			seccompVol := findVolume(volumes, common.SeccompSecurityVolumeName)
			assert.NotNil(t, seccompVol, "seccomp security volume should exist")
			assert.Equal(t, tt.expectedSeccompName, seccompVol.ConfigMap.Name)
		})
	}
}

func TestCommonEnvVars(t *testing.T) {
	tests := []struct {
		name                string
		dda                 metav1.Object
		expectedServiceName string
		expectedSecretName  string
	}{
		{
			name: "foo DDA",
			dda: &metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				Labels:    map[string]string{},
			},
			expectedServiceName: "foo-cluster-agent",
			expectedSecretName:  "foo-token",
		},
		{
			name: "profile DDAI",
			dda: &metav1.ObjectMeta{
				Name:      "my-profile",
				Namespace: "default",
				Labels: map[string]string{
					constants.ProfileLabelKey:          "my-profile",
					apicommon.DatadogAgentNameLabelKey: "foo",
				},
			},
			expectedServiceName: "foo-cluster-agent",
			expectedSecretName:  "foo-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := commonEnvVars(tt.dda)

			// Find the relevant env vars
			var clusterAgentServiceName string
			var clusterAgentTokenName string

			for _, env := range envVars {
				switch env.Name {
				case common.DDClusterAgentKubeServiceName:
					clusterAgentServiceName = env.Value
				case common.DDClusterAgentTokenName:
					clusterAgentTokenName = env.Value
				}
			}

			assert.Equal(t, tt.expectedServiceName, clusterAgentServiceName)
			assert.Equal(t, tt.expectedSecretName, clusterAgentTokenName)
		})
	}
}

func TestEnvVarsForCoreAgentJMXUseContainerSupport(t *testing.T) {
	tests := []struct {
		name string
		dda  metav1.Object
		want bool
	}{
		{
			name: "metadata only does not add JMX env var",
			dda: &metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				Labels:    map[string]string{},
			},
			want: false,
		},
		{
			name: "DatadogAgent without override does not add JMX env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
			},
			want: false,
		},
		{
			name: "Node Agent override without image does not add JMX env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {},
					},
				},
			},
			want: false,
		},
		{
			name: "Cluster Agent JMX image does not add core Agent env var",
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
			name: "DatadogAgent JMX image flag adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Image: &v2alpha1.AgentImageConfig{JMXEnabled: true},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "DatadogAgent JMX image tag adds env var",
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
			want: true,
		},
		{
			name: "Agent image name with JMX suffix adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Image: &v2alpha1.AgentImageConfig{Name: "agent:7.80.2-jmx"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "full image name adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Image: &v2alpha1.AgentImageConfig{Name: "agent:7.80.2-full"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "FIPS full image name adds env var",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Image: &v2alpha1.AgentImageConfig{Name: "agent:7.80.2-fips-full"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "tagged image name without JMX suffix ignores JMX fields",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
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
		{
			name: "DatadogAgentInternal JMX image flag adds env var",
			dda: &v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Image: &v2alpha1.AgentImageConfig{JMXEnabled: true},
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJMXUseContainerSupportEnv(t, envVarsForCoreAgent(tt.dda), tt.want)
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

func TestDefaultSyscallsForSystemProbe(t *testing.T) {
	tests := []struct {
		name             string
		ddaSpec          *v2alpha1.DatadogAgentSpec
		expectedSyscalls []string
	}{
		{
			name: "default syscalls",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{},
			},
			expectedSyscalls: DefaultSyscallsForSystemProbe(),
		},
		{
			name: "cws enabled and enforcement disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(true),
					},
				},
			},
			expectedSyscalls: DefaultSyscallsForSystemProbe(),
		},
		{
			name: "cws enabled and enforcement enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(true),
						Enforcement: &v2alpha1.CWSEnforcementConfig{
							Enabled: ptr.To(true),
						},
					},
				},
			},
			expectedSyscalls: append(DefaultSyscallsForSystemProbe(), "kill"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syscalls := syscallsForSystemProbe(tt.ddaSpec)
			assert.Equal(t, tt.expectedSyscalls, syscalls)
		})
	}
}

func TestHostProfilerContainer(t *testing.T) {
	dda := &metav1.ObjectMeta{Name: "foo", Namespace: "default", Labels: map[string]string{}}

	containers := agentOptimizedContainers(dda, []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.HostProfiler,
	})
	assert.Len(t, containers, 2)

	c := containers[1]
	assert.Equal(t, string(apicommon.HostProfiler), c.Name)
	assert.NotNil(t, c.SecurityContext)
	// The component layer only sets ReadOnlyRootFilesystem; the feature's ManageNodeAgent sets
	// AllowPrivilegeEscalation, SeccompProfile, and Capabilities.
	assert.Nil(t, c.SecurityContext.Privileged, "host-profiler should not run as privileged")
	assert.NotNil(t, c.SecurityContext.ReadOnlyRootFilesystem)
	assert.True(t, *c.SecurityContext.ReadOnlyRootFilesystem)
}

func TestPrivateActionRunnerContainer(t *testing.T) {
	dda := &metav1.ObjectMeta{
		Name:      "test-dda",
		Namespace: "default",
	}

	containers := agentOptimizedContainers(dda, []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.PrivateActionRunnerContainerName,
	})

	assert.Len(t, containers, 2)

	parContainer := containers[1]
	assert.Equal(t, string(apicommon.PrivateActionRunnerContainerName), parContainer.Name)
	assert.Equal(t, agentImage(), parContainer.Image)
	assert.Equal(t, []string{
		"/opt/datadog-agent/embedded/bin/privateactionrunner",
		"run",
		"-c=/etc/datadog-agent/datadog.yaml",
		"-E=/etc/datadog-agent/privateactionrunner.yaml",
	}, parContainer.Command)

	assert.True(t, *parContainer.SecurityContext.ReadOnlyRootFilesystem)
	mountNames := make(map[string]bool)
	for _, m := range parContainer.VolumeMounts {
		mountNames[m.Name] = true
	}
	assert.True(t, mountNames[common.LogDatadogVolumeName])
	assert.True(t, mountNames[common.AuthVolumeName])
	assert.True(t, mountNames[common.ConfigVolumeName])
	assert.True(t, mountNames[common.DogstatsdSocketVolumeName])
	assert.True(t, mountNames[common.TmpVolumeName])
}
