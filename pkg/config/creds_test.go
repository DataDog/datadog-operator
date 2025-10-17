// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-operator/pkg/secrets"
	"github.com/go-logr/logr"

	"github.com/stretchr/testify/assert"
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
			credsManager := NewCredentialManager()
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

func Test_RefreshVersionIncrement(t *testing.T) {
	tests := []struct {
		name              string
		setupFunc         func(*CredentialManager) *secrets.DummyDecryptor
		expectedVersion   int64
		expectVersionIncr bool
		resetFunc         func()
	}{
		{
			name: "version increments when credentials change",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				// Set initial credentials
				cm.cacheCreds(Creds{APIKey: "oldkey", AppKey: "oldapp"})
				os.Setenv("DD_API_KEY", "newkey")
				os.Setenv("DD_APP_KEY", "newapp")
				return secrets.NewDummyDecryptor(0)
			},
			expectedVersion:   2,
			expectVersionIncr: true,
			resetFunc: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "version doesn't increment when credentials are same",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				cm.cacheCreds(Creds{APIKey: "samekey", AppKey: "sameapp"})
				os.Setenv("DD_API_KEY", "samekey")
				os.Setenv("DD_APP_KEY", "sameapp")
				return secrets.NewDummyDecryptor(0)
			},
			expectedVersion:   1,
			expectVersionIncr: false,
			resetFunc: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "version doesn't increment on error",
			setupFunc: func(cm *CredentialManager) *secrets.DummyDecryptor {
				cm.cacheCreds(Creds{APIKey: "key", AppKey: "app"})
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
				return secrets.NewDummyDecryptor(0)
			},
			expectedVersion:   1,
			expectVersionIncr: false,
			resetFunc:         func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := NewCredentialManager()
			decryptor := tt.setupFunc(cm)
			cm.secretBackend = decryptor

			initialVersion := cm.GetCredentialVersion()
			err := cm.refresh()

			if tt.expectVersionIncr {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVersion, cm.GetCredentialVersion())
				assert.Greater(t, cm.GetCredentialVersion(), initialVersion)
			} else {
				assert.Equal(t, initialVersion, cm.GetCredentialVersion())
			}
			tt.resetFunc()
		})
	}
}

// NOTE: could be a finicky test since it is time based, but I'm not sure how else to test the refresh routine

func Test_StartCredentialRefreshRoutine(t *testing.T) {
	cm := NewCredentialManager()
	d := secrets.NewDummyDecryptor(0)

	cm.secretBackend = d

	os.Setenv("DD_API_KEY", "ENC[oldkey]")
	os.Setenv("DD_APP_KEY", "ENC[oldapp]")

	d.On("Decrypt", []string{"ENC[oldkey]", "ENC[oldapp]"}).Once()

	// Get credentials to establish initial state
	cm.GetCredentials()

	// that or just directly cache creds
	// cm.cacheCreds(Creds{APIKey: "DEC[ENC[oldkey]]", AppKey: "DEC[ENC[oldapp]]"})
	// require.NoError(t, err)
	initialVersion := cm.GetCredentialVersion()
	t.Logf("Initial version: %d", initialVersion)

	// get credentials to cache value
	go cm.StartCredentialRefreshRoutine(10*time.Millisecond, logr.Discard())

	// Change credentials to new encrypted values
	os.Setenv("DD_API_KEY", "ENC[newkey]")
	os.Setenv("DD_APP_KEY", "ENC[newapp]")

	d.On("Decrypt", []string{"ENC[newkey]", "ENC[newapp]"}).Maybe()

	// Wait for refresh
	assert.Eventually(t, func() bool {
		currentVersion := cm.GetCredentialVersion()
		t.Logf("Current version: %d", currentVersion)
		return currentVersion == initialVersion+1
	}, 100*time.Millisecond, 10*time.Millisecond)

	// Cleanup
	os.Unsetenv("DD_API_KEY")
	os.Unsetenv("DD_APP_KEY")
}
