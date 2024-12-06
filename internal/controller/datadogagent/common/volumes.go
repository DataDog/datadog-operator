// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/crds/datadoghq/common"
)

// This file contains definitions of volumes used in the agent specs

// GetVolumeForConfig return the volume that contains the agent config
func GetVolumeForConfig() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.ConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForConfd return the volume that contains the agent confd config files
func GetVolumeForConfd() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.ConfdVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForChecksd return the volume that contains the agent confd config files
func GetVolumeForChecksd() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.ChecksdVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForRmCorechecks return the volume that overwrites the corecheck directory
func GetVolumeForRmCorechecks() corev1.Volume {
	return corev1.Volume{
		Name: "remove-corechecks",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForAuth return the Volume container authentication information
func GetVolumeForAuth() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.AuthVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForLogs return the Volume that should container generated logs
func GetVolumeForLogs() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.LogDatadogVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeInstallInfo return the Volume that should install-info file
func GetVolumeInstallInfo(owner metav1.Object) corev1.Volume {
	return corev1.Volume{
		Name: apicommon.InstallInfoVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: GetInstallInfoConfigMapName(owner),
				},
			},
		},
	}
}

// GetVolumeForProc returns the volume with /proc
func GetVolumeForProc() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.ProcdirVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: apicommon.ProcdirHostPath,
			},
		},
	}
}

// GetVolumeForCgroups returns the volume that contains the cgroup directory
func GetVolumeForCgroups() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.CgroupsVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/sys/fs/cgroup",
			},
		},
	}
}

// GetVolumeForDogstatsd returns the volume with the Dogstatsd socket
func GetVolumeForDogstatsd() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.DogstatsdSocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetInstallInfoConfigMapName return the InstallInfo config map name base on the dda name
func GetInstallInfoConfigMapName(dda metav1.Object) string {
	return fmt.Sprintf("%s-install-info", dda.GetName())
}

// GetVolumeMountForConfig return the VolumeMount that contains the agent config
func GetVolumeMountForConfig() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.ConfigVolumeName,
		MountPath: apicommon.ConfigVolumePath,
	}
}

// GetVolumeMountForConfd return the VolumeMount that contains the agent confd config files
func GetVolumeMountForConfd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.ConfdVolumeName,
		MountPath: apicommon.ConfdVolumePath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForChecksd return the VolumeMount that contains the agent checksd config files
func GetVolumeMountForChecksd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.ChecksdVolumeName,
		MountPath: apicommon.ChecksdVolumePath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForRmCorechecks return the VolumeMount that overwrites the corechecks directory
func GetVolumeMountForRmCorechecks() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "remove-corechecks",
		MountPath: fmt.Sprintf("%s/%s", apicommon.ConfigVolumePath, "conf.d"),
	}
}

// GetVolumeMountForAuth returns the VolumeMount that contains the authentication information
func GetVolumeMountForAuth(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.AuthVolumeName,
		MountPath: apicommon.AuthVolumePath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeMountForLogs return the VolumeMount for the container generated logs
func GetVolumeMountForLogs() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.LogDatadogVolumeName,
		MountPath: apicommon.LogDatadogVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeForTmp return the Volume use for /tmp
func GetVolumeForTmp() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.TmpVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeMountForTmp return the VolumeMount for /tmp
func GetVolumeMountForTmp() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.TmpVolumeName,
		MountPath: apicommon.TmpVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeForCertificates return the Volume use to store certificates
func GetVolumeForCertificates() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.CertificatesVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeMountForCertificates return the VolumeMount use to store certificates
func GetVolumeMountForCertificates() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.CertificatesVolumeName,
		MountPath: apicommon.CertificatesVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeMountForInstallInfo return the VolumeMount that contains the agent install-info file
func GetVolumeMountForInstallInfo() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.InstallInfoVolumeName,
		MountPath: apicommon.InstallInfoVolumePath,
		SubPath:   apicommon.InstallInfoVolumeSubPath,
		ReadOnly:  apicommon.InstallInfoVolumeReadOnly,
	}
}

// GetVolumeMountForProc returns the VolumeMount that contains /proc
func GetVolumeMountForProc() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.ProcdirVolumeName,
		MountPath: apicommon.ProcdirMountPath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForCgroups returns the VolumeMount that contains the cgroups info
func GetVolumeMountForCgroups() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.CgroupsVolumeName,
		MountPath: apicommon.CgroupsMountPath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForDogstatsdSocket returns the VolumeMount with the Dogstatsd socket
func GetVolumeMountForDogstatsdSocket(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.DogstatsdSocketVolumeName,
		MountPath: apicommon.DogstatsdSocketLocalPath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeForRuntimeSocket returns the Volume for the runtime socket
func GetVolumeForRuntimeSocket() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.CriSocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: apicommon.RuntimeDirVolumePath,
			},
		},
	}
}

// GetVolumeMountForRuntimeSocket returns the VolumeMount with the runtime socket
func GetVolumeMountForRuntimeSocket(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.CriSocketVolumeName,
		MountPath: apicommon.HostCriSocketPathPrefix + apicommon.RuntimeDirVolumePath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeMountForSecurity returns the VolumeMount for datadog-agent-security
func GetVolumeMountForSecurity() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.SeccompSecurityVolumeName,
		MountPath: apicommon.SeccompSecurityVolumePath,
	}
}

// GetVolumeForSecurity returns the Volume for datadog-agent-security
func GetVolumeForSecurity(owner metav1.Object) corev1.Volume {
	return corev1.Volume{
		Name: apicommon.SeccompSecurityVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: GetDefaultSeccompConfigMapName(owner),
				},
			},
		},
	}
}

// GetVolumeMountForSeccomp returns the VolumeMount for seccomp root
func GetVolumeMountForSeccomp() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.SeccompRootVolumeName,
		MountPath: apicommon.SeccompRootVolumePath,
	}
}

// GetVolumeForSeccomp returns the volume for seccomp root
func GetVolumeForSeccomp() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.SeccompRootVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: apicommon.SeccompRootPath,
			},
		},
	}
}
