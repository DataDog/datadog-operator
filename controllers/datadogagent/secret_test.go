// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package datadogagent

import (
	"os"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/testutils"
)

// Test_newAgentSecret tests that the credentials are stored in the secret in an expected hierarchy
func Test_newAgentSecret(t *testing.T) {
	type fields struct {
		APIKey string
		appKey string
		token  string
	}
	tests := []struct {
		name       string
		fields     fields
		wantAPIKey string
		wantAppKey string
		wantToken  string
	}{
		{
			name: "API key, App key, and token are set",
			fields: fields{
				APIKey: "adflkajdflkjalkcmlkdjacsf",
				appKey: "sgfggtdhfghfghfghfgbdfdgs",
				token:  "iamamoderatelylongtoken",
			},
			wantAPIKey: "adflkajdflkjalkcmlkdjacsf",
			wantAppKey: "sgfggtdhfghfghfghfgbdfdgs",
			wantToken:  "iamamoderatelylongtoken",
		},
		{
			name: "API and App keys are empty, token is set",
			fields: fields{
				APIKey: "",
				appKey: "",
				token:  "iamamoderatelylongtoken",
			},
			wantAPIKey: "",
			wantAppKey: "",
			wantToken:  "iamamoderatelylongtoken",
		},
		{
			name: "API and App keys are set, token is empty",
			fields: fields{
				APIKey: "adflkajdflkjalkcmlkdjacsf",
				appKey: "sgfggtdhfghfghfghfgbdfdgs",
				token:  "",
			},
			wantAPIKey: "adflkajdflkjalkcmlkdjacsf",
			wantAppKey: "sgfggtdhfghfghfghfgbdfdgs",
			wantToken:  "<GENERATED>", // indicates token is randomly generated
		},
		{
			name: "API key, App key and token use secret backend",
			fields: fields{
				APIKey: "ENC[api_key]",
				appKey: "ENC[app_key]",
				token:  "ENC[token]",
			},
			wantAPIKey: "ENC[api_key]",
			wantAppKey: "ENC[app_key]",
			wantToken:  "ENC[token]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &testutils.NewDatadogAgentOptions{
				APIKey: tt.fields.APIKey,
				AppKey: tt.fields.appKey,
				Token:  tt.fields.token,
			}

			dda := testutils.NewDatadogAgent("default", "test", "datadog/agent:7.24.1", options)
			// Generate token if needed
			dso := datadoghqv1alpha1.DefaultDatadogAgent(dda)
			dda.Status = *dso

			result := newAgentSecret("foo", dda)

			if val, ok := result.Data[datadoghqv1alpha1.DefaultAPIKeyKey]; ok {
				if string(val) != tt.wantAPIKey {
					t.Errorf("newAgentSecret() API key = %v, want %v", string(result.Data[datadoghqv1alpha1.DefaultAPIKeyKey]), tt.wantAPIKey)
				}
			} else {
				if tt.wantAPIKey != "" {
					t.Errorf("newAgentSecret() API key is empty but want %v", tt.wantAPIKey)
				}
			}

			if val, ok := result.Data[datadoghqv1alpha1.DefaultAPPKeyKey]; ok {
				if string(val) != tt.wantAppKey {
					t.Errorf("newAgentSecret() App key = %v, want %v", string(result.Data[datadoghqv1alpha1.DefaultAPPKeyKey]), tt.wantAPIKey)
				}
			} else {
				if tt.wantAppKey != "" {
					t.Errorf("newAgentSecret() App key is empty but want %v", tt.wantAppKey)
				}
			}

			if val, ok := result.Data[datadoghqv1alpha1.DefaultTokenKey]; ok {
				if string(val) != tt.wantToken && tt.wantToken != "<GENERATED>" {
					t.Errorf("newAgentSecret() token key = %v, want %v", string(result.Data[datadoghqv1alpha1.DefaultTokenKey]), tt.wantAPIKey)
				}
			} else {
				if tt.wantToken != "" {
					t.Errorf("newAgentSecret() token key is empty but want %v", tt.wantToken)
				}
			}
		})
	}
}

func Test_needAgentSecret(t *testing.T) {
	type fields struct {
		APIKey string
		appKey string
		token  string
	}
	tests := []struct {
		name           string
		fields         fields
		nilCredentials bool
		want           bool
	}{
		{
			name: "API key, App key, and token are set",
			fields: fields{
				APIKey: "adflkajdflkjalkcmlkdjacsf",
				appKey: "sgfggtdhfghfghfghfgbdfdgs",
				token:  "iamamoderatelylongtoken",
			},
			want: true,
		},
		{
			name: "API key, App key, and token use secret backend",
			fields: fields{
				APIKey: "ENC[api_key]",
				appKey: "ENC[app_key]",
				token:  "ENC[token]",
			},
			want: true,
		},
		{
			name:           "Nil credentials",
			nilCredentials: true,
			fields: fields{
				APIKey: "",
				appKey: "",
				token:  "",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &testutils.NewDatadogAgentOptions{
				APIKey: tt.fields.APIKey,
				AppKey: tt.fields.appKey,
				Token:  tt.fields.token,
			}

			dda := testutils.NewDatadogAgent("default", "test", "datadog/agent:7.24.1", options)

			// Generate token if needed
			dso := datadoghqv1alpha1.DefaultDatadogAgent(dda)
			dda.Status = *dso

			// For the case to check if Credetials are nil; need this because NewDatadogAgent always defines credentials
			if tt.nilCredentials {
				dda.Spec.Credentials = nil
			}

			result := needAgentSecret(dda)
			if tt.want != result {
				t.Errorf("needAgentSecret() result is %v but want %v", result, tt.want)
			}
		})
	}
}

