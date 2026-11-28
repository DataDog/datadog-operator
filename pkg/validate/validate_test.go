// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package validate

import (
	"strings"
	"testing"
)

func TestParseAndValidateBytes(t *testing.T) {
	tests := []struct {
		name        string
		manifest    string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid manifest",
			manifest: `apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog-demo
spec:
  global:
    credentials:
      apiKey: key
      appKey: appkey
    commonLabels:
      team: platform
`,
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: `apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  global:
    credentials:
      apiKey: key
`,
			wantErr:     true,
			errContains: "no metadata.name",
		},
		{
			name: "missing credentials fails semantic validation",
			manifest: `apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog-demo
spec:
  global:
    clusterName: demo
`,
			wantErr:     true,
			errContains: "credentials not configured",
		},
		{
			name: "reserved commonLabels prefix rejected",
			manifest: `apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog-demo
spec:
  global:
    credentials:
      apiKey: key
    commonLabels:
      agent.datadoghq.com/foo: bar
`,
			wantErr:     true,
			errContains: "reserved key",
		},
		{
			name: "unknown field rejected by strict decoding",
			manifest: `apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog-demo
spec:
  global:
    credentials:
      apiKey: key
    thisFieldDoesNotExist: true
`,
			wantErr:     true,
			errContains: "parsing DatadogAgent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda, err := ParseAndValidateBytes([]byte(tt.manifest))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if dda == nil {
				t.Fatalf("expected non-nil DatadogAgent")
			}
			if dda.Name != "datadog-demo" {
				t.Fatalf("expected name datadog-demo, got %q", dda.Name)
			}
		})
	}
}

func TestParseAndValidateFile(t *testing.T) {
	_, err := ParseAndValidateFile("testdata/valid_datadogagent.yaml")
	if err != nil {
		t.Fatalf("unexpected error validating demo manifest: %v", err)
	}
}

func TestParseAndValidateFile_NotFound(t *testing.T) {
	_, err := ParseAndValidateFile("testdata/does_not_exist.yaml")
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "reading") {
		t.Fatalf("expected reading error, got %q", err.Error())
	}
}
