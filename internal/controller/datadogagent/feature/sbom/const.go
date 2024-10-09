// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

const (
	containerdDirVolumeName = "host-containerd-dir"
	containerdDirVolumePath = "/var/lib/containerd"
	containerdDirMountPath  = "/host/var/lib/containerd"
	apkDirVolumeName        = "host-apk-dir"
	apkDirVolumePath        = "/var/lib/apk"
	apkDirMountPath         = "/host/var/lib/apk"
	dpkgDirVolumeName       = "host-dpkg-dir"
	dpkgDirVolumePath       = "/var/lib/dpkg"
	dpkgDirMountPath        = "/host/var/lib/dpkg"
	rpmDirVolumeName        = "host-rpm-dir"
	rpmDirVolumePath        = "/var/lib/rpm"
	rpmDirMountPath         = "/host/var/lib/rpm"
	redhatReleaseVolumeName = "etc-redhat-release"
	redhatReleaseVolumePath = "/etc/redhat-release"
	redhatReleaseMountPath  = "/host/etc/redhat-release"
	fedoraReleaseVolumeName = "etc-fedora-release"
	fedoraReleaseVolumePath = "/etc/fedora-release"
	fedoraReleaseMountPath  = "/host/etc/fedora-release"
	lsbReleaseVolumeName    = "etc-lsb-release"
	lsbReleaseVolumePath    = "/etc/lsb-release"
	lsbReleaseMountPath     = "/host/etc/lsb-release"
	systemReleaseVolumeName = "etc-system-release"
	systemReleaseVolumePath = "/etc/system-release"
	systemReleaseMountPath  = "/host/etc/system-release"
)
