// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

const (
	PrivateActionRunnerConfigPath = "/etc/datadog-agent/privateactionrunner.yaml"

	privateActionRunnerVolumeNameSuffix = "privateactionrunner-config"
	privateActionRunnerFileName         = "privateactionrunner.yaml"
	privateActionRunnerSuffix           = "private-action-runner"

	hostVarLogVolumeName = "host-varlog"
	hostVarLogHostPath   = "/var/log"
	hostVarLogMountPath  = "/host/var/log"

	hostMachineIDVolumeName = "host-machine-id"
	hostMachineIDHostPath   = "/etc/machine-id"
	hostMachineIDMountPath  = "/host/etc/machine-id"

	hostManagerBusSocketVolumeName = "host-manager-bus-socket"
	hostManagerBusSocketHostPath   = "/run/dbus/system_bus_socket"
	hostManagerBusSocketMountPath  = "/host/run/dbus/system_bus_socket"

	hostJournalControlSocketVolumeName = "host-journal-control-socket"
	hostJournalControlSocketHostPath   = "/run/systemd/journal/io.systemd.journal"
	hostJournalControlSocketMountPath  = "/host/run/systemd/journal/io.systemd.journal"

	hostPersistentJournalVolumeName = "host-persistent-journal"
	hostPersistentJournalHostPath   = "/var/log/journal"
	hostPersistentJournalMountPath  = "/host/var/log/journal"

	hostVolatileJournalVolumeName = "host-volatile-journal"
	hostVolatileJournalHostPath   = "/run/log/journal"
	hostVolatileJournalMountPath  = "/host/run/log/journal"
)
