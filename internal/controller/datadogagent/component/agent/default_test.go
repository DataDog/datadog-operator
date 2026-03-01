package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

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
			volumes := volumesForAgent(tt.dda, &v2alpha1.DatadogAgentSpec{}, tt.requiredContainers)

			// Check install-info volume
			var installInfoVolume *corev1.Volume
			for i := range volumes {
				if volumes[i].Name == common.InstallInfoVolumeName {
					installInfoVolume = &volumes[i]
					break
				}
			}
			assert.NotNil(t, installInfoVolume, "install-info volume should exist")
			assert.Equal(t, tt.expectedInstallName, installInfoVolume.ConfigMap.Name)

			// Check seccomp volume if system probe is required
			if len(tt.requiredContainers) > 0 {
				var seccompVolume *corev1.Volume
				for i := range volumes {
					if volumes[i].Name == common.SeccompSecurityVolumeName {
						seccompVolume = &volumes[i]
						break
					}
				}
				assert.NotNil(t, seccompVolume, "seccomp security volume should exist")
				assert.Equal(t, tt.expectedSeccompName, seccompVolume.ConfigMap.Name)
			}
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
						Enabled: apiutils.NewBoolPointer(true),
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
						Enabled: apiutils.NewBoolPointer(true),
						Enforcement: &v2alpha1.CWSEnforcementConfig{
							Enabled: apiutils.NewBoolPointer(true),
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
