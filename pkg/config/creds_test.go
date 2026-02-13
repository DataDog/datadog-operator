// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	testutils_test "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/secrets"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_getCredentials(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(*CredentialManager) *secrets.DummyDecryptor
		want      Creds
		wantErr   bool
		wantFunc  func(*testing.T, *secrets.DummyDecryptor, *CredentialManager)
		resetFunc func(*CredentialManager)
	}{
		{
			name: "creds found, no SB",
			setupFunc: func(*CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_API_KEY", "foo")
				os.Setenv("DD_APP_KEY", "bar")
				return secrets.NewDummyDecryptor(0)
			},
			want:    Creds{APIKey: "foo", AppKey: "bar"},
			wantErr: false,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) {
				d.AssertNotCalled(t, "Decrypt")
				cachedCreds, cached := cm.getCredsFromCache()
				assert.True(t, cached)
				assert.EqualValues(t, Creds{APIKey: "foo", AppKey: "bar"}, cachedCreds)
			},
			resetFunc: func(cm *CredentialManager) {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "creds found in cache",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				cm.cacheCreds(Creds{APIKey: "foo", AppKey: "bar"})
				return secrets.NewDummyDecryptor(0)
			},
			want:    Creds{APIKey: "foo", AppKey: "bar"},
			wantErr: false,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) {
				d.AssertNotCalled(t, "Decrypt")
				cachedCreds, cached := cm.getCredsFromCache()
				assert.True(t, cached)
				assert.EqualValues(t, Creds{APIKey: "foo", AppKey: "bar"}, cachedCreds)
			},
			resetFunc: func(cm *CredentialManager) {},
		},
		{
			name: "creds found, both encrypted",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_API_KEY", "ENC[ApiKey]")
				os.Setenv("DD_APP_KEY", "ENC[AppKey]")
				d := secrets.NewDummyDecryptor(0)
				d.On("Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"}).Once()
				return d
			},
			want:    Creds{APIKey: "DEC[ENC[ApiKey]]", AppKey: "DEC[ENC[AppKey]]"},
			wantErr: false,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) {
				d.AssertCalled(t, "Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"})
				cachedCreds, cached := cm.getCredsFromCache()
				assert.True(t, cached)
				assert.EqualValues(t, Creds{APIKey: "DEC[ENC[ApiKey]]", AppKey: "DEC[ENC[AppKey]]"}, cachedCreds)
			},
			resetFunc: func(cm *CredentialManager) {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")

			},
		},
		{
			name: "creds found, api key encrypted",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_API_KEY", "ENC[ApiKey]")
				os.Setenv("DD_APP_KEY", "bar")
				d := secrets.NewDummyDecryptor(0)
				d.On("Decrypt", []string{"ENC[ApiKey]"}).Once()
				return d
			},
			want:    Creds{APIKey: "DEC[ENC[ApiKey]]", AppKey: "bar"},
			wantErr: false,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) {
				d.AssertCalled(t, "Decrypt", []string{"ENC[ApiKey]"})
				cachedCreds, cached := cm.getCredsFromCache()
				assert.True(t, cached)
				assert.EqualValues(t, Creds{APIKey: "DEC[ENC[ApiKey]]", AppKey: "bar"}, cachedCreds)
			},
			resetFunc: func(cm *CredentialManager) {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "creds found, app key encrypted",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_API_KEY", "foo")
				os.Setenv("DD_APP_KEY", "ENC[AppKey]")
				d := secrets.NewDummyDecryptor(0)
				d.On("Decrypt", []string{"ENC[AppKey]"}).Once()
				return d
			},
			want:    Creds{APIKey: "foo", AppKey: "DEC[ENC[AppKey]]"},
			wantErr: false,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) {
				d.AssertCalled(t, "Decrypt", []string{"ENC[AppKey]"})
				cachedCreds, cached := cm.getCredsFromCache()
				assert.True(t, cached)
				assert.EqualValues(t, Creds{APIKey: "foo", AppKey: "DEC[ENC[AppKey]]"}, cachedCreds)
			},
			resetFunc: func(cm *CredentialManager) {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name:      "creds not found",
			setupFunc: func(*CredentialManager) *secrets.DummyDecryptor { return secrets.NewDummyDecryptor(0) },
			want:      Creds{},
			wantErr:   true,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func(cm *CredentialManager) {},
		},
		{
			name: "app key not found",
			setupFunc: func(*CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_API_KEY", "foo")
				return secrets.NewDummyDecryptor(0)
			},
			want:      Creds{},
			wantErr:   true,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func(cm *CredentialManager) { os.Unsetenv("DD_API_KEY") },
		},
		{
			name: "api key not found",
			setupFunc: func(*CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_APP_KEY", "bar")
				return secrets.NewDummyDecryptor(0)
			},
			want:      Creds{},
			wantErr:   true,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func(cm *CredentialManager) { os.Unsetenv("DD_APP_KEY") },
		},
		{
			name: "creds found, decrypted after 3 retries",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_API_KEY", "ENC[ApiKey]")
				os.Setenv("DD_APP_KEY", "ENC[AppKey]")
				d := secrets.NewDummyDecryptor(3)
				d.On("Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"}).Times(3)
				return d
			},
			want:    Creds{APIKey: "DEC[ENC[ApiKey]]", AppKey: "DEC[ENC[AppKey]]"},
			wantErr: false,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) {
				d.AssertCalled(t, "Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"})
				d.AssertNumberOfCalls(t, "Decrypt", 3)
				cachedCreds, cached := cm.getCredsFromCache()
				assert.True(t, cached)
				assert.EqualValues(t, Creds{APIKey: "DEC[ENC[ApiKey]]", AppKey: "DEC[ENC[AppKey]]"}, cachedCreds)
			},
			resetFunc: func(cm *CredentialManager) {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "creds found, cannot be decrypted",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_API_KEY", "ENC[ApiKey]")
				os.Setenv("DD_APP_KEY", "ENC[AppKey]")
				d := secrets.NewDummyDecryptor(-1)
				d.On("Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"}).Once()
				return d
			},
			want:    Creds{},
			wantErr: true,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) {
				d.AssertCalled(t, "Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"})
				d.AssertNumberOfCalls(t, "Decrypt", 1)
			},
			resetFunc: func(cm *CredentialManager) {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testutils_test.TestScheme()
			client := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&v2alpha1.DatadogAgent{}).Build()
			credsManager := NewCredentialManager(client)
			decryptor := tt.setupFunc(credsManager)
			credsManager.secretBackend = decryptor
			got, err := credsManager.GetCredentials()
			assert.Equal(t, tt.wantErr, err != nil)
			assert.EqualValues(t, tt.want, got)
			tt.wantFunc(t, decryptor, credsManager)
			tt.resetFunc(credsManager)
		})
	}
}

