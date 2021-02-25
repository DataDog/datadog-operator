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
		setupFunc func(*secrets.DummyDecryptor)
		want      Creds
		wantErr   bool
		wantFunc  func(*testing.T, *secrets.DummyDecryptor)
		resetFunc func()
	}{
		{
			name:      "creds found, no SB",
			setupFunc: func(*secrets.DummyDecryptor) { os.Setenv("DD_API_KEY", "foo"); os.Setenv("DD_APP_KEY", "bar") },
			want:      Creds{APIKey: "foo", AppKey: "bar"},
			wantErr:   false,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func() { os.Unsetenv("DD_API_KEY"); os.Unsetenv("DD_APP_KEY") },
		},
		{
			name: "creds found, both encrypted",
			setupFunc: func(d *secrets.DummyDecryptor) {
				os.Setenv("DD_API_KEY", "ENC[ApiKey]")
				os.Setenv("DD_APP_KEY", "ENC[AppKey]")
				d.On("Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"}).Once()
			},
			want:    Creds{APIKey: "DEC[ENC[ApiKey]]", AppKey: "DEC[ENC[AppKey]]"},
			wantErr: false,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor) {
				d.AssertCalled(t, "Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"})
			},
			resetFunc: func() { os.Unsetenv("DD_API_KEY"); os.Unsetenv("DD_APP_KEY") },
		},
		{
			name: "creds found, api key encrypted",
			setupFunc: func(d *secrets.DummyDecryptor) {
				os.Setenv("DD_API_KEY", "ENC[ApiKey]")
				os.Setenv("DD_APP_KEY", "bar")
				d.On("Decrypt", []string{"ENC[ApiKey]"}).Once()
			},
			want:    Creds{APIKey: "DEC[ENC[ApiKey]]", AppKey: "bar"},
			wantErr: false,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor) {
				d.AssertCalled(t, "Decrypt", []string{"ENC[ApiKey]"})
			},
			resetFunc: func() { os.Unsetenv("DD_API_KEY"); os.Unsetenv("DD_APP_KEY") },
		},
		{
			name: "creds found, app key encrypted",
			setupFunc: func(d *secrets.DummyDecryptor) {
				os.Setenv("DD_API_KEY", "foo")
				os.Setenv("DD_APP_KEY", "ENC[AppKey]")
				d.On("Decrypt", []string{"ENC[AppKey]"}).Once()
			},
			want:    Creds{APIKey: "foo", AppKey: "DEC[ENC[AppKey]]"},
			wantErr: false,
			wantFunc: func(t *testing.T, d *secrets.DummyDecryptor) {
				d.AssertCalled(t, "Decrypt", []string{"ENC[AppKey]"})
			},
			resetFunc: func() { os.Unsetenv("DD_API_KEY"); os.Unsetenv("DD_APP_KEY") },
		},
		{
			name:      "creds not found",
			setupFunc: func(*secrets.DummyDecryptor) {},
			want:      Creds{},
			wantErr:   true,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func() {},
		},
		{
			name:      "app key not found",
			setupFunc: func(*secrets.DummyDecryptor) { os.Setenv("DD_API_KEY", "foo") },
			want:      Creds{},
			wantErr:   true,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func() { os.Unsetenv("DD_API_KEY") },
		},
		{
			name:      "api key not found",
			setupFunc: func(*secrets.DummyDecryptor) { os.Setenv("DD_APP_KEY", "bar") },
			want:      Creds{},
			wantErr:   true,
			wantFunc:  func(t *testing.T, d *secrets.DummyDecryptor) { d.AssertNotCalled(t, "Decrypt") },
			resetFunc: func() { os.Unsetenv("DD_APP_KEY") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decryptor := &secrets.DummyDecryptor{}
			tt.setupFunc(decryptor)
			got, err := getCredentials(decryptor)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.EqualValues(t, tt.want, got)
			tt.wantFunc(t, decryptor)
			tt.resetFunc()
		})
	}
}
