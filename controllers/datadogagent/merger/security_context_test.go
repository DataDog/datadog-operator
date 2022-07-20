// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"testing"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestAddCapabilitiesToContainer(t *testing.T) {
	tests := []struct {
		name                   string
		existingContainers     []corev1.Container
		capabilities           []corev1.Capability
		addToContainerWithName commonv1.AgentContainerName
		expectedCapabilities   map[commonv1.AgentContainerName][]corev1.Capability
	}{
		{
			name: "Add to container without capabilities defined",
			existingContainers: []corev1.Container{
				{
					Name:            string(commonv1.TraceAgentContainerName),
					SecurityContext: nil,
				},
			},
			capabilities: []corev1.Capability{
				"AUDIT_CONTROL",
				"AUDIT_READ",
			},
			addToContainerWithName: commonv1.TraceAgentContainerName,
			expectedCapabilities: map[commonv1.AgentContainerName][]corev1.Capability{
				commonv1.TraceAgentContainerName: {
					"AUDIT_CONTROL",
					"AUDIT_READ",
				},
			},
		},
		{
			name: "Add to container with some capabilities already defined",
			existingContainers: []corev1.Container{
				{
					Name: string(commonv1.TraceAgentContainerName),
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{
								"AUDIT_CONTROL",
							},
						},
					},
				},
			},
			capabilities: []corev1.Capability{
				"AUDIT_READ",
			},
			addToContainerWithName: commonv1.TraceAgentContainerName,
			expectedCapabilities: map[commonv1.AgentContainerName][]corev1.Capability{
				commonv1.TraceAgentContainerName: {
					"AUDIT_CONTROL",
					"AUDIT_READ",
				},
			},
		},
		{
			name: "Add to specific container when there are multiple defined",
			existingContainers: []corev1.Container{
				{
					Name:            string(commonv1.TraceAgentContainerName),
					SecurityContext: nil,
				},
				{
					Name:            string(commonv1.SystemProbeContainerName),
					SecurityContext: nil,
				},
			},
			capabilities: []corev1.Capability{
				"AUDIT_CONTROL",
				"AUDIT_READ",
			},
			addToContainerWithName: commonv1.SystemProbeContainerName,
			expectedCapabilities: map[commonv1.AgentContainerName][]corev1.Capability{
				commonv1.TraceAgentContainerName: nil,
				commonv1.SystemProbeContainerName: {
					"AUDIT_CONTROL",
					"AUDIT_READ",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			securityContextManager := securityContextManagerImpl{
				podTmpl: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: test.existingContainers,
					},
				},
			}

			securityContextManager.AddCapabilitiesToContainer(test.capabilities, test.addToContainerWithName)

			for _, container := range securityContextManager.podTmpl.Spec.Containers {
				expectedCapabilities := test.expectedCapabilities[commonv1.AgentContainerName(container.Name)]
				if len(expectedCapabilities) > 0 {
					assert.Equal(t, expectedCapabilities, container.SecurityContext.Capabilities.Add)
				} else {
					assert.True(t, container.SecurityContext == nil ||
						container.SecurityContext.Capabilities == nil ||
						len(container.SecurityContext.Capabilities.Add) == 0)
				}
			}

		})
	}
}
