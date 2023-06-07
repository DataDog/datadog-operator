// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
)

// Provider is used to override a corev1.PodTemplateSpec with a provider-specific override
func Provider(pod *corev1.PodTemplateSpec, p kubernetes.Provider, features []feature.Feature) *corev1.PodTemplateSpec {
	switch p.Name {
	case kubernetes.GCPCosContainerdProvider:
		gcpCosOverride(pod, features)
	case kubernetes.GCPCosProvider:
		gcpCosOverride(pod, features)
	}
	return pod
}

func gcpCosOverride(pod *corev1.PodTemplateSpec, features []feature.Feature) {
	// GCP cos does not allow default kernel headers paths to be mounted in system probe
	for _, f := range features {
		if f.ID() == feature.OOMKillIDType || f.ID() == feature.TCPQueueLengthIDType {
			podManagers := feature.NewPodTemplateManagers(pod)
			podManagers.VolumeMount().RemoveVolumeMount(apicommon.SrcVolumeName)
			podManagers.Volume().RemoveVolume(apicommon.SrcVolumeName)

		}
	}
}
