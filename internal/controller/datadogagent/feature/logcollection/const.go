// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logcollection

const (
	pointerVolumeName          = "pointerdir"
	pointerVolumePath          = "/opt/datadog-agent/run"
	podLogVolumeName           = "logpodpath"
	podLogVolumePath           = "/var/log/pods"
	containerLogVolumeName     = "logcontainerpath"
	containerLogVolumePath     = "/var/lib/docker/containers"
	symlinkContainerVolumeName = "symlinkcontainerpath"
	symlinkContainerVolumePath = "/var/log/containers"
)
