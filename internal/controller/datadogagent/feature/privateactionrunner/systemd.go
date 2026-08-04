// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

type systemdJournalStorage string

const (
	systemdJournalStoragePersistent systemdJournalStorage = "persistent"
	systemdJournalStorageVolatile   systemdJournalStorage = "volatile"
	systemdJournalStorageBoth       systemdJournalStorage = "both"
)

type systemdHostConfig struct {
	enabled              bool
	journalStorage       systemdJournalStorage
	journalVacuumEnabled bool
}

func systemdHostConfigFromAnnotations(annotations map[string]string) (systemdHostConfig, error) {
	config := systemdHostConfig{}

	enabled, _, err := strictBoolAnnotation(annotations, featureutils.EnablePrivateActionRunnerSystemdAnnotation)
	if err != nil {
		return config, err
	}
	config.enabled = enabled

	vacuumEnabled, vacuumSet, err := strictBoolAnnotation(annotations, featureutils.EnablePrivateActionRunnerSystemdJournalVacuumAnnotation)
	if err != nil {
		return config, err
	}
	config.journalVacuumEnabled = vacuumEnabled

	storage, storageSet := annotations[featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation]
	if !config.enabled {
		if storageSet || (vacuumSet && vacuumEnabled) {
			return config, fmt.Errorf("annotation %q must be true when configuring systemd journal access", featureutils.EnablePrivateActionRunnerSystemdAnnotation)
		}
		return config, nil
	}

	if !storageSet || storage == "" {
		return config, fmt.Errorf("annotation %q is required when %q is true", featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation, featureutils.EnablePrivateActionRunnerSystemdAnnotation)
	}

	config.journalStorage = systemdJournalStorage(storage)
	switch config.journalStorage {
	case systemdJournalStoragePersistent, systemdJournalStorageVolatile, systemdJournalStorageBoth:
		return config, nil
	default:
		return systemdHostConfig{}, fmt.Errorf("invalid value %q for annotation %q (allowed values: persistent, volatile, both)", storage, featureutils.PrivateActionRunnerSystemdJournalStorageAnnotation)
	}
}

func strictBoolAnnotation(annotations map[string]string, name string) (value, set bool, err error) {
	raw, set := annotations[name]
	if !set {
		return false, false, nil
	}

	switch raw {
	case "true":
		return true, true, nil
	case "false":
		return false, true, nil
	default:
		return false, true, fmt.Errorf("invalid value %q for annotation %q (allowed values: true, false)", raw, name)
	}
}

func (config systemdHostConfig) addVolumeMounts(managers feature.PodTemplateManagers) {
	if !config.enabled {
		return
	}

	addHostPathVolumeMount(managers, hostMachineIDVolumeName, hostMachineIDHostPath, hostMachineIDMountPath, corev1.HostPathFile, true)
	addHostPathVolumeMount(managers, hostManagerBusSocketVolumeName, hostManagerBusSocketHostPath, hostManagerBusSocketMountPath, corev1.HostPathSocket, true)
	// Mount the runtime directory so journald socket replacements remain visible.
	addHostPathVolumeMount(managers, hostJournaldRuntimeVolumeName, hostJournaldRuntimeHostPath, hostJournaldRuntimeMountPath, corev1.HostPathDirectory, true)

	journalReadOnly := !config.journalVacuumEnabled
	if config.journalStorage == systemdJournalStoragePersistent || config.journalStorage == systemdJournalStorageBoth {
		addHostPathVolumeMount(managers, hostPersistentJournalVolumeName, hostPersistentJournalHostPath, hostPersistentJournalMountPath, corev1.HostPathDirectory, journalReadOnly)
	}
	if config.journalStorage == systemdJournalStorageVolatile || config.journalStorage == systemdJournalStorageBoth {
		addHostPathVolumeMount(managers, hostVolatileJournalVolumeName, hostVolatileJournalHostPath, hostVolatileJournalMountPath, corev1.HostPathDirectory, journalReadOnly)
	}
}

func addHostPathVolumeMount(managers feature.PodTemplateManagers, name, hostPath, mountPath string, hostPathType corev1.HostPathType, readOnly bool) {
	vol, volMount := volume.GetVolumes(name, hostPath, mountPath, readOnly)
	vol.HostPath.Type = new(hostPathType)
	managers.Volume().AddVolume(&vol)
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.PrivateActionRunnerContainerName)
}
