// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mapper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chartutil"
)

// crdSchemaPath is the v2alpha1 DatadogAgent OpenAPI schema, relative to the repo root.
const crdSchemaRelPath = "config/crd/bases/v1/datadoghq.com_datadogagents_v2alpha1.json"

// TestMappingTargetsExistInCRD guards against mapping rot: every destination path in the
// embedded mapping file must resolve to a real field in the current DatadogAgent v2alpha1
// CRD schema. A mapping whose target has been renamed or removed from the CRD (as happened
// with spec.features.serviceDiscovery.networkStats.enabled) silently produces a DDA the
// operator ignores; this test turns that into a build failure.
func TestMappingTargetsExistInCRD(t *testing.T) {
	spec := loadCRDSpecSchema(t)

	mapping, err := chartutil.ReadValues(defaultDDAMap)
	require.NoError(t, err, "failed to parse embedded mapping file")

	var dead []string
	for helmKey, raw := range mapping {
		for _, target := range mappingTargets(raw) {
			// Only spec.* targets are validated against the CRD spec schema.
			// metadata.* and other targets are handled by the mapper directly.
			if !strings.HasPrefix(target, "spec.") {
				continue
			}
			segs := strings.Split(target, ".")[1:]
			if !schemaHasPath(spec, segs) {
				dead = append(dead, helmKey+" -> "+target)
			}
		}
	}

	sort.Strings(dead)
	require.Empty(t, dead,
		"mapping targets not found in DatadogAgent v2alpha1 CRD schema (rename/removal in the CRD, "+
			"or a typo in the mapping). Fix the target or set the mapping to \"\":\n  %s",
		strings.Join(dead, "\n  "))
}

// mappingTargets extracts the CRD destination path(s) from a single mapping entry value,
// which may be a direct string, a fan-out list of strings, or a map with a "newPath".
func mappingTargets(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case map[string]any:
		// Collect newPath plus any spec.* target embedded in args (e.g. parentPath, keyNamePath).
		var out []string
		if np, ok := v["newPath"].(string); ok && np != "" {
			out = append(out, np)
		}
		out = append(out, specTargetsFromArgs(v["args"])...)
		return out
	}
	return nil
}

// specTargetsFromArgs recursively collects any string starting with "spec." from a mapFunc's args,
// so arg-embedded destination paths (parentPath, keyNamePath, …) are validated against the CRD too.
func specTargetsFromArgs(v any) []string {
	switch t := v.(type) {
	case string:
		if strings.HasPrefix(t, "spec.") {
			return []string{t}
		}
	case []any:
		var out []string
		for _, e := range t {
			out = append(out, specTargetsFromArgs(e)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, e := range t {
			out = append(out, specTargetsFromArgs(e)...)
		}
		return out
	}
	return nil
}

// schemaHasPath reports whether the dotted path segments resolve to a node in the OpenAPI
// schema. It descends through object properties, map values (additionalProperties), and
// array items, and accepts any path under an x-kubernetes-preserve-unknown-fields subtree.
func schemaHasPath(node map[string]any, segs []string) bool {
	cur := node
	for _, seg := range segs {
		if props, ok := mapField(cur, "properties"); ok {
			if next, ok := mapField(props, seg); ok {
				cur = next
				continue
			}
		}
		if ap, ok := mapField(cur, "additionalProperties"); ok {
			cur = ap
			continue
		}
		if items, ok := mapField(cur, "items"); ok {
			if props, ok := mapField(items, "properties"); ok {
				if next, ok := mapField(props, seg); ok {
					cur = next
					continue
				}
			}
			if ap, ok := mapField(items, "additionalProperties"); ok {
				cur = ap
				continue
			}
		}
		if preserve, ok := cur["x-kubernetes-preserve-unknown-fields"].(bool); ok && preserve {
			return true
		}
		return false
	}
	return true
}

func mapField(m map[string]any, key string) (map[string]any, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	sub, ok := v.(map[string]any)
	return sub, ok
}

// loadCRDSpecSchema loads the .spec sub-schema of the DatadogAgent v2alpha1 CRD.
func loadCRDSpecSchema(t *testing.T) map[string]any {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not determine test file path")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	schemaPath := filepath.Join(repoRoot, crdSchemaRelPath)

	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err, "failed to read CRD schema at %s", schemaPath)

	var root map[string]any
	require.NoError(t, json.Unmarshal(data, &root), "failed to parse CRD schema JSON")

	props, ok := mapField(root, "properties")
	require.True(t, ok, "CRD schema missing properties")
	spec, ok := mapField(props, "spec")
	require.True(t, ok, "CRD schema missing spec")
	return spec
}
