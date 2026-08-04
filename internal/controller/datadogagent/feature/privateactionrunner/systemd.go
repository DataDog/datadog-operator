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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

const (
	systemdEnabledAnnotation              = "agent.datadoghq.com/private-action-runner-systemd-enabled"
	systemdJournalStorageAnnotation       = "agent.datadoghq.com/private-action-runner-systemd-journal-storage"
	systemdJournalVacuumEnabledAnnotation = "agent.datadoghq.com/private-action-runner-systemd-journal-vacuum-enabled"

	systemdJournalStoragePersistent = "persistent"
	systemdJournalStorageVolatile   = "volatile"
	systemdJournalStorageBoth       = "both"
)

type systemdHostConfig struct {
	journalStorage       string
	journalVacuumEnabled bool
}

func systemdHostConfigFromAnnotations(annotations map[string]string) (systemdHostConfig, error) {
	enabled, err := strictBoolAnnotation(annotations, systemdEnabledAnnotation)
	if err != nil {
		return systemdHostConfig{}, err
	}

	vacuumEnabled, err := strictBoolAnnotation(annotations, systemdJournalVacuumEnabledAnnotation)
	if err != nil {
		return systemdHostConfig{}, err
	}

	storage, storageSet := annotations[systemdJournalStorageAnnotation]
	if !enabled {
		if storageSet || vacuumEnabled {
			return systemdHostConfig{}, fmt.Errorf("annotation %q must be true when configuring systemd journal access", systemdEnabledAnnotation)
		}
		return systemdHostConfig{}, nil
	}

	if !storageSet || storage == "" {
		return systemdHostConfig{}, fmt.Errorf("annotation %q is required when %q is true", systemdJournalStorageAnnotation, systemdEnabledAnnotation)
	}

	switch storage {
	case systemdJournalStoragePersistent, systemdJournalStorageVolatile, systemdJournalStorageBoth:
		return systemdHostConfig{journalStorage: storage, journalVacuumEnabled: vacuumEnabled}, nil
	default:
		return systemdHostConfig{}, fmt.Errorf("invalid value %q for annotation %q (allowed values: persistent, volatile, both)", storage, systemdJournalStorageAnnotation)
	}
}

func strictBoolAnnotation(annotations map[string]string, name string) (bool, error) {
	raw, set := annotations[name]
	if !set || raw == "false" {
		return false, nil
	}
	if raw == "true" {
		return true, nil
	}
	return false, fmt.Errorf("invalid value %q for annotation %q (allowed values: true, false)", raw, name)
}

func (config systemdHostConfig) addVolumeMounts(managers feature.PodTemplateManagers) {
	if config.journalStorage == "" {
		return
	}

	addHostPathVolumeMount(managers, "host-machine-id", "/etc/machine-id", corev1.HostPathFile, true)
	addHostPathVolumeMount(managers, "host-manager-bus-socket", "/run/dbus/system_bus_socket", corev1.HostPathSocket, true)
	// Mount the runtime directory so journald socket replacements remain visible.
	addHostPathVolumeMount(managers, "host-journald-runtime", "/run/systemd/journal", corev1.HostPathDirectory, true)

	journalReadOnly := !config.journalVacuumEnabled
	if config.journalStorage == systemdJournalStoragePersistent || config.journalStorage == systemdJournalStorageBoth {
		addHostPathVolumeMount(managers, "host-persistent-journal", "/var/log/journal", corev1.HostPathDirectory, journalReadOnly)
	}
	if config.journalStorage == systemdJournalStorageVolatile || config.journalStorage == systemdJournalStorageBoth {
		addHostPathVolumeMount(managers, "host-volatile-journal", "/run/log/journal", corev1.HostPathDirectory, journalReadOnly)
	}
}

func addHostPathVolumeMount(managers feature.PodTemplateManagers, name, hostPath string, hostPathType corev1.HostPathType, readOnly bool) {
	vol, volMount := volume.GetVolumes(name, hostPath, "/host"+hostPath, readOnly)
	vol.HostPath.Type = new(hostPathType)
	managers.Volume().AddVolume(&vol)
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.PrivateActionRunnerContainerName)
}
