// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	corev1 "k8s.io/api/core/v1"
)

// SecurityContextManager use to add Security Context settings to containers.
type SecurityContextManager interface {
	// AddCapabilitiesToContainer Adds capabilities to a container in the PodTemplate.
	AddCapabilitiesToContainer(capabilities []corev1.Capability, containerName commonv1.AgentContainerName)
}

// NewSecurityContextManager returns a new instance of the SecurityContextManager
func NewSecurityContextManager(podTmpl *corev1.PodTemplateSpec) SecurityContextManager {
	return &securityContextManagerImpl{
		podTmpl: podTmpl,
	}
}

type securityContextManagerImpl struct {
	podTmpl *corev1.PodTemplateSpec
}

func (impl *securityContextManagerImpl) AddCapabilitiesToContainer(capabilities []corev1.Capability, containerName commonv1.AgentContainerName) {
	for i, container := range impl.podTmpl.Spec.Containers {
		if container.Name == string(containerName) {
			if container.SecurityContext == nil {
				impl.podTmpl.Spec.Containers[i].SecurityContext = &corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Add: capabilities,
					},
				}
			} else if container.SecurityContext.Capabilities == nil {
				impl.podTmpl.Spec.Containers[i].SecurityContext.Capabilities = &corev1.Capabilities{
					Add: capabilities,
				}
			} else {
				// TODO add deduplication
				impl.podTmpl.Spec.Containers[i].SecurityContext.Capabilities.Add = append(
					impl.podTmpl.Spec.Containers[i].SecurityContext.Capabilities.Add,
					capabilities...,
				)
			}
			return
		}
	}
}
