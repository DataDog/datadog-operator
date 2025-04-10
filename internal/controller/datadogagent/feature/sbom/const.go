// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

const (
	containerdDirVolumeName = "host-containerd-dir"
	containerdDirVolumePath = "/var/lib/containerd"
	containerdDirMountPath  = "/host/var/lib/containerd"
	criDirVolumeName        = "host-cri-dir"
	criDirVolumePath        = "/var/lib/containers"
	criDirMountPath         = "/host/var/lib/containers"

	agentAppArmorAnnotationKey   = "container.apparmor.security.beta.kubernetes.io/agent"
	agentAppArmorAnnotationValue = "unconfined"
)
