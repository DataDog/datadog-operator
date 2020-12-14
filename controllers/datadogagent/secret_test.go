// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package datadogagent

import (
	"os"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/testutils"
	"github.com/DataDog/datadog-operator/pkg/config"
)

// TestNewAgentSecret that the credentials are stored in the secret in an expected hierarchy
func TestNewAgentSecret(t *testing.T) {
	options := &testutils.NewDatadogAgentOptions{}

	type fields struct {
		DatadogAgentAPIKey string
		DatadogAgentAppKey string
		APIKeyEnvVar       string
		AppKeyEnvVar       string
	}
	tests := []struct {
		name       string
		fields     fields
		wantAPIKey string
		wantAppKey string
	}{
		{
			name: "API and App keys are set in the DatadogAgent",
			fields: fields{
				DatadogAgentAPIKey: "adflkajdflkjalkcmlkdjacsf",
				DatadogAgentAppKey: "sgfggtdhfghfghfghfgbdfdgs",
				APIKeyEnvVar:       "",
				AppKeyEnvVar:       "",
			},
			wantAPIKey: "adflkajdflkjalkcmlkdjacsf",
			wantAppKey: "sgfggtdhfghfghfghfgbdfdgs",
		},
		{
			name: "API and App keys are set in env vars",
			fields: fields{
				DatadogAgentAPIKey: "",
				DatadogAgentAppKey: "",
				APIKeyEnvVar:       "zzkjhcoiksncoiquwnzidufnq",
				AppKeyEnvVar:       "podpogoasdkgpoqiweposdfop",
			},
			wantAPIKey: "zzkjhcoiksncoiquwnzidufnq",
			wantAppKey: "podpogoasdkgpoqiweposdfop",
		},
		{
			name: "API and App keys are set in both the DatadogAgent and env vars",
			fields: fields{
				DatadogAgentAPIKey: "adflkajdflkjalkcmlkdjacsf",
				DatadogAgentAppKey: "sgfggtdhfghfghfghfgbdfdgs",
				APIKeyEnvVar:       "zzkjhcoiksncoiquwnzidufnq",
				AppKeyEnvVar:       "podpogoasdkgpoqiweposdfop",
			},
			wantAPIKey: "adflkajdflkjalkcmlkdjacsf",
			wantAppKey: "sgfggtdhfghfghfghfgbdfdgs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(config.DDAPIKeyEnvVar, tt.fields.APIKeyEnvVar)
			os.Setenv(config.DDAppKeyEnvVar, tt.fields.AppKeyEnvVar)

			options.APIKey = tt.fields.DatadogAgentAPIKey
			options.AppKey = tt.fields.DatadogAgentAppKey
			dda := testutils.NewDatadogAgent("default", "test", "datadog/agent:7.24.1", options)

			result := newAgentSecret(dda)
			if string(result.Data[datadoghqv1alpha1.DefaultAPIKeyKey]) != tt.wantAPIKey {
				t.Errorf("newAgentSecret() API key = %v, want %v", string(result.Data[datadoghqv1alpha1.DefaultAPIKeyKey]), tt.wantAPIKey)
			}
			if string(result.Data[datadoghqv1alpha1.DefaultAPPKeyKey]) != tt.wantAppKey {
				t.Errorf("newAgentSecret() App key = %v, want %v", string(result.Data[datadoghqv1alpha1.DefaultAPPKeyKey]), tt.wantAppKey)
			}
			os.Unsetenv(tt.fields.APIKeyEnvVar)
			os.Unsetenv(tt.fields.AppKeyEnvVar)
		})
	}
}
