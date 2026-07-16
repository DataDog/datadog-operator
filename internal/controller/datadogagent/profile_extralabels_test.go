// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// TestSetProfileSpec_CommonLabels verifies that spec.global.commonLabels from the
// base DDAI are preserved when setProfileSpec replaces the DDAI spec with a
// non-default profile's Config. Without this, label-enforcing admission policies
// (e.g. Kyverno) would reject the profile DaemonSet even when the parent DDA
// sets spec.global.commonLabels.
func TestSetProfileSpec_CommonLabels(t *testing.T) {
	profileAffinity := &v1alpha1.ProfileAffinity{
		ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
			{
				Key:      "test",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"foo"},
			},
		},
	}

	tests := []struct {
		name            string
		baseCommonLabels map[string]string
		profileConfig   *v2alpha1.DatadogAgentSpec
		wantCommonLabels map[string]string
	}{
		{
			name: "base commonLabels are preserved when profile config has no Global",
			baseCommonLabels: map[string]string{
				"team":        "platform",
				"cost-center": "ops",
			},
			profileConfig: &v2alpha1.DatadogAgentSpec{},
			wantCommonLabels: map[string]string{
				"team":        "platform",
				"cost-center": "ops",
			},
		},
		{
			name: "base commonLabels are preserved when profile config has empty Global",
			baseCommonLabels: map[string]string{
				"team": "platform",
			},
			profileConfig: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{},
			},
			wantCommonLabels: map[string]string{
				"team": "platform",
			},
		},
		{
			name: "profile config commonLabels win on key conflict; base fills missing keys",
			baseCommonLabels: map[string]string{
				"team": "base-team",
				"env":  "prod",
			},
			profileConfig: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					CommonLabels: map[string]string{
						"team": "profile-team", // conflicts — profile wins
					},
				},
			},
			wantCommonLabels: map[string]string{
				"team": "profile-team", // profile wins
				"env":  "prod",         // base fills missing key
			},
		},
		{
			name:            "no commonLabels in base and profile — nothing set",
			baseCommonLabels: nil,
			profileConfig:   &v2alpha1.DatadogAgentSpec{},
			wantCommonLabels: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ddai := v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						CommonLabels: tt.baseCommonLabels,
					},
				},
			}

			profile := v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-profile",
					Namespace: "bar",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: profileAffinity,
					Config:          tt.profileConfig,
				},
			}

			setProfileSpec(&ddai, &profile)

			if tt.wantCommonLabels == nil {
				if ddai.Spec.Global != nil {
					assert.Nil(t, ddai.Spec.Global.CommonLabels)
				}
			} else {
				assert.NotNil(t, ddai.Spec.Global)
				assert.Equal(t, tt.wantCommonLabels, ddai.Spec.Global.CommonLabels)
			}

			// Sanity: profile label must still be set on the node agent override.
			assert.Equal(t, ptr.To("foo-profile-agent"), ddai.Spec.Override[v2alpha1.NodeAgentComponentName].Name)
		})
	}
}
