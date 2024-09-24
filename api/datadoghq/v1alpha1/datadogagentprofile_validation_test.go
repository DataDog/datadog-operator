// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestIsValidDatadogAgentProfile(t *testing.T) {
	// Test cases are missing each of the required parameters
	valid := &DatadogAgentProfileSpec{
		ProfileAffinity: &ProfileAffinity{
			ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
				{
					Key:      "foo",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"bar"},
				},
			},
		},
		Config: &Config{
			Override: map[ComponentName]*Override{
				NodeAgentComponentName: {
					Containers: map[common.AgentContainerName]*Container{
						common.CoreAgentContainerName: {
							Resources: &corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
								},
							},
						},
					},
				},
			},
		},
	}
	validResourceOverrideInOneContainerOnly := &DatadogAgentProfileSpec{
		ProfileAffinity: &ProfileAffinity{
			ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
				{
					Key:      "foo",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"bar"},
				},
			},
		},
		Config: &Config{
			Override: map[ComponentName]*Override{
				NodeAgentComponentName: {
					Containers: map[common.AgentContainerName]*Container{
						common.CoreAgentContainerName: {
							Resources: &corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
								},
							},
						},
						common.TraceAgentContainerName: {},
					},
				},
			},
		},
	}
	missingOverride := &DatadogAgentProfileSpec{
		ProfileAffinity: &ProfileAffinity{
			ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
				{
					Key:      "foo",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"bar"},
				},
			},
		},
		Config: &Config{},
	}
	missingConfig := &DatadogAgentProfileSpec{
		ProfileAffinity: &ProfileAffinity{
			ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
				{
					Key:      "foo",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"bar"},
				},
			},
		},
	}
	missingNSR := &DatadogAgentProfileSpec{
		ProfileAffinity: &ProfileAffinity{
			ProfileNodeAffinity: []corev1.NodeSelectorRequirement{},
		},
	}
	missingNodeAffinity := &DatadogAgentProfileSpec{
		ProfileAffinity: &ProfileAffinity{},
	}
	missingProfileAffinity := &DatadogAgentProfileSpec{}

	testCases := []struct {
		name    string
		spec    *DatadogAgentProfileSpec
		wantErr string
	}{
		{
			name: "valid dap",
			spec: valid,
		},
		{
			name: "valid dap, resources specified in one container only",
			spec: validResourceOverrideInOneContainerOnly,
		},
		{
			name:    "missing override",
			spec:    missingOverride,
			wantErr: "config override must be defined",
		},
		{
			name:    "missing config",
			spec:    missingConfig,
			wantErr: "config must be defined",
		},
		{
			name:    "missing node selector requirement",
			spec:    missingNSR,
			wantErr: "profileNodeAffinity must have at least 1 requirement",
		},
		{
			name:    "missing profile node affinity",
			spec:    missingNodeAffinity,
			wantErr: "profileNodeAffinity must be defined",
		},
		{
			name:    "missing profile affinity",
			spec:    missingProfileAffinity,
			wantErr: "profileAffinity must be defined",
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := ValidateDatadogAgentProfileSpec(test.spec)
			if test.wantErr != "" {
				assert.EqualError(t, result, test.wantErr)
			} else {
				assert.NoError(t, result)
			}
		})
	}
}
