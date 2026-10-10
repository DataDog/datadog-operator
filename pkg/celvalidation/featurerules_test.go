// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package celvalidation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestFeatureRulesFire checks that each registered feature rule rejects what it
// is for and accepts what it should.
//
// The safeguards elsewhere check that rules compile, answer and stay cheap.
// None of them would notice a rule that is simply never true, which is the
// easier mistake to make: a mistyped path under a has() guard holds vacuously
// on every object.
func TestFeatureRulesFire(t *testing.T) {
	rules := ddaRules(t)

	dda := func(spec, annotations map[string]any) *unstructured.Unstructured {
		metadata := map[string]any{"name": "dda", "namespace": "ns"}
		if annotations != nil {
			metadata["annotations"] = annotations
		}
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "datadoghq.com/v2alpha1",
			"kind":       "DatadogAgent",
			"metadata":   metadata,
			"spec":       spec,
		}}
	}
	credentials := map[string]any{
		"global": map[string]any{"credentials": map[string]any{"apiKey": "k"}},
	}
	withOTLPEndpoint := func(endpoint string) map[string]any {
		return map[string]any{
			"global": credentials["global"],
			"features": map[string]any{"otlp": map[string]any{"receiver": map[string]any{
				"protocols": map[string]any{"grpc": map[string]any{"endpoint": endpoint}}}}},
		}
	}

	cases := []struct {
		name    string
		object  *unstructured.Unstructured
		wantErr string
	}{
		{
			name:    "otlp unix endpoint",
			object:  dda(withOTLPEndpoint("unix:///var/run/otlp.sock"), nil),
			wantErr: "unix",
		},
		{
			name:    "otlp unix-abstract endpoint",
			object:  dda(withOTLPEndpoint("unix-abstract:otlp"), nil),
			wantErr: "unix",
		},
		{name: "otlp host:port endpoint", object: dda(withOTLPEndpoint("0.0.0.0:4317"), nil)},
		{name: "otlp endpoint unset", object: dda(credentials, nil)},
		{
			name: "host profiler seccomp annotation is not a bool",
			object: dda(credentials, map[string]any{
				"agent.datadoghq.com/host-profiler-seccomp-enabled": "yes"}),
			wantErr: "host-profiler-seccomp-enabled",
		},
		{
			name: "host profiler seccomp annotation parses as a bool",
			object: dda(credentials, map[string]any{
				"agent.datadoghq.com/host-profiler-seccomp-enabled": "1"}),
		},
		{
			name: "enable annotation mis-cased",
			object: dda(credentials, map[string]any{
				"agent.datadoghq.com/host-profiler-enabled": "True"}),
			wantErr: "host-profiler-enabled",
		},
		{
			name: "enable annotation exactly true",
			object: dda(credentials, map[string]any{
				"agent.datadoghq.com/host-profiler-enabled": "true"}),
		},
		{name: "no annotations at all", object: dda(credentials, nil)},
		// One per feature package that registered rules in the sweep, so a
		// package dropped from the blank-import list in rules_test.go shows up
		// here rather than silently losing its rules.
		{
			name:    "adp annotation mis-cased",
			object:  dda(credentials, map[string]any{"agent.datadoghq.com/adp-enabled": "True"}),
			wantErr: "adp-enabled",
		},
		{
			name:    "private action runner annotation mis-cased",
			object:  dda(credentials, map[string]any{"agent.datadoghq.com/private-action-runner-enabled": "yes"}),
			wantErr: "private-action-runner-enabled",
		},
		{
			name:    "ksm cache annotation mis-cased",
			object:  dda(credentials, map[string]any{"agent.datadoghq.com/ksm-use-apiserver-cache": "TRUE"}),
			wantErr: "ksm-use-apiserver-cache",
		},
		{
			name:    "flight recorder annotation mis-cased",
			object:  dda(credentials, map[string]any{"agent.datadoghq.com/flightrecorder-enabled": "1"}),
			wantErr: "flightrecorder-enabled",
		},
		{
			name:    "network crds annotation mis-cased",
			object:  dda(credentials, map[string]any{"agent.datadoghq.com/network-crds-enabled": "False "}),
			wantErr: "network-crds-enabled",
		},
		{
			name:    "instrumentation crd annotation mis-cased",
			object:  dda(credentials, map[string]any{"agent.datadoghq.com/instrumentation-crd-enabled": "t"}),
			wantErr: "instrumentation-crd-enabled",
		},
		{
			name:    "check runner annotation mis-cased",
			object:  dda(credentials, map[string]any{"agent.datadoghq.com/check-runner-enabled": "on"}),
			wantErr: "check-runner-enabled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := rules.ValidateObject(context.Background(), tc.object, "ns", "dda")
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
