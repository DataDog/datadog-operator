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
)

func TestSystemdHostConfigFromAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        systemdHostConfig
		wantErr     string
	}{
		{
			name: "disabled",
		},
		{
			name: "explicitly disabled with vacuum disabled",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerSystemdAnnotation:              "false",
				featureutils.EnablePrivateActionRunnerSystemdJournalVacuumAnnotation: "false",
			},
		},
		{
			name: "storage configured while disabled",
			annotations: map[string]string{
				featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation: string(systemdJournalStoragePersistent),
			},
			wantErr: "must be true when configuring systemd journal access",
		},
		{
			name: "vacuum enabled while systemd disabled",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerSystemdJournalVacuumAnnotation: "true",
			},
			wantErr: "must be true when configuring systemd journal access",
		},
		{
			name: "missing journal storage",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerSystemdAnnotation: "true",
			},
			wantErr: "is required",
		},
		{
			name: "invalid journal storage",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerSystemdAnnotation:         "true",
				featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation: "unknown",
			},
			wantErr: "allowed values: persistent, volatile, both",
		},
		{
			name: "invalid enabled boolean",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerSystemdAnnotation: "True",
			},
			wantErr: "allowed values: true, false",
		},
		{
			name: "invalid vacuum boolean",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerSystemdAnnotation:              "true",
				featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation:      string(systemdJournalStoragePersistent),
				featureutils.EnablePrivateActionRunnerSystemdJournalVacuumAnnotation: "1",
			},
			wantErr: "allowed values: true, false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := systemdHostConfigFromAnnotations(tt.annotations)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSystemdHostConfigAddVolumeMounts(t *testing.T) {
	type expectedMount struct {
		name         string
		hostPath     string
		mountPath    string
		hostPathType corev1.HostPathType
		readOnly     bool
	}

	baseMounts := []expectedMount{
		{
			name:         hostMachineIDVolumeName,
			hostPath:     "/etc/machine-id",
			mountPath:    "/host/etc/machine-id",
			hostPathType: corev1.HostPathFile,
			readOnly:     true,
		},
		{
			name:         hostManagerBusSocketVolumeName,
			hostPath:     "/run/dbus/system_bus_socket",
			mountPath:    "/host/run/dbus/system_bus_socket",
			hostPathType: corev1.HostPathSocket,
			readOnly:     true,
		},
		{
			name:         hostJournalControlSocketVolumeName,
			hostPath:     "/run/systemd/journal/io.systemd.journal",
			mountPath:    "/host/run/systemd/journal/io.systemd.journal",
			hostPathType: corev1.HostPathSocket,
			readOnly:     true,
		},
	}
	persistentJournalMount := expectedMount{
		name:         hostPersistentJournalVolumeName,
		hostPath:     "/var/log/journal",
		mountPath:    "/host/var/log/journal",
		hostPathType: corev1.HostPathDirectory,
		readOnly:     true,
	}
	volatileJournalMount := expectedMount{
		name:         hostVolatileJournalVolumeName,
		hostPath:     "/run/log/journal",
		mountPath:    "/host/run/log/journal",
		hostPathType: corev1.HostPathDirectory,
		readOnly:     true,
	}

	tests := []struct {
		name        string
		annotations map[string]string
		wantMounts  []expectedMount
	}{
		{
			name: "disabled",
		},
		{
			name: "persistent journal is read-only without vacuum",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerSystemdAnnotation:         "true",
				featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation: string(systemdJournalStoragePersistent),
			},
			wantMounts: append(append([]expectedMount{}, baseMounts...), persistentJournalMount),
		},
		{
			name: "volatile journal is writable with vacuum",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerSystemdAnnotation:              "true",
				featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation:      string(systemdJournalStorageVolatile),
				featureutils.EnablePrivateActionRunnerSystemdJournalVacuumAnnotation: "true",
			},
			wantMounts: append(append([]expectedMount{}, baseMounts...), expectedMount{
				name:         volatileJournalMount.name,
				hostPath:     volatileJournalMount.hostPath,
				mountPath:    volatileJournalMount.mountPath,
				hostPathType: volatileJournalMount.hostPathType,
				readOnly:     false,
			}),
		},
		{
			name: "both journals are selected",
			annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerSystemdAnnotation:         "true",
				featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation: string(systemdJournalStorageBoth),
			},
			wantMounts: append(append(append([]expectedMount{}, baseMounts...), persistentJournalMount), volatileJournalMount),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := systemdHostConfigFromAnnotations(tt.annotations)
			require.NoError(t, err)

			managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
			config.addVolumeMounts(managers)

			volumesByName := make(map[string]*corev1.Volume, len(managers.VolumeMgr.Volumes))
			for _, volume := range managers.VolumeMgr.Volumes {
				require.NotNil(t, volume.HostPath)
				assert.NotEqual(t, common.HostRunPath, volume.HostPath.Path, "the broad host /run path must not be mounted")
				volumesByName[volume.Name] = volume
			}

			mounts := managers.VolumeMountMgr.VolumeMountsByC[apicommon.PrivateActionRunnerContainerName]
			mountsByName := make(map[string]*corev1.VolumeMount, len(mounts))
			for _, mount := range mounts {
				assert.NotEqual(t, common.HostRunMountPath, mount.MountPath, "the broad host /run path must not be mounted")
				mountsByName[mount.Name] = mount
			}

			require.Len(t, volumesByName, len(tt.wantMounts))
			require.Len(t, mountsByName, len(tt.wantMounts))
			for _, want := range tt.wantMounts {
				volume, found := volumesByName[want.name]
				require.True(t, found, "volume %q should exist", want.name)
				assert.Equal(t, want.hostPath, volume.HostPath.Path)
				require.NotNil(t, volume.HostPath.Type)
				assert.Equal(t, want.hostPathType, *volume.HostPath.Type)

				mount, found := mountsByName[want.name]
				require.True(t, found, "mount %q should exist", want.name)
				assert.Equal(t, want.mountPath, mount.MountPath)
				assert.Equal(t, want.readOnly, mount.ReadOnly)
			}
		})
	}
}

