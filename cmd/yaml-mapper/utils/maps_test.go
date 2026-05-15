// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chartutil"
)

func TestInsertAtPath(t *testing.T) {
	values := map[string]any{
		"datadog": map[string]any{
			"apiKey": "existing",
		},
	}

	InsertAtPath("datadog.logs.enabled", true, values)

	require.Equal(t, "existing", values["datadog"].(map[string]any)["apiKey"])
	require.Equal(t, true, values["datadog"].(map[string]any)["logs"].(map[string]any)["enabled"])
}

func TestMergeMapDeep(t *testing.T) {
	t.Run("deep merges nested maps", func(t *testing.T) {
		left := map[string]any{
			"datadog": map[string]any{
				"apiKey": "existing",
			},
		}
		right := map[string]any{
			"datadog": map[string]any{
				"logs": map[string]any{"enabled": true},
			},
		}

		got := MergeMapDeep(left, right)

		require.Equal(t, "existing", got["datadog"].(map[string]any)["apiKey"])
		require.Equal(t, true, got["datadog"].(map[string]any)["logs"].(map[string]any)["enabled"])
	})

	t.Run("overwrites scalars and ignores nil values", func(t *testing.T) {
		got := MergeMapDeep(
			map[string]any{"logs": false, "apm": true},
			map[string]any{"logs": true, "apm": nil},
		)

		require.Equal(t, true, got["logs"])
		require.Equal(t, true, got["apm"])
	})
}

func TestMergeOrSet(t *testing.T) {
	t.Run("merges map values", func(t *testing.T) {
		values := map[string]any{
			"datadog": map[string]any{"apiKey": "existing"},
		}

		MergeOrSet(values, "datadog", chartutil.Values{"logs": map[string]any{"enabled": true}})

		require.Equal(t, "existing", values["datadog"].(map[string]any)["apiKey"])
		require.Equal(t, true, values["datadog"].(map[string]any)["logs"].(map[string]any)["enabled"])
	})

	t.Run("does not write nil values", func(t *testing.T) {
		values := map[string]any{}

		MergeOrSet(values, "datadog", nil)

		require.Empty(t, values)
	})
}

func TestGetPathHelpers(t *testing.T) {
	values := chartutil.Values{
		"datadog": map[string]any{
			"apiKey": "abc",
			"logs": map[string]any{
				"enabled": true,
				"items":   []any{"first"},
			},
		},
	}

	stringValue, ok := GetPathString(values, "datadog", "apiKey")
	require.True(t, ok)
	require.Equal(t, "abc", stringValue)

	boolValue, ok := GetPathBool(values, "datadog", "logs", "enabled")
	require.True(t, ok)
	require.True(t, boolValue)

	sliceValue, ok := GetPathSlice(values, "datadog", "logs", "items")
	require.True(t, ok)
	require.Equal(t, []any{"first"}, sliceValue)

	mapValue, ok := GetPathMap(values, "datadog", "logs")
	require.True(t, ok)
	require.Equal(t, true, mapValue["enabled"])

	_, ok = GetPathString(values, "datadog", "logs", "enabled")
	require.False(t, ok)

	_, ok = GetPathVal(values, "datadog", "missing")
	require.False(t, ok)
}

func TestApplyDeprecationRules(t *testing.T) {
	t.Run("moves deprecated boolean aliases to their standard key", func(t *testing.T) {
		values := chartutil.Values{
			"datadog": map[string]any{
				"apm": map[string]any{
					"enabled": false,
				},
			},
		}

		got := ApplyDeprecationRules(values)

		portEnabled, ok := GetPathBool(got, "datadog", "apm", "portEnabled")
		require.True(t, ok)
		require.False(t, portEnabled)

		_, ok = GetPathBool(got, "datadog", "apm", "enabled")
		require.False(t, ok)
	})

	t.Run("standard key participates in boolean OR mapping", func(t *testing.T) {
		values := chartutil.Values{
			"datadog": map[string]any{
				"apm": map[string]any{
					"enabled":     false,
					"portEnabled": true,
				},
			},
		}

		got := ApplyDeprecationRules(values)

		portEnabled, ok := GetPathBool(got, "datadog", "apm", "portEnabled")
		require.True(t, ok)
		require.True(t, portEnabled)
	})

	t.Run("negates deprecated inverse keys unless the standard key is present", func(t *testing.T) {
		values := chartutil.Values{
			"datadog": map[string]any{
				"systemProbe": map[string]any{
					"enableDefaultOsReleasePaths": false,
				},
			},
		}

		got := ApplyDeprecationRules(values)

		disableDefaultPaths, ok := GetPathBool(got, "datadog", "disableDefaultOsReleasePaths")
		require.True(t, ok)
		require.True(t, disableDefaultPaths)

		_, ok = GetPathBool(got, "datadog", "systemProbe", "enableDefaultOsReleasePaths")
		require.False(t, ok)
	})
}
