// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// FinalizeAppArmorProfile applies Kubernetes-version compatibility to AppArmor
// settings on a completed Pod template. It must run after features and overrides
// have finished mutating the template, but before the workload is hashed and sent
// to the API server.
//
// On Kubernetes 1.30 and later, it moves deprecated container-scoped AppArmor
// annotations to securityContext.appArmorProfile. Older Kubernetes versions keep
// the annotations. An explicitly configured appArmorProfile field takes precedence
// over an annotation.
func FinalizeAppArmorProfile(podTemplate *corev1.PodTemplateSpec, platformInfo kubernetes.PlatformInfo) {
	if podTemplate == nil || !platformInfo.SupportsAppArmorProfile() {
		return
	}

	for annotation, value := range podTemplate.Annotations {
		containerName, found := strings.CutPrefix(annotation, AppArmorAnnotationKey+"/")
		if !found {
			continue
		}

		setAppArmorProfile(podTemplate.Spec.Containers, containerName, value)
		setAppArmorProfile(podTemplate.Spec.InitContainers, containerName, value)
		delete(podTemplate.Annotations, annotation)
	}

	if len(podTemplate.Annotations) == 0 {
		podTemplate.Annotations = nil
	}
}

func setAppArmorProfile(containers []corev1.Container, containerName, value string) {
	for i := range containers {
		if containers[i].Name != containerName {
			continue
		}

		if containers[i].SecurityContext == nil {
			containers[i].SecurityContext = &corev1.SecurityContext{}
		}
		if containers[i].SecurityContext.AppArmorProfile == nil {
			containers[i].SecurityContext.AppArmorProfile = appArmorProfileFromAnnotation(value)
		}
		return
	}
}

func appArmorProfileFromAnnotation(value string) *corev1.AppArmorProfile {
	switch value {
	case "", "runtime/default":
		// Kubernetes treats an empty legacy AppArmor annotation as runtime/default.
		return &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeRuntimeDefault}
	case "unconfined":
		return &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeUnconfined}
	default:
		return &corev1.AppArmorProfile{
			Type:             corev1.AppArmorProfileTypeLocalhost,
			LocalhostProfile: ptr.To(strings.TrimPrefix(value, "localhost/")),
		}
	}
}