func Test_refresh(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(*CredentialManager) *secrets.DummyDecryptor
		callbackHit bool
		wantErr     bool
		resetFunc   func()
	}{
		{
			name: "no refresh when creds unchanged",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				// Set same creds in cache and env
				os.Setenv("DD_API_KEY", "same-api")
				os.Setenv("DD_APP_KEY", "same-app")
				cm.cacheCreds(Creds{APIKey: "same-api", AppKey: "same-app"})
				return secrets.NewDummyDecryptor(0)
			},
			callbackHit: false,
			wantErr:     false,
			resetFunc: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "refresh triggers callback on cred change",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				// Set different creds in cache vs env
				os.Setenv("DD_API_KEY", "new-api")
				os.Setenv("DD_APP_KEY", "new-app")
				cm.cacheCreds(Creds{APIKey: "old-api", AppKey: "old-app"})
				return secrets.NewDummyDecryptor(0)
			},
			callbackHit: true,
			wantErr:     false,
			resetFunc: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "refresh returns error on GetCredentials failure",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				// No env vars set - will cause GetCredentials to fail
				cm.cacheCreds(Creds{APIKey: "old-api", AppKey: "old-app"})
				return secrets.NewDummyDecryptor(0)
			},
			callbackHit: false,
			wantErr:     true,
			resetFunc:   func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer tt.resetFunc()
			s := testutils_test.TestScheme()
			client := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&v2alpha1.DatadogAgent{}).Build()
			cm := NewCredentialManager(client)
			decryptor := tt.setupFunc(cm)
			cm.secretBackend = decryptor

			// Track if callback was called
			var callbackCalled bool
			cm.RegisterCallback(func(newCreds Creds) error {
				callbackCalled = true
				return nil
			})

			err := cm.refresh(logr.Logger{})

			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.callbackHit, callbackCalled)
		})
	}
}

