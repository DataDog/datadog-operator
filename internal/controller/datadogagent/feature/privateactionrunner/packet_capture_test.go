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
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
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
			reqComp := buildPrivateActionRunnerFeature(nil).Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

			assert.Equal(t, tt.wantAgent, reqComp.Agent.IsEnabled())
			if tt.wantSystemProbe {
				assert.Contains(t, reqComp.Agent.Containers, apicommon.SystemProbeContainerName)
			} else {
				assert.NotContains(t, reqComp.Agent.Containers, apicommon.SystemProbeContainerName)
			}
		})
	}
}

func TestManageNodeAgentAddsPacketCaptureSocket(t *testing.T) {
	managers, err := manageNodeAgent(t, map[string]string{
		featureutils.EnablePrivateActionRunnerAnnotation:              "true",
		featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation: "true",
	})
	require.NoError(t, err)

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
