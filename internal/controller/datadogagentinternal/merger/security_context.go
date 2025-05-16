// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"sort"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// SecurityContextManager use to add Security Context settings to containers.
type SecurityContextManager interface {
	// AddCapabilitiesToContainer Adds capabilities to a container in the PodTemplate.
	AddCapabilitiesToContainer(capabilities []corev1.Capability, containerName common.AgentContainerName)
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

func (impl *securityContextManagerImpl) AddCapabilitiesToContainer(capabilities []corev1.Capability, containerName common.AgentContainerName) {
	for i, container := range impl.podTmpl.Spec.Containers {
		if container.Name == string(containerName) {
			if container.SecurityContext == nil {
				impl.podTmpl.Spec.Containers[i].SecurityContext = &corev1.SecurityContext{}
			}
			if impl.podTmpl.Spec.Containers[i].SecurityContext.Capabilities == nil {
				impl.podTmpl.Spec.Containers[i].SecurityContext.Capabilities = &corev1.Capabilities{}
			}
			impl.podTmpl.Spec.Containers[i].SecurityContext.Capabilities.Add = SortAndUnique(append(impl.podTmpl.Spec.Containers[i].SecurityContext.Capabilities.Add, capabilities...))

			return
		}
	}
}

func SortAndUnique(in []corev1.Capability) []corev1.Capability {
	c := capabilitiesSorted(in)
	n := c.Len()
	if n == 0 {
		return in
	}
	sort.Sort(c)

	k := 0
	for i := 1; i < n; i++ {
		if c.Less(k, i) {
			k++
			c.Swap(k, i)
		}
	}

	return c[:k+1]
}

// capabilitiesSorted used to sort and find and unique
type capabilitiesSorted []corev1.Capability

func (c capabilitiesSorted) Len() int {
	return len(c)
}
func (c capabilitiesSorted) Less(i, j int) bool {
	return c[i] < c[j]
}

func (c capabilitiesSorted) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}
