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
	tests := map[string]struct {
		enabled string
		storage string
		vacuum  string
		wantErr string
	}{
		"disabled": {},
		"explicitly disabled with vacuum disabled": {enabled: "false", vacuum: "false"},
		"storage configured while disabled": {
			storage: systemdJournalStoragePersistent,
			wantErr: "must be true when configuring systemd journal access",
		},
		"vacuum enabled while systemd disabled": {
			vacuum:  "true",
			wantErr: "must be true when configuring systemd journal access",
		},
		"missing journal storage": {enabled: "true", wantErr: "is required"},
		"invalid journal storage": {
			enabled: "true",
			storage: "unknown",
			wantErr: "allowed values: persistent, volatile, both",
		},
		"invalid enabled boolean": {enabled: "True", wantErr: "allowed values: true, false"},
		"invalid vacuum boolean": {
			enabled: "true",
			storage: systemdJournalStoragePersistent,
			vacuum:  "1",
			wantErr: "allowed values: true, false",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			annotations := map[string]string{}
			if tt.enabled != "" {
				annotations[featureutils.EnablePrivateActionRunnerSystemdAnnotation] = tt.enabled
			}
			if tt.storage != "" {
				annotations[featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation] = tt.storage
			}
			if tt.vacuum != "" {
				annotations[featureutils.EnablePrivateActionRunnerSystemdJournalVacuumAnnotation] = tt.vacuum
			}

			got, err := systemdHostConfigFromAnnotations(annotations)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Zero(t, got)
		})
	}
}

func TestSystemdHostConfigAddVolumeMounts(t *testing.T) {
	type expectedMount struct {
		name         string
		hostPath     string
		hostPathType corev1.HostPathType
		writable     bool
	}

	baseMounts := []expectedMount{
		{
			name:         "host-machine-id",
			hostPath:     "/etc/machine-id",
			hostPathType: corev1.HostPathFile,
		},
		{
			name:         "host-manager-bus-socket",
			hostPath:     "/run/dbus/system_bus_socket",
			hostPathType: corev1.HostPathSocket,
		},
		{
			name:         "host-journald-runtime",
			hostPath:     "/run/systemd/journal",
			hostPathType: corev1.HostPathDirectory,
		},
	}
	persistentJournalMount := expectedMount{
		name:         "host-persistent-journal",
		hostPath:     "/var/log/journal",
		hostPathType: corev1.HostPathDirectory,
	}
	volatileJournalMount := expectedMount{
		name:         "host-volatile-journal",
		hostPath:     "/run/log/journal",
		hostPathType: corev1.HostPathDirectory,
	}
	writableVolatileJournalMount := volatileJournalMount
	writableVolatileJournalMount.writable = true

	tests := map[string]struct {
		storage           string
		vacuum            bool
		wantJournalMounts []expectedMount
	}{
		"disabled": {},
		"persistent journal is read-only without vacuum": {
			storage:           systemdJournalStoragePersistent,
			wantJournalMounts: []expectedMount{persistentJournalMount},
		},
		"volatile journal is writable with vacuum": {
			storage:           systemdJournalStorageVolatile,
			vacuum:            true,
			wantJournalMounts: []expectedMount{writableVolatileJournalMount},
		},
		"both journals are selected": {
			storage:           systemdJournalStorageBoth,
			wantJournalMounts: []expectedMount{persistentJournalMount, volatileJournalMount},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			annotations := map[string]string{}
			if tt.storage != "" {
				annotations[featureutils.EnablePrivateActionRunnerSystemdAnnotation] = "true"
				annotations[featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation] = tt.storage
			}
			if tt.vacuum {
				annotations[featureutils.EnablePrivateActionRunnerSystemdJournalVacuumAnnotation] = "true"
			}
			config, err := systemdHostConfigFromAnnotations(annotations)
			require.NoError(t, err)
			wantMounts := tt.wantJournalMounts
			if len(wantMounts) > 0 {
				wantMounts = append(append([]expectedMount{}, baseMounts...), wantMounts...)
			}

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

			require.Len(t, volumesByName, len(wantMounts))
			require.Len(t, mountsByName, len(wantMounts))
			for _, want := range wantMounts {
				volume, found := volumesByName[want.name]
				require.True(t, found, "volume %q should exist", want.name)
				assert.Equal(t, want.hostPath, volume.HostPath.Path)
				require.NotNil(t, volume.HostPath.Type)
				assert.Equal(t, want.hostPathType, *volume.HostPath.Type)

				mount, found := mountsByName[want.name]
				require.True(t, found, "mount %q should exist", want.name)
				assert.Equal(t, "/host"+want.hostPath, mount.MountPath)
				assert.Equal(t, !want.writable, mount.ReadOnly)
			}
		})
	}
}

func TestManageNodeAgentAddsConfiguredSystemdMounts(t *testing.T) {
	managers, err := manageNodeAgent(t, map[string]string{
		featureutils.EnablePrivateActionRunnerAnnotation:                     "true",
		featureutils.EnablePrivateActionRunnerSystemdAnnotation:              "true",
		featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation:      systemdJournalStoragePersistent,
		featureutils.EnablePrivateActionRunnerSystemdJournalVacuumAnnotation: "true",
	})
	require.NoError(t, err)

	mountsByName := make(map[string]*corev1.VolumeMount)
	for _, mount := range managers.VolumeMountMgr.VolumeMountsByC[apicommon.PrivateActionRunnerContainerName] {
		mountsByName[mount.Name] = mount
	}

	require.Contains(t, mountsByName, "host-persistent-journal")
	require.False(t, mountsByName["host-persistent-journal"].ReadOnly)
}

func TestManageNodeAgentRejectsInvalidSystemdConfigBeforeMutation(t *testing.T) {
	managers, err := manageNodeAgent(t, map[string]string{
		featureutils.EnablePrivateActionRunnerAnnotation:        "true",
		featureutils.EnablePrivateActionRunnerSystemdAnnotation: "true",
	})
	require.ErrorContains(t, err, featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation)
	assert.Empty(t, managers.VolumeMgr.Volumes)
	assert.Empty(t, managers.VolumeMountMgr.VolumeMountsByC)
}

func manageNodeAgent(t *testing.T, annotations map[string]string) (*fake.PodTemplateManagers, error) {
	t.Helper()
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}}
	f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
	f.Configure(dda, &v2alpha1.DatadogAgentSpec{}, nil)
	managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	return managers, f.ManageNodeAgent(managers)
}
