// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestAddCapabilitiesToContainer(t *testing.T) {
	tests := []struct {
		name                   string
		existingContainers     []corev1.Container
		capabilities           []corev1.Capability
		addToContainerWithName common.AgentContainerName
		expectedCapabilities   map[common.AgentContainerName][]corev1.Capability
	}{
		{
			name: "Add to container without capabilities defined",
			existingContainers: []corev1.Container{
				{
					Name:            string(common.TraceAgentContainerName),
					SecurityContext: nil,
				},
			},
			capabilities: []corev1.Capability{
				"AUDIT_CONTROL",
				"AUDIT_READ",
			},
			addToContainerWithName: common.TraceAgentContainerName,
			expectedCapabilities: map[common.AgentContainerName][]corev1.Capability{
				common.TraceAgentContainerName: {
					"AUDIT_CONTROL",
					"AUDIT_READ",
				},
			},
		},
		{
			name: "Add to container with some capabilities already defined",
			existingContainers: []corev1.Container{
				{
					Name: string(common.TraceAgentContainerName),
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
			addToContainerWithName: common.TraceAgentContainerName,
			expectedCapabilities: map[common.AgentContainerName][]corev1.Capability{
				common.TraceAgentContainerName: {
					"AUDIT_CONTROL",
					"AUDIT_READ",
				},
			},
		},
		{
			name: "Add to specific container when there are multiple defined",
			existingContainers: []corev1.Container{
				{
					Name:            string(common.TraceAgentContainerName),
					SecurityContext: nil,
				},
				{
					Name:            string(common.SystemProbeContainerName),
					SecurityContext: nil,
				},
			},
			capabilities: []corev1.Capability{
				"AUDIT_CONTROL",
				"AUDIT_READ",
			},
			addToContainerWithName: common.SystemProbeContainerName,
			expectedCapabilities: map[common.AgentContainerName][]corev1.Capability{
				common.TraceAgentContainerName: nil,
				common.SystemProbeContainerName: {
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
				expectedCapabilities := test.expectedCapabilities[common.AgentContainerName(container.Name)]
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

func TestSortAndUnique(t *testing.T) {
	tests := []struct {
		name string
		in   []corev1.Capability
		want []corev1.Capability
	}{
		{
			name: "empty in",
			in:   nil,
			want: nil,
		},
		{
			name: "2 capabilities already sorted",
			in: []corev1.Capability{
				"BAR",
				"FOO",
			},
			want: []corev1.Capability{
				"BAR",
				"FOO",
			},
		},
		{
			name: "2 capability not sorted",
			in: []corev1.Capability{
				"FOO",
				"BAR",
			},
			want: []corev1.Capability{
				"BAR",
				"FOO",
			},
		},
		{
			name: "Input not unique",
			in: []corev1.Capability{
				"BAR",
				"BAR",
				"FOO",
				"BAR",
			},
			want: []corev1.Capability{
				"BAR",
				"FOO",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SortAndUnique(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SortAndUnique() = %v, want %v", got, tt.want)
			}
		})
	}
}
