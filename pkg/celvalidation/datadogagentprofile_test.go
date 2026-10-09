// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package celvalidation

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/utils/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestIsValidDatadogAgentProfile(t *testing.T) {
	basicProfileAffinity := &v1alpha1.ProfileAffinity{
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
	valid := &v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Override: basicNodeAgentOverride,
		},
	}
	validResourceOverrideInOneContainerOnly := &v1alpha1.DatadogAgentProfileSpec{
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
	invalidComponentOverride := &v1alpha1.DatadogAgentProfileSpec{
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
	invalidContainerOverride := &v1alpha1.DatadogAgentProfileSpec{
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
	missingOverride := &v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config:          &v2alpha1.DatadogAgentSpec{},
	}
	missingConfig := &v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
	}
	missingNSR := &v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: &v1alpha1.ProfileAffinity{
			ProfileNodeAffinity: []corev1.NodeSelectorRequirement{},
		},
	}
	missingNodeAffinity := &v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: &v1alpha1.ProfileAffinity{},
	}
	missingProfileAffinity := &v1alpha1.DatadogAgentProfileSpec{}
	validGPUFeature := &v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				GPU: &v2alpha1.GPUFeatureConfig{
					Enabled: ptr.To(true),
				},
			},
		},
	}
	validFeaturesNoOverride := &v1alpha1.DatadogAgentProfileSpec{
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
	validAPMFeature := &v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				APM: &v2alpha1.APMFeatureConfig{
					Enabled: ptr.To(true),
					SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
						Enabled:           ptr.To(true),
						EnabledNamespaces: []string{"gpu"},
					},
				},
			},
		},
	}
	invalidFeatures := &v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				NPM: &v2alpha1.NPMFeatureConfig{
					Enabled: ptr.To(true),
				},
			},
		},
	}
	invalidDataPlaneFeature := &v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: basicProfileAffinity,
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				DataPlane: &v2alpha1.DataPlaneFeatureConfig{},
			},
		},
	}
	testCases := []struct {
		name    string
		spec    *v1alpha1.DatadogAgentProfileSpec
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
			name: "missing node selector requirement",
			spec: missingNSR,
			// This fixture omits config too, so both rules fail. The Go
			// validator returned on the first; every failure is reported now,
			// so that the operator and the API server say the same thing.
			wantErr: "profileNodeAffinity must have at least 1 requirement\nconfig must be defined",
		},
		{
			name: "missing profile node affinity",
			spec: missingNodeAffinity,
			// One rule covers an absent and an empty profileNodeAffinity. The
			// field is omitempty, so an empty list reaches the API server as
			// `profileNodeAffinity: []` but reaches the operator as an absent
			// field; a rule that told them apart would disagree with itself
			// between the two evaluators.
			wantErr: "profileNodeAffinity must have at least 1 requirement\nconfig must be defined",
		},
		{
			name:    "missing profile affinity",
			spec:    missingProfileAffinity,
			wantErr: "profileAffinity must be defined\nconfig must be defined",
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
			name: "apm feature override",
			spec: validAPMFeature,
		},
		{
			name:    "dap with unsupported feature",
			spec:    invalidFeatures,
			wantErr: "npm override is not supported",
		},
		{
			name:    "dap with unsupported data plane feature",
			spec:    invalidDataPlaneFeature,
			wantErr: "dataPlane override is not supported",
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := validateSpec(t, test.spec)
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

			spec := &v1alpha1.DatadogAgentProfileSpec{
				ProfileAffinity: validProfileAffinity(),
				Config: &v2alpha1.DatadogAgentSpec{
					Features: features,
				},
			}

			result := validateSpec(t, spec)
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
		"Volumes":           {},
	}

	overrideType := reflect.TypeOf(v2alpha1.DatadogAgentComponentOverride{})
	for i := 0; i < overrideType.NumField(); i++ {
		field := overrideType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			_, allowed := allowedComponentOverrideFields[field.Name]
			override := &v2alpha1.DatadogAgentComponentOverride{}
			setConfiguredField(t, reflect.ValueOf(override).Elem().FieldByName(field.Name), !allowed)

			spec := &v1alpha1.DatadogAgentProfileSpec{
				ProfileAffinity: validProfileAffinity(),
				Config: &v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: override,
					},
				},
			}

			result := validateSpec(t, spec)
			if allowed {
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
		"Resources":    {},
		"Env":          {},
		"VolumeMounts": {},
	}

	containerType := reflect.TypeOf(v2alpha1.DatadogAgentGenericContainer{})
	for i := 0; i < containerType.NumField(); i++ {
		field := containerType.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			_, allowed := allowedContainerOverrideFields[field.Name]
			containerOverride := &v2alpha1.DatadogAgentGenericContainer{}
			setConfiguredField(t, reflect.ValueOf(containerOverride).Elem().FieldByName(field.Name), !allowed)

			spec := &v1alpha1.DatadogAgentProfileSpec{
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

			result := validateSpec(t, spec)
			if allowed {
				assert.NoError(t, result)
			} else {
				if assert.Error(t, result) {
					assert.Contains(t, result.Error(), "override is not supported")
				}
			}
		})
	}
}