func Test_getCredentialsForMetadata(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(*CredentialManager) *secrets.DummyDecryptor
		want      Creds
		wantErr   bool
		resetFunc func(*CredentialManager)
	}{
		{
			name: "cache hit with API key only (app key empty)",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				cm.cacheCreds(Creds{APIKey: "cached-api", AppKey: ""})
				return secrets.NewDummyDecryptor(0)
			},
			want:      Creds{APIKey: "cached-api", AppKey: ""},
			wantErr:   false,
			resetFunc: func(cm *CredentialManager) {},
		},
		{
			name: "API key only (app key optional for metadata)",
			setupFunc: func(*CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_API_KEY", "test-api-key")
				// DD_APP_KEY intentionally not set
				return secrets.NewDummyDecryptor(0)
			},
			want:    Creds{APIKey: "test-api-key", AppKey: ""},
			wantErr: false,
			resetFunc: func(cm *CredentialManager) {
				os.Unsetenv("DD_API_KEY")
			},
		},
		{
			name: "both API key and app key set",
			setupFunc: func(*CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_API_KEY", "test-api-key")
				os.Setenv("DD_APP_KEY", "test-app-key")
				return secrets.NewDummyDecryptor(0)
			},
			want:    Creds{APIKey: "test-api-key", AppKey: "test-app-key"},
			wantErr: false,
			resetFunc: func(cm *CredentialManager) {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "missing API key should error",
			setupFunc: func(*CredentialManager) *secrets.DummyDecryptor {
				os.Setenv("DD_APP_KEY", "test-app-key")
				// DD_API_KEY intentionally not set
				return secrets.NewDummyDecryptor(0)
			},
			want:    Creds{},
			wantErr: true,
			resetFunc: func(cm *CredentialManager) {
				os.Unsetenv("DD_APP_KEY")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testutils_test.TestScheme()
			client := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&v2alpha1.DatadogAgent{}).Build()
			credsManager := NewCredentialManager(client)
			decryptor := tt.setupFunc(credsManager)
			credsManager.secretBackend = decryptor
			got, err := credsManager.GetCredentialsForMetadata()
			assert.Equal(t, tt.wantErr, err != nil)
			assert.EqualValues(t, tt.want, got)
			tt.resetFunc(credsManager)
		})
	}
}

