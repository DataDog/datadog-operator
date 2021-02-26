// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package config

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-operator/pkg/secrets"

	"github.com/stretchr/testify/assert"
)

func Test_getCredentials(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(*secrets.DummyDecryptor, *CredentialManager)
		want      Creds
		wantErr   bool
		wantFunc  func(*testing.T, *secrets.DummyDecryptor, *CredentialManager)
		resetFunc func(*CredentialManager)
	}{
		{
			name: "creds found, no SB",
			setupFunc: func(*secrets.DummyDecryptor, *CredentialManager) {
				os.Setenv("DD_API_KEY", "foo")
				os.Setenv("DD_APP_KEY", "bar")
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
			setupFunc: func(d *secrets.DummyDecryptor, cm *CredentialManager) {
				cm.cacheCreds(Creds{APIKey: "foo", AppKey: "bar"})
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
			setupFunc: func(d *secrets.DummyDecryptor, cm *CredentialManager) {
				os.Setenv("DD_API_KEY", "ENC[ApiKey]")
				os.Setenv("DD_APP_KEY", "ENC[AppKey]")
				d.On("Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"}).Once()
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
			setupFunc: func(d *secrets.DummyDecryptor, cm *CredentialManager) {
				os.Setenv("DD_API_KEY", "ENC[ApiKey]")
				os.Setenv("DD_APP_KEY", "bar")
				d.On("Decrypt", []string{"ENC[ApiKey]"}).Once()
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
			setupFunc: func(d *secrets.DummyDecryptor, cm *CredentialManager) {
				os.Setenv("DD_API_KEY", "foo")
				os.Setenv("DD_APP_KEY", "ENC[AppKey]")
				d.On("Decrypt", []string{"ENC[AppKey]"}).Once()
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
			setupFunc: func(*secrets.DummyDecryptor, *CredentialManager) {},
			want:      Creds{},
			wantErr:   true,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func(cm *CredentialManager) {},
		},
		{
			name:      "app key not found",
			setupFunc: func(*secrets.DummyDecryptor, *CredentialManager) { os.Setenv("DD_API_KEY", "foo") },
			want:      Creds{},
			wantErr:   true,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func(cm *CredentialManager) { os.Unsetenv("DD_API_KEY") },
		},
		{
			name:      "api key not found",
			setupFunc: func(*secrets.DummyDecryptor, *CredentialManager) { os.Setenv("DD_APP_KEY", "bar") },
			want:      Creds{},
			wantErr:   true,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor, cm *CredentialManager) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func(cm *CredentialManager) { os.Unsetenv("DD_APP_KEY") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credsManager := NewCredentialManager()
			decryptor := &secrets.DummyDecryptor{}
			credsManager.secretBackend = decryptor
			tt.setupFunc(decryptor, credsManager)
			got, err := credsManager.GetCredentials()
			assert.Equal(t, tt.wantErr, err != nil)
			assert.EqualValues(t, tt.want, got)
			tt.wantFunc(t, decryptor, credsManager)
			tt.resetFunc(credsManager)
		})
	}
}