func TestManageNodeAgentAddsConfiguredSystemdMounts(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-dda",
			Annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation:                     "true",
				featureutils.EnablePrivateActionRunnerSystemdAnnotation:              "true",
				featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation:      string(systemdJournalStoragePersistent),
				featureutils.EnablePrivateActionRunnerSystemdJournalVacuumAnnotation: "true",
			},
		},
	}
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	require.NoError(t, f.ManageNodeAgent(managers))

	mountsByName := make(map[string]*corev1.VolumeMount)
	for _, mount := range managers.VolumeMountMgr.VolumeMountsByC[apicommon.PrivateActionRunnerContainerName] {
		assert.NotEqual(t, common.HostRunMountPath, mount.MountPath)
		mountsByName[mount.Name] = mount
	}

	require.Contains(t, mountsByName, hostVarLogVolumeName)
	require.Contains(t, mountsByName, hostPersistentJournalVolumeName)
	require.Contains(t, mountsByName, hostMachineIDVolumeName)
	require.Contains(t, mountsByName, hostManagerBusSocketVolumeName)
	require.Contains(t, mountsByName, hostJournalControlSocketVolumeName)
	require.True(t, mountsByName[hostVarLogVolumeName].ReadOnly)
	require.False(t, mountsByName[hostPersistentJournalVolumeName].ReadOnly)
	require.True(t, mountsByName[hostMachineIDVolumeName].ReadOnly)
	require.True(t, mountsByName[hostManagerBusSocketVolumeName].ReadOnly)
	require.True(t, mountsByName[hostJournalControlSocketVolumeName].ReadOnly)
}

func TestManageNodeAgentRejectsInvalidSystemdConfigBeforeMutation(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				featureutils.EnablePrivateActionRunnerAnnotation:        "true",
				featureutils.EnablePrivateActionRunnerSystemdAnnotation: "true",
			},
		},
	}
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)

	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	err := f.ManageNodeAgent(managers)
	require.ErrorContains(t, err, featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation)
	assert.Empty(t, managers.VolumeMgr.Volumes)
	assert.Empty(t, managers.VolumeMountMgr.VolumeMountsByC)
}
