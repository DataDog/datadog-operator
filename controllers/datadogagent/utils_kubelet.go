// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	corev1 "k8s.io/api/core/v1"
)

func getKubeletEnvVars(dda *datadoghqv1alpha1.DatadogAgent) []corev1.EnvVar {
	kubeletVars := make([]corev1.EnvVar, 0)

	// Host valueFrom
	var kubeletHostValueFrom *corev1.EnvVarSource
	if dda.Spec.Agent.Config.Kubelet != nil && dda.Spec.Agent.Config.Kubelet.Host != nil {
		kubeletHostValueFrom = dda.Spec.Agent.Config.Kubelet.Host
	} else {
		kubeletHostValueFrom = &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: apicommon.FieldPathStatusHostIP,
			},
		}
	}

	kubeletVars = append(kubeletVars, corev1.EnvVar{
		Name:      datadoghqv1alpha1.DDKubeletHost,
		ValueFrom: kubeletHostValueFrom,
	})

	// TLS Verify
	if dda.Spec.Agent.Config.Kubelet != nil && dda.Spec.Agent.Config.Kubelet.TLSVerify != nil {
		kubeletVars = append(kubeletVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDKubeletTLSVerify,
			Value: apiutils.BoolToString(dda.Spec.Agent.Config.Kubelet.TLSVerify),
		})
	}

	// CA Path
	if dda.Spec.Agent.Config.Kubelet != nil && (dda.Spec.Agent.Config.Kubelet.AgentCAPath != "" || dda.Spec.Agent.Config.Kubelet.HostCAPath != "") {
		kubeletVars = append(kubeletVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDKubeletCAPath,
			Value: getAgentCAPath(dda),
		})
	}

	return kubeletVars
}

func getKubeletVolumes(dda *datadoghqv1alpha1.DatadogAgent) []corev1.Volume {
	if dda.Spec.Agent.Config.Kubelet == nil {
		return nil
	}

	if dda.Spec.Agent.Config.Kubelet.HostCAPath != "" {
		fileVolumeType := corev1.HostPathFile

		return []corev1.Volume{
			{
				Name: datadoghqv1alpha1.KubeletCAVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: dda.Spec.Agent.Config.Kubelet.HostCAPath,
						Type: &fileVolumeType,
					},
				},
			},
		}
	}

	return nil
}

func getKubeletVolumeMounts(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
	if dda.Spec.Agent.Config.Kubelet == nil {
		return nil
	}

	if dda.Spec.Agent.Config.Kubelet.HostCAPath != "" {
		return []corev1.VolumeMount{
			{
				Name:      datadoghqv1alpha1.KubeletCAVolumeName,
				MountPath: getAgentCAPath(dda),
				ReadOnly:  true,
			},
		}
	}

	return nil
}

func getAgentCAPath(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.Agent.Config.Kubelet != nil && dda.Spec.Agent.Config.Kubelet.AgentCAPath != "" {
		return dda.Spec.Agent.Config.Kubelet.AgentCAPath
	}

	return datadoghqv1alpha1.DefaultKubeletAgentCAPath
}
