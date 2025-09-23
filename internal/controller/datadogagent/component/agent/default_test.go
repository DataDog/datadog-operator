package agent

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
)

func TestVolumesForAgent_ProfileDependencyHack(t *testing.T) {
	tests := []struct {
		name                string
		dda                 metav1.Object
		requiredContainers  []apicommon.AgentContainerName
		expectedSeccompName string
		expectedInstallName string
	}{
		{
			name: "regular DDAI without profile label",
			dda: &metav1.ObjectMeta{
				Name:      "datadog-agent",
				Namespace: "default",
				Labels:    map[string]string{},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "datadog-agent-system-probe-seccomp",
			expectedInstallName: "datadog-agent-install-info",
		},
		{
			name: "profile DDAI with profile and DatadogAgent labels",
			dda: &metav1.ObjectMeta{
				Name:      "my-profile",
				Namespace: "default",
				Labels: map[string]string{
					constants.ProfileLabelKey:          "my-profile",
					apicommon.DatadogAgentNameLabelKey: "datadog-agent",
				},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "datadog-agent-system-probe-seccomp",
			expectedInstallName: "datadog-agent-install-info",
		},
		{
			name: "default profile DDAI (same name as original DDA, no profile label)",
			dda: &metav1.ObjectMeta{
				Name:      "datadog-agent",
				Namespace: "default",
				Labels: map[string]string{
					apicommon.DatadogAgentNameLabelKey: "datadog-agent",
				},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "datadog-agent-system-probe-seccomp",
			expectedInstallName: "datadog-agent-install-info",
		},
		{
			name: "DDAI without DatadogAgent label (edge case)",
			dda: &metav1.ObjectMeta{
				Name:      "my-profile",
				Namespace: "default",
				Labels:    map[string]string{
					// Missing DatadogAgent label - falls back to using DDAI name
				},
			},
			requiredContainers:  []apicommon.AgentContainerName{apicommon.SystemProbeContainerName},
			expectedSeccompName: "my-profile-system-probe-seccomp",
			expectedInstallName: "my-profile-install-info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volumes := volumesForAgent(tt.dda, tt.requiredContainers)

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

func TestCommonEnvVars_ProfileDependencyHack(t *testing.T) {
	tests := []struct {
		name                string
		dda                 metav1.Object
		expectedServiceName string
		expectedSecretName  string
	}{
		{
			name: "regular DDAI without profile label",
			dda: &metav1.ObjectMeta{
				Name:      "datadog-agent",
				Namespace: "default",
				Labels:    map[string]string{},
			},
			expectedServiceName: "datadog-agent-cluster-agent",
			expectedSecretName:  "datadog-agent-token",
		},
		{
			name: "profile DDAI with profile and DatadogAgent labels",
			dda: &metav1.ObjectMeta{
				Name:      "my-profile",
				Namespace: "default",
				Labels: map[string]string{
					constants.ProfileLabelKey:          "my-profile",
					apicommon.DatadogAgentNameLabelKey: "datadog-agent",
				},
			},
			expectedServiceName: "datadog-agent-cluster-agent",
			expectedSecretName:  "datadog-agent-token",
		},
		{
			name: "DDAI without DatadogAgent label (edge case)",
			dda: &metav1.ObjectMeta{
				Name:      "my-profile",
				Namespace: "default",
				Labels:    map[string]string{
					// Missing DatadogAgent label - falls back to using DDAI name
				},
			},
			expectedServiceName: "my-profile-cluster-agent",
			expectedSecretName:  "my-profile-token",
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
