// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissionpolicy

import (
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestRenderPolicyYAML prints the policy and binding as they would be applied.
// It exists so the manifests can be reviewed or applied by hand without running
// the operator:
//
//	go test ./pkg/admissionpolicy/ -run TestRenderPolicyYAML -v
func TestRenderPolicyYAML(t *testing.T) {
	for _, target := range Targets() {
		for _, obj := range []any{BuildPolicy(target), BuildBinding(target)} {
			b, err := yaml.Marshal(obj)
			require.NoError(t, err)
			t.Logf("\n---\n%s", string(b))
		}
	}
}