func Test_getCredentialsFromConfigMap(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() *CredentialManager
		want      Creds
		wantErr   bool
		resetFunc func()
	}{
		{
			name: "all fields present",
			setupFunc: func() *CredentialManager {
				os.Setenv("POD_NAME", "test-operator-pod")
				os.Setenv("POD_NAMESPACE", "test-namespace")

				pod := &corev1.Pod{}
				pod.Name = "test-operator-pod"
				pod.Namespace = "test-namespace"
				pod.Labels = map[string]string{
					"app.kubernetes.io/instance": "my-release",
				}

				configMap := &corev1.ConfigMap{}
				configMap.Name = "my-release-endpoint-config"
				configMap.Namespace = "test-namespace"
				configMap.Data = map[string]string{
					"api-key-secret-name": "api-key-secret",
					"app-key-secret-name": "app-key-secret",
					"dd-site":             "datadoghq.eu",
					"dd-url":              "https://api.datadoghq.eu",
				}

				apiKeySecret := &corev1.Secret{}
				apiKeySecret.Name = "api-key-secret"
				apiKeySecret.Namespace = "test-namespace"
				apiKeySecret.Data = map[string][]byte{
					"api-key": []byte("test-api-key"),
				}

				appKeySecret := &corev1.Secret{}
				appKeySecret.Name = "app-key-secret"
				appKeySecret.Namespace = "test-namespace"
				appKeySecret.Data = map[string][]byte{
					"app-key": []byte("test-app-key"),
				}

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).WithObjects(pod, configMap, apiKeySecret, appKeySecret).Build()
				return NewCredentialManager(client)
			},
			want: Creds{
				APIKey: "test-api-key",
				AppKey: "test-app-key",
				Site:   strPtr("datadoghq.eu"),
				URL:    strPtr("https://api.datadoghq.eu"),
			},
			wantErr: false,
			resetFunc: func() {
				os.Unsetenv("POD_NAME")
				os.Unsetenv("POD_NAMESPACE")
			},
		},
		{
			name: "only required fields (api-key-secret-name)",
			setupFunc: func() *CredentialManager {
				os.Setenv("POD_NAME", "test-operator-pod")
				os.Setenv("POD_NAMESPACE", "test-namespace")

				pod := &corev1.Pod{}
				pod.Name = "test-operator-pod"
				pod.Namespace = "test-namespace"
				pod.Labels = map[string]string{
					"app.kubernetes.io/instance": "my-release",
				}

				configMap := &corev1.ConfigMap{}
				configMap.Name = "my-release-endpoint-config"
				configMap.Namespace = "test-namespace"
				configMap.Data = map[string]string{
					"api-key-secret-name": "api-key-secret",
				}

				apiKeySecret := &corev1.Secret{}
				apiKeySecret.Name = "api-key-secret"
				apiKeySecret.Namespace = "test-namespace"
				apiKeySecret.Data = map[string][]byte{
					"api-key": []byte("test-api-key"),
				}

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).WithObjects(pod, configMap, apiKeySecret).Build()
				return NewCredentialManager(client)
			},
			want: Creds{
				APIKey: "test-api-key",
				AppKey: "",
			},
			wantErr: false,
			resetFunc: func() {
				os.Unsetenv("POD_NAME")
				os.Unsetenv("POD_NAMESPACE")
			},
		},
		{
			name: "missing POD_NAMESPACE should error",
			setupFunc: func() *CredentialManager {
				os.Setenv("POD_NAME", "test-operator-pod")
				// POD_NAMESPACE not set

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).Build()
				return NewCredentialManager(client)
			},
			want:    Creds{},
			wantErr: true,
			resetFunc: func() {
				os.Unsetenv("POD_NAME")
			},
		},
		{
			name: "missing api-key-secret-name in ConfigMap",
			setupFunc: func() *CredentialManager {
				os.Setenv("POD_NAME", "test-operator-pod")
				os.Setenv("POD_NAMESPACE", "test-namespace")

				pod := &corev1.Pod{}
				pod.Name = "test-operator-pod"
				pod.Namespace = "test-namespace"
				pod.Labels = map[string]string{
					"app.kubernetes.io/instance": "my-release",
				}

				configMap := &corev1.ConfigMap{}
				configMap.Name = "my-release-endpoint-config"
				configMap.Namespace = "test-namespace"
				configMap.Data = map[string]string{
					"dd-site": "datadoghq.com",
				}

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).WithObjects(pod, configMap).Build()
				return NewCredentialManager(client)
			},
			want:    Creds{},
			wantErr: true,
			resetFunc: func() {
				os.Unsetenv("POD_NAME")
				os.Unsetenv("POD_NAMESPACE")
			},
		},
		{
			name: "API key secret not found",
			setupFunc: func() *CredentialManager {
				os.Setenv("POD_NAME", "test-operator-pod")
				os.Setenv("POD_NAMESPACE", "test-namespace")

				pod := &corev1.Pod{}
				pod.Name = "test-operator-pod"
				pod.Namespace = "test-namespace"
				pod.Labels = map[string]string{
					"app.kubernetes.io/instance": "my-release",
				}

				configMap := &corev1.ConfigMap{}
				configMap.Name = "my-release-endpoint-config"
				configMap.Namespace = "test-namespace"
				configMap.Data = map[string]string{
					"api-key-secret-name": "nonexistent-secret",
				}

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).WithObjects(pod, configMap).Build()
				return NewCredentialManager(client)
			},
			want:    Creds{},
			wantErr: true,
			resetFunc: func() {
				os.Unsetenv("POD_NAME")
				os.Unsetenv("POD_NAMESPACE")
			},
		},
		{
			name: "empty API key in secret",
			setupFunc: func() *CredentialManager {
				os.Setenv("POD_NAME", "test-operator-pod")
				os.Setenv("POD_NAMESPACE", "test-namespace")

				pod := &corev1.Pod{}
				pod.Name = "test-operator-pod"
				pod.Namespace = "test-namespace"
				pod.Labels = map[string]string{
					"app.kubernetes.io/instance": "my-release",
				}

				configMap := &corev1.ConfigMap{}
				configMap.Name = "my-release-endpoint-config"
				configMap.Namespace = "test-namespace"
				configMap.Data = map[string]string{
					"api-key-secret-name": "api-key-secret",
				}

				apiKeySecret := &corev1.Secret{}
				apiKeySecret.Name = "api-key-secret"
				apiKeySecret.Namespace = "test-namespace"
				apiKeySecret.Data = map[string][]byte{
					"api-key": []byte(""),
				}

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).WithObjects(pod, configMap, apiKeySecret).Build()
				return NewCredentialManager(client)
			},
			want:    Creds{},
			wantErr: true,
			resetFunc: func() {
				os.Unsetenv("POD_NAME")
				os.Unsetenv("POD_NAMESPACE")
			},
		},
		{
			name: "app key secret missing (should not error)",
			setupFunc: func() *CredentialManager {
				os.Setenv("POD_NAME", "test-operator-pod")
				os.Setenv("POD_NAMESPACE", "test-namespace")

				pod := &corev1.Pod{}
				pod.Name = "test-operator-pod"
				pod.Namespace = "test-namespace"
				pod.Labels = map[string]string{
					"app.kubernetes.io/instance": "my-release",
				}

				configMap := &corev1.ConfigMap{}
				configMap.Name = "my-release-endpoint-config"
				configMap.Namespace = "test-namespace"
				configMap.Data = map[string]string{
					"api-key-secret-name": "api-key-secret",
					"app-key-secret-name": "nonexistent-app-secret",
				}

				apiKeySecret := &corev1.Secret{}
				apiKeySecret.Name = "api-key-secret"
				apiKeySecret.Namespace = "test-namespace"
				apiKeySecret.Data = map[string][]byte{
					"api-key": []byte("test-api-key"),
				}

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).WithObjects(pod, configMap, apiKeySecret).Build()
				return NewCredentialManager(client)
			},
			want: Creds{
				APIKey: "test-api-key",
				AppKey: "",
			},
			wantErr: false,
			resetFunc: func() {
				os.Unsetenv("POD_NAME")
				os.Unsetenv("POD_NAMESPACE")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer tt.resetFunc()
			cm := tt.setupFunc()
			got, err := cm.getCredentialsFromConfigMap()
			assert.Equal(t, tt.wantErr, err != nil)
			if !tt.wantErr {
				assert.Equal(t, tt.want.APIKey, got.APIKey)
				assert.Equal(t, tt.want.AppKey, got.AppKey)
				if tt.want.Site != nil {
					assert.NotNil(t, got.Site)
					assert.Equal(t, *tt.want.Site, *got.Site)
				} else {
					assert.Nil(t, got.Site)
				}
				if tt.want.URL != nil {
					assert.NotNil(t, got.URL)
					assert.Equal(t, *tt.want.URL, *got.URL)
				} else {
					assert.Nil(t, got.URL)
				}
			}
		})
	}
}

