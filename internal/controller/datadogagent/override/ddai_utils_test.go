// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/stretchr/testify/assert"
)

func TestShouldAddDCATokenChecksumAnnotation(t *testing.T) {
	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want bool
	}{
		{
			name: "should add checksum annotation when token is set as a literal value",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterAgentToken: apiutils.NewStringPointer("token"),
					},
				},
			},
			want: true,
		},
		{
			name: "should not add checksum annotation when token is set as a secret",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterAgentTokenSecret: &v2alpha1.SecretConfig{SecretName: "secret", KeyName: "key"},
					},
				},
			},
			want: false,
		},
		{
			name: "should not add checksum annotation when both token and secret are set",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterAgentToken: apiutils.NewStringPointer("token"),
						ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
							SecretName: "secret",
							KeyName:    "key",
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAddDCATokenChecksumAnnotation(tt.dda); got != tt.want {
				t.Errorf("shouldAddDCATokenChecksumAnnotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetOverrideFromDDA(t *testing.T) {
	const (
		tokenValue            = "test-token"
		existingLabel         = "existing-label"
		existingAnnotation    = "existing-annotation"
		existingDCAAnnotation = "existing-dca-annotation"
		existingValue         = "value"
		tokenSecretName       = "token-secret"
		tokenSecretKey        = "key"
	)

	tokenHash, _ := comparison.GenerateMD5ForSpec(map[string]string{common.DefaultTokenKey: tokenValue})
	dcaTokenChecksumAnnotationKey := global.GetDCATokenChecksumAnnotationKey()

	tests := []struct {
		name         string
		dda          *v2alpha1.DatadogAgent
		wantDDAISpec *v2alpha1.DatadogAgentSpec
	}{
		{
			name: "token set as literal value",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterAgentToken: apiutils.NewStringPointer(tokenValue),
					},
				},
			},
			wantDDAISpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					ClusterAgentToken: apiutils.NewStringPointer(tokenValue),
				},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: "",
						},
						Annotations: map[string]string{
							dcaTokenChecksumAnnotationKey: tokenHash,
						},
					},
					v2alpha1.ClusterAgentComponentName: {
						Annotations: map[string]string{
							dcaTokenChecksumAnnotationKey: tokenHash,
						},
					},
					v2alpha1.ClusterChecksRunnerComponentName: {
						Annotations: map[string]string{
							dcaTokenChecksumAnnotationKey: tokenHash,
						},
					},
				},
			},
		},
		{
			name: "token set via secret",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
							SecretName: tokenSecretName,
						},
					},
				},
			},
			wantDDAISpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
						SecretName: tokenSecretName,
					},
				},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: "",
						},
					},
				},
			},
		},
		{
			name: "token set as literal value, with existing overrides",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterAgentToken: apiutils.NewStringPointer(tokenValue),
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Labels:      map[string]string{existingLabel: existingValue},
							Annotations: map[string]string{existingAnnotation: existingValue},
						},
						v2alpha1.ClusterAgentComponentName: {
							Annotations: map[string]string{existingDCAAnnotation: existingValue},
						},
					},
				},
			},
			wantDDAISpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					ClusterAgentToken: apiutils.NewStringPointer(tokenValue),
				},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Labels: map[string]string{
							existingLabel: existingValue,
							constants.MD5AgentDeploymentProviderLabelKey: "",
						},
						Annotations: map[string]string{
							existingAnnotation:            existingValue,
							dcaTokenChecksumAnnotationKey: tokenHash,
						},
					},
					v2alpha1.ClusterAgentComponentName: {
						Annotations: map[string]string{
							existingDCAAnnotation:         existingValue,
							dcaTokenChecksumAnnotationKey: tokenHash,
						},
					},
					v2alpha1.ClusterChecksRunnerComponentName: {
						Annotations: map[string]string{
							dcaTokenChecksumAnnotationKey: tokenHash,
						},
					},
				},
			},
		},
		{
			name: "token set as literal value and secret is set, with existing overrides",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterAgentToken: apiutils.NewStringPointer(tokenValue),
						ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
							SecretName: tokenSecretName,
							KeyName:    tokenSecretKey,
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Labels: map[string]string{existingLabel: existingValue},
						},
					},
				},
			},
			wantDDAISpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					ClusterAgentToken: apiutils.NewStringPointer(tokenValue),
					ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
						SecretName: tokenSecretName,
						KeyName:    tokenSecretKey,
					},
				},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Labels: map[string]string{
							existingLabel: existingValue,
							constants.MD5AgentDeploymentProviderLabelKey: "",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpec := tt.dda.Spec.DeepCopy()
			SetOverrideFromDDA(tt.dda, gotSpec)
			assert.Equal(t, tt.wantDDAISpec, gotSpec)
		})
	}
}
