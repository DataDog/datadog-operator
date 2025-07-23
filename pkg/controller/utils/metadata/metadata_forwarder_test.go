// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"os"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/config"
)

func Test_getURL(t *testing.T) {
	tests := []struct {
		name     string
		loadFunc func()
		wantURL  string
	}{
		{
			name: "default case",
			loadFunc: func() {
			},
			wantURL: "https://app.datadoghq.com/api/v1/metadata",
		},
		{
			name: "set DD_SITE",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_SITE", "datad0g.com")
			},
			wantURL: "https://app.datad0g.com/api/v1/metadata",
		},
		{
			name: "set DD_URL",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_URL", "https://app.datad0g.com")
			},
			wantURL: "https://app.datad0g.com/api/v1/metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.loadFunc()

			u := getURL()

			if u != tt.wantURL {
				t.Errorf("getURL() url = %v, want %v", u, tt.wantURL)

			}
		})
	}
}

// Test setup of API key, cluster name and URL with Operator and DDA
func Test_setup(t *testing.T) {
	fakeAPIKeyDDA := "fake_api_key_dda"
	fakeAPPKeyDDA := "fake_app_key_dda"
	fakeClusterNameDDA := "fake_cluster_name_dda"

	fakeAPIKeyOperator := "fake_api_key_operator"
	fakeClusterNameOperator := "fake_cluster_name_operator"

	tests := []struct {
		name            string
		loadFunc        func()
		dda             *v2alpha1.DatadogAgent
		wantClusterName string
		wantAPIKey      string
		wantURL         string
	}{
		{
			name: "default case, credentials set in Operator, empty DDA",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_API_KEY", fakeAPIKeyOperator)
				os.Setenv("DD_APP_KEY", fakeAPPKeyDDA)
				os.Setenv("DD_CLUSTER_NAME", fakeClusterNameOperator)
			},
			dda:             &v2alpha1.DatadogAgent{},
			wantClusterName: "fake_cluster_name_operator",
			wantAPIKey:      "fake_api_key_operator",
			wantURL:         "https://app.datadoghq.com/api/v1/metadata",
		},
		{
			name: "cluster name set in Operator, API key set in DDA",
			loadFunc: func() {
				os.Clearenv()
				os.Setenv("DD_CLUSTER_NAME", fakeClusterNameOperator)
			},
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterName: apiutils.NewStringPointer(fakeClusterNameDDA),
						Credentials: &v2alpha1.DatadogCredentials{
							APIKey: apiutils.NewStringPointer(fakeAPIKeyDDA),
						},
					},
				},
			},
			wantClusterName: "fake_cluster_name_operator",
			wantAPIKey:      "fake_api_key_dda",
			wantURL:         "https://app.datadoghq.com/api/v1/metadata",
		},
		{
			name: "credentials and site set in DDA",
			loadFunc: func() {
				os.Clearenv()
			},
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						ClusterName: apiutils.NewStringPointer(fakeClusterNameDDA),
						Credentials: &v2alpha1.DatadogCredentials{
							APIKey: apiutils.NewStringPointer(fakeAPIKeyDDA),
						},
						Site: apiutils.NewStringPointer("datad0g.com"),
					},
				},
			},
			wantClusterName: "fake_cluster_name_dda",
			wantAPIKey:      "fake_api_key_dda",
			wantURL:         "https://app.datad0g.com/api/v1/metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()

			mdf := MetadataForwarder{
				requestURL:   getURL(),
				logger:       zap.New(zap.UseDevMode(true)),
				credsManager: config.NewCredentialManager(),
			}

			tt.loadFunc()

			_ = mdf.setupFromOperator()

			_ = mdf.setupFromDDA(tt.dda)

			if mdf.clusterName != tt.wantClusterName {
				t.Errorf("setupFromDDA() clusterName = %v, want %v", mdf.clusterName, tt.wantClusterName)
			}

			if mdf.apiKey != tt.wantAPIKey {
				t.Errorf("setupFromDDA() apiKey = %v, want %v", mdf.apiKey, tt.wantAPIKey)
			}

			if mdf.requestURL != tt.wantURL {
				t.Errorf("setupFromDDA() url = %v, want %v", mdf.requestURL, tt.wantURL)
			}
		})
	}
}
