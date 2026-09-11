// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secrets

import (
	"reflect"
	"testing"
	"time"
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

func TestSecretBackend_buildPayload(t *testing.T) {
	handles := []string{"api_key", "app_key"}

	// Classic command path: only version + secrets, no SGC fields.
	legacy := (&SecretBackend{}).buildPayload(handles)
	if legacy["version"] != PayloadVersion {
		t.Errorf("legacy version = %v, want %v", legacy["version"], PayloadVersion)
	}
	for _, k := range []string{"type", "config", "secret_backend_timeout"} {
		if _, ok := legacy[k]; ok {
			t.Errorf("legacy payload must not include %q", k)
		}
	}

	// SGC path: version 1.1 plus type, config and timeout.
	sb := &SecretBackend{
		backendType:   "hashicorp.vault",
		backendConfig: map[string]any{"vault_session": map[string]any{"vault_auth_type": "kubernetes"}},
		cmdTimeout:    30 * time.Second,
	}
	sgc := sb.buildPayload(handles)
	if sgc["version"] != sgcPayloadVersion {
		t.Errorf("sgc version = %v, want %v", sgc["version"], sgcPayloadVersion)
	}
	if sgc["type"] != "hashicorp.vault" {
		t.Errorf("sgc type = %v, want hashicorp.vault", sgc["type"])
	}
	if sgc["secret_backend_timeout"] != float64(30) {
		t.Errorf("sgc secret_backend_timeout = %v, want 30", sgc["secret_backend_timeout"])
	}
	if !reflect.DeepEqual(sgc["config"], sb.backendConfig) {
		t.Errorf("sgc config = %v, want %v", sgc["config"], sb.backendConfig)
	}
}

func TestNewSecretBackend_embeddedSGC(t *testing.T) {
	defer func() {
		SetSecretBackendCommand("")
		SetSecretBackendArgs([]string{})
		SetSecretBackendType("")
		SetSecretBackendConfig(map[string]any{})
	}()

	// Backend type set without an explicit command -> use the embedded SGC binary.
	SetSecretBackendCommand("")
	SetSecretBackendArgs([]string{"--legacy-arg"})
	SetSecretBackendType("hashicorp.vault")
	SetSecretBackendConfig(map[string]any{"vault_session": map[string]any{"vault_auth_type": "kubernetes"}})
	sb := NewSecretBackend()
	if sb.cmd != defaultSGCBinaryPath {
		t.Errorf("cmd = %q, want embedded SGC path %q", sb.cmd, defaultSGCBinaryPath)
	}
	if len(sb.cmdArgs) != 0 {
		t.Errorf("embedded SGC args = %v, want none", sb.cmdArgs)
	}
	// The timeout is sent to SGC as secret_backend_timeout, so it must not be shorter
	// than SGC's own 30s default
	if sb.cmdTimeout < 30*time.Second {
		t.Errorf("cmdTimeout = %v, want >= 30s for SGC operations", sb.cmdTimeout)
	}

	// An explicit command always wins over the embedded default.
	SetSecretBackendCommand("/custom/secret-backend")
	sb = NewSecretBackend()
	if sb.cmd != "/custom/secret-backend" {
		t.Errorf("cmd = %q, want /custom/secret-backend", sb.cmd)
	}
	if !reflect.DeepEqual(sb.cmdArgs, secretBackendArgs) {
		t.Errorf("explicit command args = %v, want %v", sb.cmdArgs, secretBackendArgs)
	}
	if sb.backendType != "" || sb.backendConfig != nil {
		t.Errorf("explicit command must use the legacy payload; got type %q and config %v", sb.backendType, sb.backendConfig)
	}
	for _, key := range []string{"type", "config", "secret_backend_timeout"} {
		if _, found := sb.buildPayload([]string{"api_key"})[key]; found {
			t.Errorf("explicit command payload must not include %q", key)
		}
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
