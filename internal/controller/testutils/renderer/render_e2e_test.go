// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// baseKindCounts is the expected resource inventory for a minimal DatadogAgent
// with no extra features enabled. The three Services are the Cluster Agent,
// the admission controller, and the node Agent local service (rendered because
// the simulated Kubernetes version is >= 1.22).
var baseKindCounts = map[string]int{
	"ServiceAccount":       2,
	"ClusterRole":          5,
	"ClusterRoleBinding":   5,
	"Role":                 1,
	"RoleBinding":          1,
	"Secret":               1,
	"ConfigMap":            5,
	"Service":              3,
	"DatadogAgentInternal": 1,
	"DaemonSet":            1,
	"Deployment":           1,
}

// baseKindSeq is the expected serialized kind order for the same input,
// reflecting the dependency-aware sort in kindOrder.
var baseKindSeq = []string{
	"ServiceAccount", "ServiceAccount",
	"ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole",
	"ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding",
	"Role",
	"RoleBinding",
	"Secret",
	"ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap",
	"Service", "Service", "Service",
	"DatadogAgentInternal",
	"DaemonSet",
	"Deployment",
}

// TestRender_MinimalDDA is the primary e2e test. It loads a real DatadogAgent
// manifest, runs both reconciliation passes, and asserts:
//   - exact kind inventory
//   - full serialized kind order (dependency-aware + alphabetical within kind)
//   - dynamic metadata fields are stripped from serialized output
func TestRender_MinimalDDA(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)

	objects, scheme, err := Render(Options{DDA: dda})
	require.NoError(t, err)

	assert.Equal(t, baseKindCounts, countKinds(objects, scheme))

	out, err := Serialize(objects, scheme, "yaml", false)
	require.NoError(t, err)
	s := string(out)

	assert.Equal(t, baseKindSeq, kindSequence(s))

	for _, banned := range []string{"resourceVersion:", "generation:", "creationTimestamp:", "managedFields:"} {
		assert.NotContains(t, s, banned, "field %q must be stripped", banned)
	}
}

// TestRender_WithDAP exercises the ProfileEnabled code path with two valid profiles.
// Each profile produces its own DDAI and DaemonSet, so the output has 3 DDAIs
// (default + linux-profile + gpu-profile) and 3 DaemonSets.
func TestRender_WithDAP(t *testing.T) {
	dda, err := LoadDDA("testdata/minimal-dda.yaml")
	require.NoError(t, err)

	daps, err := LoadDAPs([]string{"testdata/linux-profile.yaml", "testdata/gpu-profile.yaml"})
	require.NoError(t, err)

	objects, scheme, err := Render(Options{
		DDA:            dda,
		DAPs:           daps,
		ProfileEnabled: true,
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]int{
		"ServiceAccount":       2,
		"ClusterRole":          5,
		"ClusterRoleBinding":   5,
		"Role":                 1,
		"RoleBinding":          1,
		"Secret":               1,
		"ConfigMap":            5,
		"Service":              3,
		"DatadogAgentInternal": 3,
		"DaemonSet":            3,
		"Deployment":           1,
	}, countKinds(objects, scheme))

	out, err := Serialize(objects, scheme, "yaml", false)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"ServiceAccount", "ServiceAccount",
		"ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole", "ClusterRole",
		"ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding", "ClusterRoleBinding",
		"Role",
		"RoleBinding",
		"Secret",
		"ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap", "ConfigMap",
		"Service", "Service", "Service",
		"DatadogAgentInternal", "DatadogAgentInternal", "DatadogAgentInternal",
		"DaemonSet", "DaemonSet", "DaemonSet",
		"Deployment",
	}, kindSequence(string(out)))
}

// kindSequence extracts the ordered list of "kind: X" values from serialized YAML.
func kindSequence(yaml string) []string {
	var kinds []string
	for line := range strings.SplitSeq(yaml, "\n") {
		if kind, ok := strings.CutPrefix(line, "kind: "); ok {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// countKinds tallies each GVK Kind in the object slice.
func countKinds(objects []client.Object, scheme *runtime.Scheme) map[string]int {
	counts := map[string]int{}
	for _, obj := range objects {
		counts[resolveKind(obj, scheme)]++
	}
	return counts
}
