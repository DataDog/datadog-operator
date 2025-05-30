// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This file contains definitions of volumes used in the agent specs

// GetVolumeForConfig return the volume that contains the agent config
func GetVolumeForConfig() corev1.Volume {
	return corev1.Volume{
		Name: ConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForConfd return the volume that contains the agent confd config files
func GetVolumeForConfd() corev1.Volume {
	return corev1.Volume{
		Name: ConfdVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForChecksd return the volume that contains the agent confd config files
func GetVolumeForChecksd() corev1.Volume {
	return corev1.Volume{
		Name: ChecksdVolumeName,
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
		Name: AuthVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForLogs return the Volume that should container generated logs
func GetVolumeForLogs() corev1.Volume {
	return corev1.Volume{
		Name: LogDatadogVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeInstallInfo return the Volume that should install-info file
func GetVolumeInstallInfo(owner metav1.Object) corev1.Volume {
	return corev1.Volume{
		Name: InstallInfoVolumeName,
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
		Name: ProcdirVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: ProcdirHostPath,
			},
		},
	}
}

// GetVolumeForCgroups returns the volume that contains the cgroup directory
func GetVolumeForCgroups() corev1.Volume {
	return corev1.Volume{
		Name: CgroupsVolumeName,
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
		Name: DogstatsdSocketVolumeName,
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
		Name:      ConfigVolumeName,
		MountPath: ConfigVolumePath,
	}
}

// GetVolumeMountForConfd return the VolumeMount that contains the agent confd config files
func GetVolumeMountForConfd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      ConfdVolumeName,
		MountPath: ConfdVolumePath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForChecksd return the VolumeMount that contains the agent checksd config files
func GetVolumeMountForChecksd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      ChecksdVolumeName,
		MountPath: ChecksdVolumePath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForRmCorechecks return the VolumeMount that overwrites the corechecks directory
func GetVolumeMountForRmCorechecks() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "remove-corechecks",
		MountPath: fmt.Sprintf("%s/%s", ConfigVolumePath, "conf.d"),
	}
}

// GetVolumeMountForAuth returns the VolumeMount that contains the authentication information
func GetVolumeMountForAuth(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      AuthVolumeName,
		MountPath: AuthVolumePath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeMountForLogs return the VolumeMount for the container generated logs
func GetVolumeMountForLogs() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      LogDatadogVolumeName,
		MountPath: LogDatadogVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeForTmp return the Volume use for /tmp
func GetVolumeForTmp() corev1.Volume {
	return corev1.Volume{
		Name: TmpVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeMountForTmp return the VolumeMount for /tmp
func GetVolumeMountForTmp() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      TmpVolumeName,
		MountPath: TmpVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeForCertificates return the Volume use to store certificates
func GetVolumeForCertificates() corev1.Volume {
	return corev1.Volume{
		Name: CertificatesVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeMountForCertificates return the VolumeMount use to store certificates
func GetVolumeMountForCertificates() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      CertificatesVolumeName,
		MountPath: CertificatesVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeMountForInstallInfo return the VolumeMount that contains the agent install-info file
func GetVolumeMountForInstallInfo() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      InstallInfoVolumeName,
		MountPath: InstallInfoVolumePath,
		SubPath:   InstallInfoVolumeSubPath,
		ReadOnly:  InstallInfoVolumeReadOnly,
	}
}

// GetVolumeMountForProc returns the VolumeMount that contains /proc
func GetVolumeMountForProc() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      ProcdirVolumeName,
		MountPath: ProcdirMountPath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForCgroups returns the VolumeMount that contains the cgroups info
func GetVolumeMountForCgroups() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      CgroupsVolumeName,
		MountPath: CgroupsMountPath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForDogstatsdSocket returns the VolumeMount with the Dogstatsd socket
func GetVolumeMountForDogstatsdSocket(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      DogstatsdSocketVolumeName,
		MountPath: DogstatsdSocketLocalPath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeForRuntimeSocket returns the Volume for the runtime socket
func GetVolumeForRuntimeSocket() corev1.Volume {
	return corev1.Volume{
		Name: CriSocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: RuntimeDirVolumePath,
			},
		},
	}
}

// GetVolumeMountForRuntimeSocket returns the VolumeMount with the runtime socket
func GetVolumeMountForRuntimeSocket(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      CriSocketVolumeName,
		MountPath: HostCriSocketPathPrefix + RuntimeDirVolumePath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeMountForSecurity returns the VolumeMount for datadog-agent-security
func GetVolumeMountForSecurity() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      SeccompSecurityVolumeName,
		MountPath: SeccompSecurityVolumePath,
	}
}

// GetVolumeForSecurity returns the Volume for datadog-agent-security
func GetVolumeForSecurity(owner metav1.Object) corev1.Volume {
	return corev1.Volume{
		Name: SeccompSecurityVolumeName,
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
		Name:      SeccompRootVolumeName,
		MountPath: SeccompRootVolumePath,
	}
}

// GetVolumeForSeccomp returns the volume for seccomp root
func GetVolumeForSeccomp() corev1.Volume {
	return corev1.Volume{
		Name: SeccompRootVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: SeccompRootPath,
			},
		},
	}
}
