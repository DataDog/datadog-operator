// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package secrets

import (
	"reflect"
	"testing"
)

func TestSecretBackend_execCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		payload string
		want    []byte
		wantErr bool
	}{
		{
			name:    "nominal case",
			cmd:     "./testdata/decryptor/dummy_decryptor.py",
			payload: "{\"version\": \"1\", \"secrets\": [\"api_key\", \"app_key\"]}",
			want:    []byte("{\"api_key\": {\"value\": \"decrypted_api_key\"}, \"app_key\": {\"value\": \"decrypted_app_key\"}}"),
			wantErr: false,
		},
		{
			name:    "secret backend returns error",
			cmd:     "./testdata/notfound/decryptor",
			payload: "{\"version\": \"1\", \"secrets\": [\"api_key\", \"app_key\"]}",
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := &SecretBackend{
				cmd:              tt.cmd,
				cmdTimeout:       defaultCmdTimeout,
				cmdOutputMaxSize: defaultCmdOutputMaxSize,
			}
			got, err := sb.execCommand(tt.payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("SecretBackend.execCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SecretBackend.execCommand() = %v, want %v", string(got), string(tt.want))
			}
		})
	}
}

func TestSecretBackend_Decrypt(t *testing.T) {
	tests := []struct {
		name      string
		cmd       string
		encrypted []string
		want      map[string]string
		wantErr   bool
	}{
		{
			name: "exec secret backend command",
			cmd:  "./testdata/decryptor/dummy_decryptor.py",
			encrypted: []string{
				"ENC[api_key]",
				"ENC[app_key]",
			},
			want: map[string]string{
				"ENC[api_key]": "decrypted_api_key",
				"ENC[app_key]": "decrypted_app_key",
			},
			wantErr: false,
		},
		{
			name: "secret backend command cannot decrypt",
			cmd:  "./testdata/decryptor/dummy_decryptor.py",
			encrypted: []string{
				"ENC[api_key_error]",
				"ENC[app_key]",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "secret backend command not set",
			cmd:  "",
			encrypted: []string{
				"ENC[api_key]",
				"ENC[app_key]",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "secret backend binary not found",
			cmd:  "./testdata/notfound/dummy_decryptor.py",
			encrypted: []string{
				"ENC[api_key]",
				"ENC[app_key]",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := &SecretBackend{
				cmd:              tt.cmd,
				cmdTimeout:       defaultCmdTimeout,
				cmdOutputMaxSize: defaultCmdOutputMaxSize,
			}
			got, err := sb.Decrypt(tt.encrypted)
			if (err != nil) != tt.wantErr {
				t.Errorf("SecretBackend.Decrypt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SecretBackend.Decrypt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecretBackend_fetchSecret(t *testing.T) {
	tests := []struct {
		name      string
		cmd       string
		encrypted []string
		want      map[string]string
		wantErr   bool
	}{
		{
			name: "nominal case",
			cmd:  "./testdata/decryptor/dummy_decryptor.py",
			encrypted: []string{
				"ENC[api_key]",
				"ENC[app_key]",
			},
			want: map[string]string{
				"ENC[api_key]": "decrypted_api_key",
				"ENC[app_key]": "decrypted_app_key",
			},
			wantErr: false,
		},
		{
			name: "error decrypting a secret",
			cmd:  "./testdata/decryptor/dummy_decryptor.py",
			encrypted: []string{
				"ENC[api_key]",
				"ENC[app_key_error]",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "secret not found",
			cmd:  "./testdata/decryptor/dummy_decryptor.py",
			encrypted: []string{
				"ENC[api_key]",
				"ENC[app_key_ignore]",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := &SecretBackend{
				cmd:              tt.cmd,
				cmdTimeout:       defaultCmdTimeout,
				cmdOutputMaxSize: defaultCmdOutputMaxSize,
			}
			got, err := sb.fetchSecret(tt.encrypted)
			if (err != nil) != tt.wantErr {
				t.Errorf("SecretBackend.fetchSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SecretBackend.fetchSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}