func Test_newExternalMetricsSecret(t *testing.T) {
	name := "test-external-metrics"
	ns := "default"
	dda := testutils.NewDatadogAgent(ns, name, "datadog/agent:7.24.1", &testutils.NewDatadogAgentOptions{})
	dda.Spec.ClusterAgent.Config.ExternalMetrics = &datadoghqv1alpha1.ExternalMetricsConfig{
		Enabled: apiutils.NewBoolPointer(true),
		Credentials: &datadoghqv1alpha1.DatadogCredentials{
			APIKey: "adflkajdflkjalkcmlkdjacsf",
			AppKey: "sgfggtdhfghfghfghfgbdfdgs",
		},
	}
	result := newExternalMetricsSecret(name, dda)

	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, "")
	wantSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Labels:      labels,
			Annotations: map[string]string{},
		},
		Type: corev1.SecretTypeOpaque,
		Data: getKeysFromCredentials(dda.Spec.ClusterAgent.Config.ExternalMetrics.Credentials),
	}

	if !reflect.DeepEqual(result, wantSecret) {
		t.Errorf("newExternalMetricsSecret() result is %v but want %v", result, wantSecret)
	}
}

func Test_needExternalMetricsSecret(t *testing.T) {
	tests := []struct {
		name             string
		clusterAgentSpec datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec
		want             bool
	}{
		{
			name: "cluster agent is not enabled",
			clusterAgentSpec: datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(false),
			},
			want: false,
		},
		{
			name: "cluster agent config is nil",
			clusterAgentSpec: datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Config:  nil,
			},
			want: false,
		},
		{
			name: "external metrics config is nil",
			clusterAgentSpec: datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Config: &datadoghqv1alpha1.ClusterAgentConfig{
					ExternalMetrics: nil,
				},
			},
			want: false,
		},
		{
			name: "external metrics config is not enabled",
			clusterAgentSpec: datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Config: &datadoghqv1alpha1.ClusterAgentConfig{
					ExternalMetrics: &datadoghqv1alpha1.ExternalMetricsConfig{
						Enabled: apiutils.NewBoolPointer(false),
					},
				},
			},
			want: false,
		},
		{
			name: "external metrics config credentials is nil",
			clusterAgentSpec: datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Config: &datadoghqv1alpha1.ClusterAgentConfig{
					ExternalMetrics: &datadoghqv1alpha1.ExternalMetricsConfig{
						Enabled:     apiutils.NewBoolPointer(true),
						Credentials: nil,
					},
				},
			},
			want: false,
		},
		{
			name: "external metrics config credentials API and app keys are empty",
			clusterAgentSpec: datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Config: &datadoghqv1alpha1.ClusterAgentConfig{
					ExternalMetrics: &datadoghqv1alpha1.ExternalMetricsConfig{
						Enabled: apiutils.NewBoolPointer(true),
						Credentials: &datadoghqv1alpha1.DatadogCredentials{
							APIKey: "",
							AppKey: "",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "external metrics config credentials API and app keys are set",
			clusterAgentSpec: datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Config: &datadoghqv1alpha1.ClusterAgentConfig{
					ExternalMetrics: &datadoghqv1alpha1.ExternalMetricsConfig{
						Enabled: apiutils.NewBoolPointer(true),
						Credentials: &datadoghqv1alpha1.DatadogCredentials{
							APIKey: "adflkajdflkjalkcmlkdjacsf",
							AppKey: "sgfggtdhfghfghfghfgbdfdgs",
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := "test-external-metrics"
			ns := "default"
			dda := testutils.NewDatadogAgent(ns, name, "datadog/agent:7.24.1", &testutils.NewDatadogAgentOptions{})
			dda.Spec.ClusterAgent = tt.clusterAgentSpec
			result := needExternalMetricsSecret(dda)
			if result != tt.want {
				t.Errorf("needExternalMetricsSecret() result is %v but want %v", result, tt.want)
			}
		})
	}
}

func Test_getKeysFromCredentials(t *testing.T) {
	tests := []struct {
		name     string
		APIKey   string
		appKey   string
		wantFunc func() map[string][]byte
	}{
		{
			name:     "API and app keys are empty",
			APIKey:   "",
			appKey:   "",
			wantFunc: func() map[string][]byte { return map[string][]byte{} },
		},
		{
			name:   "API and app keys are formatted for the secret backend",
			APIKey: "ENC[api_key]",
			appKey: "ENC[app_key]",
			wantFunc: func() map[string][]byte {
				wantMap := make(map[string][]byte)
				wantMap[datadoghqv1alpha1.DefaultAPIKeyKey] = []byte("ENC[api_key]")
				wantMap[datadoghqv1alpha1.DefaultAPPKeyKey] = []byte("ENC[app_key]")
				return wantMap
			},
		},
		{
			name:   "API and app keys are set",
			APIKey: "adflkajdflkjalkcmlkdjacsf",
			appKey: "sgfggtdhfghfghfghfgbdfdgs",
			wantFunc: func() map[string][]byte {
				wantMap := make(map[string][]byte)
				wantMap[datadoghqv1alpha1.DefaultAPIKeyKey] = []byte("adflkajdflkjalkcmlkdjacsf")
				wantMap[datadoghqv1alpha1.DefaultAPPKeyKey] = []byte("sgfggtdhfghfghfghfgbdfdgs")
				return wantMap
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := &datadoghqv1alpha1.DatadogCredentials{
				APIKey: tt.APIKey,
				AppKey: tt.appKey,
			}
			result := getKeysFromCredentials(creds)
			wantMap := tt.wantFunc()
			if !reflect.DeepEqual(result, wantMap) {
				t.Errorf("getKeysFromCredentials() result is %v but want %v", result, wantMap)
			}
		})
	}
}

func Test_checkAPIKeySufficiency(t *testing.T) {
	apiKeyValue := "adflkajdflkjalkcmlkdjacsf"

	tests := []struct {
		name        string
		credentials *datadoghqv1alpha1.DatadogCredentials
		envVarName  string
		want        bool
	}{
		{
			name: "APISecret is used",
			credentials: &datadoghqv1alpha1.DatadogCredentials{
				APISecret: &commonv1.SecretConfig{
					SecretName: "test-secret",
					KeyName:    "api_key",
				},
			},
			want: true,
		},
		{
			name: "APIKeyExistingSecret is used",
			credentials: &datadoghqv1alpha1.DatadogCredentials{
				APIKeyExistingSecret: "test-secret",
			},
			want: true,
		},
		{
			name: "secret backend is used",
			credentials: &datadoghqv1alpha1.DatadogCredentials{
				APIKey: "ENC[api_key]",
			},
			want: false,
		},
		{
			name:        "envvar is used",
			credentials: &datadoghqv1alpha1.DatadogCredentials{},
			envVarName:  "DD_API_KEY",
			want:        true,
		},
		{
			name: "credential is set",
			credentials: &datadoghqv1alpha1.DatadogCredentials{
				APIKey: apiKeyValue,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVarName != "" {
				os.Setenv(tt.envVarName, apiKeyValue)
			}
			result := checkAPIKeySufficiency(tt.credentials, tt.envVarName)
			if result != tt.want {
				t.Errorf("checkAPIKeySufficiency() result is %v but want %v", result, tt.want)
			}
			if tt.envVarName != "" {
				os.Unsetenv(tt.envVarName)
			}
		})
	}
}

func Test_checkAppKeySufficiency(t *testing.T) {
	appKeyValue := "sgfggtdhfghfghfghfgbdfdgs"

	tests := []struct {
		name        string
		credentials *datadoghqv1alpha1.DatadogCredentials
		envVarName  string
		want        bool
	}{
		{
			name: "APPSecret is used",
			credentials: &datadoghqv1alpha1.DatadogCredentials{
				APPSecret: &commonv1.SecretConfig{
					SecretName: "test-secret",
					KeyName:    "app_key",
				},
			},
			want: true,
		},
		{
			name: "AppKeyExistingSecret is used",
			credentials: &datadoghqv1alpha1.DatadogCredentials{
				AppKeyExistingSecret: "test-secret",
			},
			want: true,
		},
		{
			name: "secret backend is used",
			credentials: &datadoghqv1alpha1.DatadogCredentials{
				AppKey: "ENC[api_key]",
			},
			want: false,
		},
		{
			name:        "envvar is used",
			credentials: &datadoghqv1alpha1.DatadogCredentials{},
			envVarName:  "DD_APP_KEY",
			want:        true,
		},
		{
			name: "credential is set",
			credentials: &datadoghqv1alpha1.DatadogCredentials{
				AppKey: appKeyValue,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVarName != "" {
				os.Setenv(tt.envVarName, appKeyValue)
			}
			result := checkAppKeySufficiency(tt.credentials, tt.envVarName)
			if result != tt.want {
				t.Errorf("checkAppKeySufficiency() result is %v but want %v", result, tt.want)
			}
			if tt.envVarName != "" {
				os.Unsetenv(tt.envVarName)
			}
		})
	}
}
