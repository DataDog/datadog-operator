// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/images"
	pkgutils "github.com/DataDog/datadog-operator/pkg/utils"
)

func TestPacketCaptureConfigFromAnnotations(t *testing.T) {
	tests := map[string]struct {
		value       *string
		wantEnabled bool
		wantErr     string
	}{
		"unset":               {},
		"explicitly enabled":  {value: new("true"), wantEnabled: true},
		"explicitly disabled": {value: new("false")},
		"invalid boolean":     {value: new("True"), wantErr: "allowed values: true, false"},
		"empty value":         {value: new(""), wantErr: "allowed values: true, false"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			annotations := map[string]string{}
			if tt.value != nil {
				annotations[featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation] = *tt.value
			}

			got, err := packetCaptureConfigFromAnnotations(annotations)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantEnabled, got.enabled)
		})
	}
}

func TestConfigureRequestsSystemProbeForPacketCapture(t *testing.T) {
	tests := map[string]struct {
		annotations     map[string]string
		wantAgent       bool
		wantSystemProbe bool
	}{
		"node PAR without packet capture": {
			annotations: map[string]string{featureutils.EnablePrivateActionRunnerAnnotation: "true"},
			wantAgent:   true,
		},
		"node PAR with packet capture": {
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation:              "true",
				featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation: "true",
			},
			wantAgent:       true,
			wantSystemProbe: true,
		},
		"packet capture without node PAR is ignored": {
			annotations: map[string]string{featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation: "true"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			reqComp := buildPrivateActionRunnerFeature(nil).Configure(dda, specWithAgentTag("7.86.0"), nil)

			assert.Equal(t, tt.wantAgent, reqComp.Agent.IsEnabled())
			if tt.wantSystemProbe {
				assert.Contains(t, reqComp.Agent.Containers, apicommon.SystemProbeContainerName)
			} else {
				assert.NotContains(t, reqComp.Agent.Containers, apicommon.SystemProbeContainerName)
			}
		})
	}
}

func TestPacketCaptureRequiresSupportingAgent(t *testing.T) {
	tests := []struct {
		name        string
		tag         string
		wantEnabled bool
	}{
		{name: "default image", wantEnabled: pkgutils.IsAboveMinVersion(images.AgentLatestVersion, packetCaptureMinVersion, new(true))},
		{name: "old agent", tag: "7.85.0"},
		{name: "minimum version", tag: "7.86.0", wantEnabled: true},
		{name: "prerelease", tag: "7.86.0-rc.1", wantEnabled: true},
		{name: "unknown version", tag: "dev-remote-pcap-5190445c", wantEnabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation:              "true",
				featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation: "true",
			}}}
			spec := &v2alpha1.DatadogAgentSpec{}
			if tt.tag != "" {
				spec = specWithAgentTag(tt.tag)
			}
			f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
			reqComp := f.Configure(dda, spec, nil)

			assert.Equal(t, tt.wantEnabled, f.packetCaptureConfig.enabled)
			if tt.wantEnabled {
				assert.Contains(t, reqComp.Agent.Containers, apicommon.SystemProbeContainerName)
			} else {
				assert.NotContains(t, reqComp.Agent.Containers, apicommon.SystemProbeContainerName)
			}
		})
	}
}

func TestPacketCaptureWithTypedPrivateActionRunner(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "node enabled", false: "node disabled"}[enabled], func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation: "true",
				featureutils.PrivateActionRunnerConfigDataAnnotation:          "invalid: [",
			}}}
			spec := specWithAgentTag("7.86.0")
			spec.Features = &v2alpha1.DatadogFeatures{PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{
				Enabled:          new(true),
				ActionsAllowlist: new([]string{"com.datadoghq.script.runPredefinedScript"}),
			}}
			spec.Override[v2alpha1.NodeAgentComponentName].CustomConfigurations = map[v2alpha1.AgentConfigFileName]v2alpha1.AgentCustomConfig{
				v2alpha1.AgentGeneralConfigFile: {Enabled: new(enabled)},
			}
			f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
			reqComp := f.Configure(dda, spec, nil)

			require.NoError(t, f.configErr)
			assert.Equal(t, enabled, reqComp.Agent.IsEnabled())
			assert.Equal(t, enabled, f.packetCaptureConfig.enabled)
			if !enabled {
				assert.NotContains(t, reqComp.Agent.Containers, apicommon.SystemProbeContainerName)
				return
			}
			assert.Contains(t, reqComp.Agent.Containers, apicommon.SystemProbeContainerName)
			config, err := parsePrivateActionRunnerConfig(f.nodeConfigData)
			require.NoError(t, err)
			assert.Equal(t, *spec.Features.PrivateActionRunner.ActionsAllowlist, config.ActionsAllowlist)
			managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: string(apicommon.CoreAgentContainerName), Args: []string{"agent", "run"}},
			}}})
			require.NoError(t, f.ManageNodeAgent(managers))
			assert.Contains(t, managers.PodTemplateSpec().Spec.Containers[0].Args, "-E="+PrivateActionRunnerConfigPath)
			assert.Contains(t, managers.EnvVarMgr.EnvVarsByC[apicommon.PrivateActionRunnerContainerName], &corev1.EnvVar{
				Name: "DD_PACKET_CAPTURE_SOCKET", Value: packetCaptureSocketPath,
			})
		})
	}
}

