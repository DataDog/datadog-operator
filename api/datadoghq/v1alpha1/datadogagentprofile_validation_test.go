// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/api/utils"
)

func TestIsValidDatadogAgentProfile(t *testing.T) {
	basicProfileAffinity := &ProfileAffinity{
		ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
			{
				Key:      "foo",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"bar"},
			},
		},
	}
	basicNodeAgentOverride := map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.NodeAgentComponentName: {
			Containers: map[common.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
				common.CoreAgentContainerName: {
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
						},
					},
				},
			},
		},
	}
	valid := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Override: basicNodeAgentOverride,
		},
	}
	validResourceOverrideInOneContainerOnly := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Containers: map[common.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
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
	invalidComponentOverride := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					NodeSelector: map[string]string{
						"foo": "bar",
					},
					Containers: map[common.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
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
	invalidContainerOverride := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Containers: map[common.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
						common.CoreAgentContainerName: {
							Resources: &corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
								},
							},
							Command: []string{"foo", "bar"},
						},
						common.TraceAgentContainerName: {},
					},
				},
			},
		},
	}
	missingOverride := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config:          &v2alpha1.DatadogAgentSpec{},
	}
	missingConfig := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
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
	validGPUFeature := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				GPU: &v2alpha1.GPUFeatureConfig{
					Enabled: utils.NewBoolPointer(true),
				},
			},
		},
	}
	validFeaturesNoOverride := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				GPU: &v2alpha1.GPUFeatureConfig{
					Enabled:        utils.NewBoolPointer(true),
					PrivilegedMode: utils.NewBoolPointer(true),
				},
			},
		},
	}
	invalidFeatures := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				NPM: &v2alpha1.NPMFeatureConfig{
					Enabled: utils.NewBoolPointer(true),
				},
			},
		},
	}
	invalidFeaturesNoDDAI := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				GPU: &v2alpha1.GPUFeatureConfig{
					Enabled: utils.NewBoolPointer(true),
				},
			},
		},
	}

	testCases := []struct {
		name                        string
		spec                        *DatadogAgentProfileSpec
		datadogAgentInternalEnabled bool
		wantErr                     string
	}{
		{
			name:                        "valid dap",
			spec:                        valid,
			datadogAgentInternalEnabled: true,
		},
		{
			name:                        "valid dap, resources specified in one container only",
			spec:                        validResourceOverrideInOneContainerOnly,
			datadogAgentInternalEnabled: true,
		},
		{
			name:                        "invalid component override",
			spec:                        invalidComponentOverride,
			datadogAgentInternalEnabled: true,
			wantErr:                     "component node selector override is not supported",
		},
		{
			name:                        "invalid container override",
			spec:                        invalidContainerOverride,
			datadogAgentInternalEnabled: true,
			wantErr:                     "container command override is not supported",
		},
		{
			name:                        "missing override when ddai disabled",
			spec:                        missingOverride,
			datadogAgentInternalEnabled: false,
			wantErr:                     "config override must be defined",
		},
		{
			name:                        "missing config",
			spec:                        missingConfig,
			datadogAgentInternalEnabled: true,
			wantErr:                     "config must be defined",
		},
		{
			name:                        "missing node selector requirement",
			spec:                        missingNSR,
			datadogAgentInternalEnabled: true,
			wantErr:                     "profileNodeAffinity must have at least 1 requirement",
		},
		{
			name:                        "missing profile node affinity",
			spec:                        missingNodeAffinity,
			datadogAgentInternalEnabled: true,
			wantErr:                     "profileNodeAffinity must be defined",
		},
		{
			name:                        "missing profile affinity",
			spec:                        missingProfileAffinity,
			datadogAgentInternalEnabled: true,
			wantErr:                     "profileAffinity must be defined",
		},
		{
			name:                        "gpu feature override",
			spec:                        validGPUFeature,
			datadogAgentInternalEnabled: true,
		},
		{
			name:                        "valid dap with features only when ddai enabled",
			spec:                        validFeaturesNoOverride,
			datadogAgentInternalEnabled: true,
		},
		{
			name:                        "dap with unsupported feature when ddai enabled",
			spec:                        invalidFeatures,
			datadogAgentInternalEnabled: true,
			wantErr:                     "npm override is not supported",
		},
		{
			name:                        "features not supported when ddai disabled",
			spec:                        invalidFeaturesNoDDAI,
			datadogAgentInternalEnabled: false,
			wantErr:                     "the 'features' field is only supported when DatadogAgentInternal is enabled",
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := ValidateDatadogAgentProfileSpec(test.spec, test.datadogAgentInternalEnabled)
			if test.wantErr != "" {
				assert.EqualError(t, result, test.wantErr)
			} else {
				assert.NoError(t, result)
			}
		})
	}
}
