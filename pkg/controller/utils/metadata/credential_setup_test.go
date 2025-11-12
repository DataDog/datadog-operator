// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	testutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

// mockDecryptor implements secrets.Decryptor interface for testing
type mockDecryptor struct{}

func (m *mockDecryptor) Decrypt(encrypted []string) (map[string]string, error) {
	decrypted := make(map[string]string)
	for _, enc := range encrypted {
		if strings.HasPrefix(enc, "ENC[") && strings.HasSuffix(enc, "]") {
			// Extract content between ENC[ and ]
			content := enc[4 : len(enc)-1]
			decrypted[enc] = content + "-decrypted"
		} else {
			decrypted[enc] = enc // Pass through if not encrypted
		}
	}
	return decrypted, nil
}

// Current coverage
// | Credential Source | Site Config | URL Config | API Key Source             | Status    |
// |-------------------|-------------|------------|----------------------------|-----------|
// | Operator env vars | Default     | Default    | DD_API_KEY                 | ✅         |
// | Operator env vars | DD_SITE     | Default    | DD_API_KEY                 | ✅         |
// | Operator env vars | Default     | DD_URL     | DD_API_KEY                 | ✅         |
// | DDA CRD           | Default     | Default    | spec.credentials.apiKey    | ✅         |
// | DDA CRD           | spec.site   | Default    | spec.credentials.apiKey    | ✅         |
// | DDA CRD           | Default     | Default    | spec.credentials.apiSecret | ✅         |
// | None              | Any         | Any        | Missing                    | ✅ (Error) |
// | Any               | Any         | Any        | No hostname                | ✅ (Error) |

