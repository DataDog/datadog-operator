// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"reflect"
	"testing"

	"k8s.io/utils/ptr"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
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
					Enabled: ptr.To(true),
				},
			},
		},
	}
	validFeaturesNoOverride := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				GPU: &v2alpha1.GPUFeatureConfig{
					Enabled:        ptr.To(true),
					PrivilegedMode: ptr.To(true),
				},
			},
		},
	}
	invalidFeatures := &DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				NPM: &v2alpha1.NPMFeatureConfig{
					Enabled: ptr.To(true),
				},
			},
		},
	}
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
			name:    "invalid component override",
			spec:    invalidComponentOverride,
			wantErr: "component node selector override is not supported",
		},
		{
			name:    "invalid container override",
			spec:    invalidContainerOverride,
			wantErr: "container command override is not supported",
		},
		{
			name: "missing override is valid",
			spec: missingOverride,
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
		{
			name: "gpu feature override",
			spec: validGPUFeature,
		},
		{
			name: "valid dap with features only, no override",
			spec: validFeaturesNoOverride,
		},
		{
			name:    "dap with unsupported feature",
			spec:    invalidFeatures,
			wantErr: "npm override is not supported",
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

func TestValidateDatadogAgentProfileFeaturesAllowlist(t *testing.T) {
	allowedFeatureFields := map[string]struct{}{
		"APM": {},
		"GPU": {},
	}

	featuresType := reflect.TypeOf(v2alpha1.DatadogFeatures{})
	for i := 0; i < featuresType.NumField(); i++ {
		field := featuresType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			if !assert.Equal(t, reflect.Ptr, field.Type.Kind(), "DatadogFeatures fields should be pointer types") {
				return
			}

			features := &v2alpha1.DatadogFeatures{}
			reflect.ValueOf(features).Elem().FieldByName(field.Name).Set(reflect.New(field.Type.Elem()))

			spec := &DatadogAgentProfileSpec{
				ProfileAffinity: validProfileAffinity(),
				Config: &v2alpha1.DatadogAgentSpec{
					Features: features,
				},
			}

			result := ValidateDatadogAgentProfileSpec(spec)
			if _, ok := allowedFeatureFields[field.Name]; ok {
				assert.NoError(t, result)
			} else {
				if assert.Error(t, result) {
					assert.Contains(t, result.Error(), "override is not supported")
				}
			}
		})
	}
}

func TestValidateDatadogAgentProfileComponentOverrideAllowlist(t *testing.T) {
	allowedComponentOverrideFields := map[string]struct{}{
		"Containers":        {},
		"PriorityClassName": {},
		"RuntimeClassName":  {},
		"UpdateStrategy":    {},
		"Labels":            {},
	}

	overrideType := reflect.TypeOf(v2alpha1.DatadogAgentComponentOverride{})
	for i := 0; i < overrideType.NumField(); i++ {
		field := overrideType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			override := &v2alpha1.DatadogAgentComponentOverride{}
			setConfiguredField(t, reflect.ValueOf(override).Elem().FieldByName(field.Name))

			spec := &DatadogAgentProfileSpec{
				ProfileAffinity: validProfileAffinity(),
				Config: &v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: override,
					},
				},
			}

			result := ValidateDatadogAgentProfileSpec(spec)
			if _, ok := allowedComponentOverrideFields[field.Name]; ok {
				assert.NoError(t, result)
			} else {
				if assert.Error(t, result) {
					assert.Contains(t, result.Error(), "override is not supported")
				}
			}
		})
	}
}

func TestValidateDatadogAgentProfileContainerOverrideAllowlist(t *testing.T) {
	allowedContainerOverrideFields := map[string]struct{}{
		"Resources": {},
		"Env":       {},
	}

	containerType := reflect.TypeOf(v2alpha1.DatadogAgentGenericContainer{})
	for i := 0; i < containerType.NumField(); i++ {
		field := containerType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			containerOverride := &v2alpha1.DatadogAgentGenericContainer{}
			setConfiguredField(t, reflect.ValueOf(containerOverride).Elem().FieldByName(field.Name))

			spec := &DatadogAgentProfileSpec{
				ProfileAffinity: validProfileAffinity(),
				Config: &v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Containers: map[common.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								common.CoreAgentContainerName: containerOverride,
							},
						},
					},
				},
			}

			result := ValidateDatadogAgentProfileSpec(spec)
			if _, ok := allowedContainerOverrideFields[field.Name]; ok {
				assert.NoError(t, result)
			} else {
				if assert.Error(t, result) {
					assert.Contains(t, result.Error(), "override is not supported")
				}
			}
		})
	}
}

func validProfileAffinity() *ProfileAffinity {
	return &ProfileAffinity{
		ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
			{
				Key:      "foo",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"bar"},
			},
		},
	}
}

func setConfiguredField(t *testing.T, fieldValue reflect.Value) {
	t.Helper()

	switch fieldValue.Kind() {
	case reflect.Map:
		fieldValue.Set(reflect.MakeMap(fieldValue.Type()))
	case reflect.Ptr:
		fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
	case reflect.Slice:
		fieldValue.Set(reflect.MakeSlice(fieldValue.Type(), 0, 0))
	default:
		t.Fatalf("unsupported field kind %q", fieldValue.Kind())
	}
}
