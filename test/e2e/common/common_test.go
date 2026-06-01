// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"strings"
	"testing"
)

func TestParseCollectorJson(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantKey string
	}{
		{
			name:    "clean collector status JSON",
			output:  `{"runnerStats":{"Checks":{"kubelet":{"kubelet":{"CheckName":"kubelet"}}}}}`,
			wantKey: "runnerStats",
		},
		{
			name: "skips noisy config logs before collector status JSON",
			output: `2026-06-01 14:12:12 UTC | CORE | WARN | Set('proxy.no_proxy'): converting value from []interface {} to []string to match default type
2026-06-01 14:12:13 UTC | CORE | WARN | dac.dataset:teeconfig_config_diffs | item secret_backend_config: map[string]interface {}{"vault_session":map[interface {}]interface {}{"vault_auth_type":interface {}(nil)}} | map[string]interface {}{"vault_session":map[string]interface {}{"vault_auth_type":interface {}(nil)}}
{}
{"runnerStats":{"Checks":{"kubelet":{"kubelet":{"CheckName":"kubelet"}}}}}`,
			wantKey: "runnerStats",
		},
		{
			name:    "logs status JSON",
			output:  "2026-06-01 14:12:12 UTC | CORE | INFO | startup log\n" + `{"logsStats":{"integrations":[]}}`,
			wantKey: "logsStats",
		},
		{
			name:    "apm status JSON",
			output:  "2026-06-01 14:12:12 UTC | CORE | INFO | startup log\n" + `{"apmStats":{"receiver":[]}}`,
			wantKey: "apmStats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCollectorJson(tt.output)
			if _, ok := got[tt.wantKey]; !ok {
				t.Fatalf("ParseCollectorJson() missing key %q in %#v", tt.wantKey, got)
			}
		})
	}
}

func TestParseCollectorJsonNoStatusJSON(t *testing.T) {
	got, diagnostics := ParseCollectorJsonWithDiagnostics(`2026-06-01 14:12:12 UTC | CORE | WARN | no status JSON here: []interface {}`)
	if len(got) != 0 {
		t.Fatalf("ParseCollectorJson() = %#v, want empty map", got)
	}
	if !strings.Contains(diagnostics, "no Agent status JSON found") {
		t.Fatalf("ParseCollectorJsonWithDiagnostics() diagnostics = %q, want parse failure detail", diagnostics)
	}
}