func specWithAgentTag(tag string) *v2alpha1.DatadogAgentSpec {
	return &v2alpha1.DatadogAgentSpec{Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.NodeAgentComponentName: {Image: &v2alpha1.AgentImageConfig{Tag: tag}},
	}}
}

func TestManageNodeAgentAddsPacketCaptureSocket(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		featureutils.EnablePrivateActionRunnerAnnotation:              "true",
		featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation: "true",
	}}}
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	f.Configure(dda, specWithAgentTag("7.86.0"), nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	require.NoError(t, f.ManageNodeAgent(managers))

	var captureVolume *corev1.Volume
	for _, vol := range managers.VolumeMgr.Volumes {
		if vol.Name == packetCaptureSocketVolumeName {
			captureVolume = vol
		}
	}
	require.NotNil(t, captureVolume, "capture socket volume should exist")
	assert.NotNil(t, captureVolume.EmptyDir, "capture socket volume should be an emptyDir")

	mountFor := func(container apicommon.AgentContainerName, name string) *corev1.VolumeMount {
		for _, mount := range managers.VolumeMountMgr.VolumeMountsByC[container] {
			if mount.Name == name {
				return mount
			}
		}
		return nil
	}

	sysProbeMount := mountFor(apicommon.SystemProbeContainerName, packetCaptureSocketVolumeName)
	require.NotNil(t, sysProbeMount)
	assert.Equal(t, "/var/run/sysprobe-capture", sysProbeMount.MountPath)
	assert.False(t, sysProbeMount.ReadOnly, "system-probe must be able to create the capture socket")

	parMount := mountFor(apicommon.PrivateActionRunnerContainerName, packetCaptureSocketVolumeName)
	require.NotNil(t, parMount)
	assert.Equal(t, "/var/run/sysprobe-capture", parMount.MountPath)
	assert.True(t, parMount.ReadOnly, "the runner only dials the capture socket")

	// The full system-probe socket must never be exposed to the runner.
	assert.Nil(t, mountFor(apicommon.PrivateActionRunnerContainerName, common.SystemProbeSocketVolumeName))
	sysProbeSocketMount := mountFor(apicommon.SystemProbeContainerName, common.SystemProbeSocketVolumeName)
	require.NotNil(t, sysProbeSocketMount)
	assert.False(t, sysProbeSocketMount.ReadOnly)

	socketEnv := &corev1.EnvVar{Name: "DD_PACKET_CAPTURE_SOCKET", Value: "/var/run/sysprobe-capture/capture.sock"}
	assert.Contains(t, managers.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName], socketEnv)
	assert.Contains(t, managers.EnvVarMgr.EnvVarsByC[apicommon.PrivateActionRunnerContainerName], socketEnv)
	assert.Contains(t, managers.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName], &corev1.EnvVar{Name: "DD_PACKET_CAPTURE_ENABLED", Value: "true"})
	assert.NotContains(t, managers.EnvVarMgr.EnvVarsByC[apicommon.PrivateActionRunnerContainerName], &corev1.EnvVar{Name: "DD_PACKET_CAPTURE_ENABLED", Value: "true"})

	assert.Contains(t, managers.SecurityContextMgr.CapabilitiesByC[apicommon.SystemProbeContainerName], corev1.Capability("NET_RAW"))
}

func TestManageNodeAgentWithoutPacketCaptureAddsNoSocket(t *testing.T) {
	managers, err := manageNodeAgent(t, map[string]string{
		featureutils.EnablePrivateActionRunnerAnnotation:              "true",
		featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation: "false",
	})
	require.NoError(t, err)

	for _, vol := range managers.VolumeMgr.Volumes {
		assert.NotEqual(t, packetCaptureSocketVolumeName, vol.Name)
		assert.NotEqual(t, common.SystemProbeSocketVolumeName, vol.Name)
	}
	assert.Empty(t, managers.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName])
	assert.Empty(t, managers.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName])
}

func TestManageNodeAgentRejectsInvalidPacketCaptureConfigBeforeMutation(t *testing.T) {
	managers, err := manageNodeAgent(t, map[string]string{
		featureutils.EnablePrivateActionRunnerAnnotation:              "true",
		featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation: "yes",
	})
	require.ErrorContains(t, err, featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation)
	assert.Empty(t, managers.VolumeMgr.Volumes)
	assert.Empty(t, managers.VolumeMountMgr.VolumeMountsByC)
}