func Test_GetCredsWithDDAFallback_withConfigMapTier(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (*CredentialManager, func() (*v2alpha1.DatadogAgent, error))
		want      Creds
		wantErr   bool
		resetFunc func()
	}{
		{
			name: "Tier 1: env vars take priority",
			setupFunc: func() (*CredentialManager, func() (*v2alpha1.DatadogAgent, error)) {
				os.Setenv("DD_API_KEY", "env-api-key")
				os.Setenv("DD_APP_KEY", "env-app-key")
				os.Setenv("DD_SITE", "datadoghq.com")

				// Setup ConfigMap (should be ignored)
				os.Setenv("POD_NAME", "test-operator-pod")
				os.Setenv("POD_NAMESPACE", "test-namespace")

				pod := &corev1.Pod{}
				pod.Name = "test-operator-pod"
				pod.Namespace = "test-namespace"
				pod.Labels = map[string]string{
					"app.kubernetes.io/instance": "my-release",
				}

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
				cm := NewCredentialManager(client)

				getDDA := func() (*v2alpha1.DatadogAgent, error) {
					return nil, nil
				}

				return cm, getDDA
			},
			want: Creds{
				APIKey: "env-api-key",
				AppKey: "env-app-key",
				Site:   strPtr("datadoghq.com"),
			},
			wantErr: false,
			resetFunc: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
				os.Unsetenv("DD_SITE")
				os.Unsetenv("POD_NAME")
				os.Unsetenv("POD_NAMESPACE")
			},
		},
		{
			name: "Tier 2: ConfigMap fallback when env vars missing",
			setupFunc: func() (*CredentialManager, func() (*v2alpha1.DatadogAgent, error)) {
				// No env vars set
				os.Setenv("POD_NAME", "test-operator-pod")
				os.Setenv("POD_NAMESPACE", "test-namespace")

				pod := &corev1.Pod{}
				pod.Name = "test-operator-pod"
				pod.Namespace = "test-namespace"
				pod.Labels = map[string]string{
					"app.kubernetes.io/instance": "my-release",
				}

				configMap := &corev1.ConfigMap{}
				configMap.Name = "my-release-endpoint-config"
				configMap.Namespace = "test-namespace"
				configMap.Data = map[string]string{
					"api-key-secret-name": "api-key-secret",
					"dd-site":             "datadoghq.eu",
				}

				apiKeySecret := &corev1.Secret{}
				apiKeySecret.Name = "api-key-secret"
				apiKeySecret.Namespace = "test-namespace"
				apiKeySecret.Data = map[string][]byte{
					"api-key": []byte("configmap-api-key"),
				}

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).WithObjects(pod, configMap, apiKeySecret).Build()
				cm := NewCredentialManager(client)

				getDDA := func() (*v2alpha1.DatadogAgent, error) {
					return nil, nil
				}

				return cm, getDDA
			},
			want: Creds{
				APIKey: "configmap-api-key",
				Site:   strPtr("datadoghq.eu"),
			},
			wantErr: false,
			resetFunc: func() {
				os.Unsetenv("POD_NAME")
				os.Unsetenv("POD_NAMESPACE")
			},
		},
		{
			name: "Tier 3: DatadogAgent fallback when ConfigMap missing",
			setupFunc: func() (*CredentialManager, func() (*v2alpha1.DatadogAgent, error)) {
				// No env vars, no ConfigMap setup
				os.Setenv("POD_NAME", "test-operator-pod")
				os.Setenv("POD_NAMESPACE", "test-namespace")

				s := testutils_test.TestScheme()
				client := fake.NewClientBuilder().WithScheme(s).Build()
				cm := NewCredentialManager(client)

				getDDA := func() (*v2alpha1.DatadogAgent, error) {
					dda := &v2alpha1.DatadogAgent{}
					dda.Namespace = "test-namespace"
					dda.Spec.Global = &v2alpha1.GlobalConfig{}
					dda.Spec.Global.Credentials = &v2alpha1.DatadogCredentials{}
					apiKey := "dda-api-key"
					dda.Spec.Global.Credentials.APIKey = &apiKey
					site := "datadoghq.com"
					dda.Spec.Global.Site = &site
					return dda, nil
				}

				return cm, getDDA
			},
			want: Creds{
				APIKey: "dda-api-key",
				Site:   strPtr("datadoghq.com"),
			},
			wantErr: false,
			resetFunc: func() {
				os.Unsetenv("POD_NAME")
				os.Unsetenv("POD_NAMESPACE")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer tt.resetFunc()
			cm, getDDA := tt.setupFunc()
			got, err := cm.GetCredsWithDDAFallback(getDDA)
			assert.Equal(t, tt.wantErr, err != nil)
			if !tt.wantErr {
				assert.Equal(t, tt.want.APIKey, got.APIKey)
				assert.Equal(t, tt.want.AppKey, got.AppKey)
				if tt.want.Site != nil {
					assert.NotNil(t, got.Site)
					assert.Equal(t, *tt.want.Site, *got.Site)
				} else {
					assert.Nil(t, got.Site)
				}
				if tt.want.URL != nil {
					assert.NotNil(t, got.URL)
					assert.Equal(t, *tt.want.URL, *got.URL)
				} else {
					assert.Nil(t, got.URL)
				}
			}
		})
	}
}

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}
