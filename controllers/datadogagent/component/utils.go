// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package component

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
)

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

// GetInstallInfoConfigMapName return the InstallInfo config map name base on the dda name
func GetInstallInfoConfigMapName(dda metav1.Object) string {
	return fmt.Sprintf("%s-install-info", dda.GetName())
}

// GetVolumeMountForConfd return the VolumeMount that contains the agent confd config files
func GetVolumeMountForConfd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.ConfdVolumeName,
		MountPath: apicommon.ConfdVolumePath,
		ReadOnly:  true,
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

// GetClusterAgentServiceName return the Cluster-Agent service name based on the DatadogAgent name
func GetClusterAgentServiceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentName return the Cluster-Agent name based on the DatadogAgent name
func GetClusterAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentVersion return the Cluster-Agent version based on the DatadogAgent info
func GetClusterAgentVersion(dda metav1.Object) string {
	// Todo implement this function
	return ""
}

// GetAgentName return the Agent name based on the DatadogAgent info
func GetAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultAgentResourceSuffix)
}

// GetAgentVersion return the Agent version based on the DatadogAgent info
func GetAgentVersion(dda metav1.Object) string {
	// TODO implement this method
	return ""
}

// GetClusterChecksRunnerName return the Cluster-Check-Runner name based on the DatadogAgent name
func GetClusterChecksRunnerName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterChecksRunnerResourceSuffix)
}
