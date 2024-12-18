// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// This file contains definitions of volumes used in the agent specs

// GetVolumeForConfig return the volume that contains the agent config
func GetVolumeForConfig() corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.ConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForConfd return the volume that contains the agent confd config files
func GetVolumeForConfd() corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.ConfdVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForChecksd return the volume that contains the agent confd config files
func GetVolumeForChecksd() corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.ChecksdVolumeName,
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
		Name: v2alpha1.AuthVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForLogs return the Volume that should container generated logs
func GetVolumeForLogs() corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.LogDatadogVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeInstallInfo return the Volume that should install-info file
func GetVolumeInstallInfo(owner metav1.Object) corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.InstallInfoVolumeName,
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
		Name: v2alpha1.ProcdirVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: v2alpha1.ProcdirHostPath,
			},
		},
	}
}

// GetVolumeForCgroups returns the volume that contains the cgroup directory
func GetVolumeForCgroups() corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.CgroupsVolumeName,
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
		Name: v2alpha1.DogstatsdSocketVolumeName,
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
		Name:      v2alpha1.ConfigVolumeName,
		MountPath: v2alpha1.ConfigVolumePath,
	}
}

// GetVolumeMountForConfd return the VolumeMount that contains the agent confd config files
func GetVolumeMountForConfd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.ConfdVolumeName,
		MountPath: v2alpha1.ConfdVolumePath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForChecksd return the VolumeMount that contains the agent checksd config files
func GetVolumeMountForChecksd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.ChecksdVolumeName,
		MountPath: v2alpha1.ChecksdVolumePath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForRmCorechecks return the VolumeMount that overwrites the corechecks directory
func GetVolumeMountForRmCorechecks() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "remove-corechecks",
		MountPath: fmt.Sprintf("%s/%s", v2alpha1.ConfigVolumePath, "conf.d"),
	}
}

// GetVolumeMountForAuth returns the VolumeMount that contains the authentication information
func GetVolumeMountForAuth(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.AuthVolumeName,
		MountPath: v2alpha1.AuthVolumePath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeMountForLogs return the VolumeMount for the container generated logs
func GetVolumeMountForLogs() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.LogDatadogVolumeName,
		MountPath: v2alpha1.LogDatadogVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeForTmp return the Volume use for /tmp
func GetVolumeForTmp() corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.TmpVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeMountForTmp return the VolumeMount for /tmp
func GetVolumeMountForTmp() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.TmpVolumeName,
		MountPath: v2alpha1.TmpVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeForCertificates return the Volume use to store certificates
func GetVolumeForCertificates() corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.CertificatesVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeMountForCertificates return the VolumeMount use to store certificates
func GetVolumeMountForCertificates() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.CertificatesVolumeName,
		MountPath: v2alpha1.CertificatesVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeMountForInstallInfo return the VolumeMount that contains the agent install-info file
func GetVolumeMountForInstallInfo() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.InstallInfoVolumeName,
		MountPath: v2alpha1.InstallInfoVolumePath,
		SubPath:   v2alpha1.InstallInfoVolumeSubPath,
		ReadOnly:  v2alpha1.InstallInfoVolumeReadOnly,
	}
}

// GetVolumeMountForProc returns the VolumeMount that contains /proc
func GetVolumeMountForProc() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.ProcdirVolumeName,
		MountPath: v2alpha1.ProcdirMountPath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForCgroups returns the VolumeMount that contains the cgroups info
func GetVolumeMountForCgroups() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.CgroupsVolumeName,
		MountPath: v2alpha1.CgroupsMountPath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForDogstatsdSocket returns the VolumeMount with the Dogstatsd socket
func GetVolumeMountForDogstatsdSocket(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.DogstatsdSocketVolumeName,
		MountPath: v2alpha1.DogstatsdSocketLocalPath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeForRuntimeSocket returns the Volume for the runtime socket
func GetVolumeForRuntimeSocket() corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.CriSocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: v2alpha1.RuntimeDirVolumePath,
			},
		},
	}
}

// GetVolumeMountForRuntimeSocket returns the VolumeMount with the runtime socket
func GetVolumeMountForRuntimeSocket(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.CriSocketVolumeName,
		MountPath: v2alpha1.HostCriSocketPathPrefix + v2alpha1.RuntimeDirVolumePath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeMountForSecurity returns the VolumeMount for datadog-agent-security
func GetVolumeMountForSecurity() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v2alpha1.SeccompSecurityVolumeName,
		MountPath: v2alpha1.SeccompSecurityVolumePath,
	}
}

// GetVolumeForSecurity returns the Volume for datadog-agent-security
func GetVolumeForSecurity(owner metav1.Object) corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.SeccompSecurityVolumeName,
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
		Name:      v2alpha1.SeccompRootVolumeName,
		MountPath: v2alpha1.SeccompRootVolumePath,
	}
}

// GetVolumeForSeccomp returns the volume for seccomp root
func GetVolumeForSeccomp() corev1.Volume {
	return corev1.Volume{
		Name: v2alpha1.SeccompRootVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: v2alpha1.SeccompRootPath,
			},
		},
	}
}