func TestSetupRequestPrerequisites(t *testing.T) {
	tests := []struct {
		name       string
		setupEnv   func()
		setupDDA   func() []client.Object
		wantAPIKey string
		wantURL    string
		wantErr    bool
	}{
		// Pure operator credential tests
		{
			name: "operator creds with default site",
			setupEnv: func() {
				os.Setenv(constants.DDAPIKey, "operator-api-key")
				os.Setenv(constants.DDAppKey, "operator-app-key")
				os.Setenv(constants.DDHostName, "test-hostname")
				os.Setenv(constants.DDClusterName, "test-cluster")
			},
			setupDDA: func() []client.Object {
				return []client.Object{} // No DDA needed
			},
			wantAPIKey: "operator-api-key",
			wantURL:    "https://app.datadoghq.com/api/v1/metadata",
			wantErr:    false,
		},
		{
			name: "operator creds with custom site via DD_SITE",
			setupEnv: func() {
				os.Setenv(constants.DDAPIKey, "operator-api-key")
				os.Setenv(constants.DDAppKey, "operator-app-key")
				os.Setenv(constants.DDHostName, "test-hostname")
				os.Setenv(constants.DDClusterName, "test-cluster")
				os.Setenv("DD_SITE", "datadoghq.eu")
			},
			setupDDA: func() []client.Object {
				return []client.Object{} // No DDA needed
			},
			wantAPIKey: "operator-api-key",
			wantURL:    "https://app.datadoghq.eu/api/v1/metadata",
			wantErr:    false,
		},
		{
			name: "operator creds with custom URL via DD_URL",
			setupEnv: func() {
				os.Setenv(constants.DDAPIKey, "operator-api-key")
				os.Setenv(constants.DDAppKey, "operator-app-key")
				os.Setenv(constants.DDHostName, "test-hostname")
				os.Setenv(constants.DDClusterName, "test-cluster")
				os.Setenv("DD_URL", "https://custom.datadoghq.com")
			},
			setupDDA: func() []client.Object {
				return []client.Object{} // No DDA needed
			},
			wantAPIKey: "operator-api-key",
			wantURL:    "https://custom.datadoghq.com/api/v1/metadata",
			wantErr:    false,
		},
		// Pure DDA credential tests
		{
			name: "DDA with plaintext API key and default site",
			setupEnv: func() {
				os.Setenv(constants.DDHostName, "test-hostname")
				// No operator credentials
			},
			setupDDA: func() []client.Object {
				return []client.Object{
					&v2alpha1.DatadogAgent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-dda",
							Namespace: "default",
						},
						Spec: v2alpha1.DatadogAgentSpec{
							Global: &v2alpha1.GlobalConfig{
								Credentials: &v2alpha1.DatadogCredentials{
									APIKey: apiutils.NewStringPointer("dda-api-key"),
								},
							},
						},
					},
				}
			},
			wantAPIKey: "dda-api-key",
			wantURL:    "https://app.datadoghq.com/api/v1/metadata",
			wantErr:    false,
		},
		{
			name: "DDA with API key and custom site",
			setupEnv: func() {
				os.Setenv(constants.DDHostName, "test-hostname")
				// No operator credentials
			},
			setupDDA: func() []client.Object {
				return []client.Object{
					&v2alpha1.DatadogAgent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-dda",
							Namespace: "default",
						},
						Spec: v2alpha1.DatadogAgentSpec{
							Global: &v2alpha1.GlobalConfig{
								Credentials: &v2alpha1.DatadogCredentials{
									APIKey: apiutils.NewStringPointer("dda-api-key"),
								},
								Site: apiutils.NewStringPointer("datadoghq.eu"),
							},
						},
					},
				}
			},
			wantAPIKey: "dda-api-key",
			wantURL:    "https://app.datadoghq.eu/api/v1/metadata",
			wantErr:    false,
		},
		{
			name: "DDA with secret reference",
			setupEnv: func() {
				os.Setenv(constants.DDHostName, "test-hostname")
				// No operator credentials
			},
			setupDDA: func() []client.Object {
				return []client.Object{
					&v2alpha1.DatadogAgent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-dda",
							Namespace: "default",
						},
						Spec: v2alpha1.DatadogAgentSpec{
							Global: &v2alpha1.GlobalConfig{
								Credentials: &v2alpha1.DatadogCredentials{
									APISecret: &v2alpha1.SecretConfig{
										SecretName: "datadog-secret",
										KeyName:    "api-key",
									},
									AppSecret: &v2alpha1.SecretConfig{
										SecretName: "datadog-secret",
										KeyName:    "app-key",
									},
								},
							},
						},
					},
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"api-key": []byte("secret-api-key"),
						},
					},
				}
			},
			wantAPIKey: "secret-api-key",
			wantURL:    "https://app.datadoghq.com/api/v1/metadata",
			wantErr:    false,
		},
		{
			name: "DDA with encrypted API key",
			setupEnv: func() {
				os.Setenv(constants.DDHostName, "test-hostname")
				// No operator credentials
			},
			setupDDA: func() []client.Object {
				return []client.Object{
					&v2alpha1.DatadogAgent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-dda",
							Namespace: "default",
						},
						Spec: v2alpha1.DatadogAgentSpec{
							Global: &v2alpha1.GlobalConfig{
								Credentials: &v2alpha1.DatadogCredentials{
									APIKey: apiutils.NewStringPointer("ENC[encrypted-api-key]"),
								},
							},
						},
					},
				}
			},
			wantAPIKey: "encrypted-api-key-decrypted", // Mock decrypts "ENC[encrypted-api-key]" to this
			wantURL:    "https://app.datadoghq.com/api/v1/metadata",
			wantErr:    false, // Still expect error due to uninitialized decryptor
		},
		// Mixed/fallback tests
		{
			name: "operator creds without cluster name falls back to DDA cluster name but operator API key",
			setupEnv: func() {
				os.Setenv(constants.DDAPIKey, "operator-api-key")
				os.Setenv(constants.DDAppKey, "operator-app-key")
				os.Setenv(constants.DDHostName, "test-hostname")
				// Note: No DD_CLUSTER_NAME set to trigger fallback
			},
			setupDDA: func() []client.Object {
				return []client.Object{
					&v2alpha1.DatadogAgent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-dda",
							Namespace: "default",
						},
						Spec: v2alpha1.DatadogAgentSpec{
							Global: &v2alpha1.GlobalConfig{
								ClusterName: apiutils.NewStringPointer("dda-cluster-name"),
								Credentials: &v2alpha1.DatadogCredentials{
									APIKey: apiutils.NewStringPointer("dda-fallback-key"),
								},
							},
						},
					},
				}
			},
			wantAPIKey: "operator-api-key", // Should use DDA credentials
			wantURL:    "https://app.datadoghq.com/api/v1/metadata",
			wantErr:    false,
		},
		// Error cases
		{
			name: "missing hostname should fail",
			setupEnv: func() {
				os.Setenv(constants.DDAPIKey, "operator-api-key")
				os.Setenv(constants.DDAppKey, "operator-app-key")
				// No DDHostName set
			},
			setupDDA: func() []client.Object {
				return []client.Object{} // No DDA
			},
			wantErr: true,
		},
		{
			name: "no credentials anywhere should fail",
			setupEnv: func() {
				os.Setenv(constants.DDHostName, "test-hostname")
				// No operator credentials
			},
			setupDDA: func() []client.Object {
				return []client.Object{} // No DDA
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			tt.setupEnv()

			// Create test client with DDA resources
			scheme := testutils.TestScheme()
			clientObjects := tt.setupDDA()

			// Add kube-system namespace for cluster UID
			kubeSystem := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
					UID:  "test-cluster-uid",
				},
			}
			clientObjects = append(clientObjects, kubeSystem)
			client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v2alpha1.DatadogAgent{}).WithObjects(clientObjects...).Build()

			credsManager := config.NewCredentialManagerWithDecryptor(client, &mockDecryptor{})
			omf := &OperatorMetadataForwarder{
				SharedMetadata: NewSharedMetadata(
					zap.New(zap.UseDevMode(true)),
					client,
					"v1.28.0",
					"v1.0.0",
					credsManager,
				),
				OperatorMetadata: OperatorMetadata{},
			}

			// Call setupRequestPrerequisites
			err := omf.setupRequestPrerequisites()

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			// Verify no error
			require.NoError(t, err)

			// Verify API key is set correctly
			assert.Equal(t, tt.wantAPIKey, omf.apiKey, "API key should match expected value")

			// Verify URL is set correctly
			assert.Equal(t, tt.wantURL, omf.requestURL, "Request URL should match expected value")

			// Verify headers are set with correct API key
			headers := omf.payloadHeader
			assert.Equal(t, tt.wantAPIKey, headers.Get("Dd-Api-Key"), "Header should contain correct API key")
			assert.Equal(t, "application/json", headers.Get("Content-Type"), "Content-Type header should be set")
			assert.Equal(t, "application/json", headers.Get("Accept"), "Accept header should be set")

			// Verify cluster UID is set
			assert.NotEmpty(t, omf.clusterUID, "Cluster UID should be set")
		})
	}
}