func validProfileAffinity() *v1alpha1.ProfileAffinity {
	return &v1alpha1.ProfileAffinity{
		ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
			{
				Key:      "foo",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"bar"},
			},
		},
	}
}

// setConfiguredField marks a field as set. populate gives maps and slices an
// entry: every field here is omitempty, so an empty map or slice is dropped
// before the rules see it and the field reads as unset. Allowlisted fields are
// left empty, both because an empty value is valid for them and because
// populating a typed map key (container names) with a zero value would not be.
// See TestEmptyDisallowedFieldIsAdmissionOnly.
func setConfiguredField(t *testing.T, fieldValue reflect.Value, populate bool) {
	t.Helper()

	entries := 0
	if populate {
		entries = 1
	}
	switch fieldValue.Kind() {
	case reflect.Map:
		m := reflect.MakeMap(fieldValue.Type())
		if populate {
			m.SetMapIndex(reflect.New(fieldValue.Type().Key()).Elem(), reflect.New(fieldValue.Type().Elem()).Elem())
		}
		fieldValue.Set(m)
	case reflect.Ptr:
		fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
	case reflect.Slice:
		fieldValue.Set(reflect.MakeSlice(fieldValue.Type(), entries, entries))
	default:
		t.Fatalf("unsupported field kind %q", fieldValue.Kind())
	}
}

// validateSpec wraps a bare spec so the table tests can keep their shape. The
// rules are expressed over the whole object, because that is what the API
// server evaluates them against.
func validateSpec(t *testing.T, spec *v1alpha1.DatadogAgentProfileSpec) error {
	t.Helper()
	profile := &v1alpha1.DatadogAgentProfile{Spec: *spec}
	if err := dapRules(t).ValidateObject(context.Background(), profile, profile.Namespace, profile.Name); err != nil {
		return err
	}
	// Not a CEL rule; see ValidateOverridesDefined.
	return v1alpha1.ValidateOverridesDefined(profile)
}

// TestEmptyDisallowedFieldIsAdmissionOnly pins down where the two evaluation
// points differ, and it is the one user-visible change from validating in Go.
//
// Every field in these structs is omitempty. The API server evaluates the rules
// against the request body, so `nodeSelector: {}` is present and the rule fires.
// The operator evaluates them against the stored object converted from Go, and
// that conversion drops an empty map, so the same spec reads as unset and the
// rule does not fire. The Go validator this replaced used reflect.IsNil and
// caught both.
//
// The practical effect is that an explicitly empty disallowed field is reported
// at kubectl apply and ignored at reconcile. An empty override has nothing to
// apply, so nothing is misconfigured as a result.
func TestEmptyDisallowedFieldIsAdmissionOnly(t *testing.T) {
	rules := dapRules(t)

	const message = "component node selector override is not supported"

	// What the API server sees: the field is in the request body.
	asSubmitted := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "datadoghq.com/v1alpha1",
		"kind":       "DatadogAgentProfile",
		"spec": map[string]any{
			"profileAffinity": map[string]any{
				"profileNodeAffinity": []any{
					map[string]any{"key": "app", "operator": "In", "values": []any{"dd"}},
				},
			},
			"config": map[string]any{
				"override": map[string]any{
					"nodeAgent": map[string]any{"nodeSelector": map[string]any{}},
				},
			},
		},
	}}
	failures, err := rules.Evaluate(context.Background(), asSubmitted, "", "")
	require.NoError(t, err)
	assert.Contains(t, failures, message)

	// What the operator sees: omitempty dropped the empty map.
	asStored := &v1alpha1.DatadogAgentProfile{Spec: v1alpha1.DatadogAgentProfileSpec{
		ProfileAffinity: validProfileAffinity(),
		Config: &v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {NodeSelector: map[string]string{}},
			},
		},
	}}
	failures, err = rules.Evaluate(context.Background(), asStored, "", "")
	require.NoError(t, err)
	assert.NotContains(t, failures, message)
}

// TestRulesCompile lives in rules_test.go, alongside the other rule safeguards.
