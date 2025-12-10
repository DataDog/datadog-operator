// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"strings"

	"helm.sh/helm/v3/pkg/chartutil"
)

// InsertAtPath inserts a value into a nested map at a dotted path, creating
// intermediate maps as needed and deep-merging with existing keys.
// It returns the updated destination map.
func InsertAtPath(path string, val interface{}, destMap map[string]interface{}) map[string]interface{} {
	parts := strings.Split(path, ".")
	res := make(map[string]interface{})
	if len(parts) > 0 {
		// create innermost map using the input value
		res[parts[len(parts)-1]] = val
		// iterate backwards, skipping the last element (starting from i=1)
		for i := 1; i <= len(parts)-1; i++ {
			p := parts[len(parts)-(i+1)]
			// `t` is a placeholder map to carry over submaps between iterations
			t := res
			res = make(map[string]interface{})
			res[p] = t
		}
	}

	MergeMapDeep(destMap, res)

	return destMap
}

// MergeMapDeep recursively merges two maps, with values from map2 taking precedence over map1.
// It handles nil maps and type assertions safely.
// Inspired by: https://stackoverflow.com/a/60420264
func MergeMapDeep(map1, map2 map[string]interface{}) map[string]interface{} {
	if map1 == nil {
		map1 = make(map[string]interface{})
	}
	if map2 == nil {
		return map1
	}

	for key, rightVal := range map2 {
		if rightVal == nil {
			continue
		}

		leftVal, exists := map1[key]
		if !exists {
			// Key doesn't exist in map1, add it
			map1[key] = rightVal
			continue
		}

		// Both values are maps, merge them recursively
		leftMap, leftIsMap := GetPathMap(leftVal)
		rightMap, rightIsMap := GetPathMap(rightVal)

		if leftIsMap && rightIsMap {
			map1[key] = MergeMapDeep(leftMap, rightMap)
		} else {
			map1[key] = rightVal
		}
	}

	return map1
}

// MergeOrSet sets a key in the interim map. If both the existing and new values are maps,
// it deep-merges them instead of overwriting. Otherwise, it overwrites.
func MergeOrSet(interim map[string]interface{}, key string, val interface{}) {
	if val == nil {
		return
	}
	if existing, exists := interim[key]; exists {
		if left, lok := asMap(existing); lok {
			if right, rok := asMap(val); rok {
				interim[key] = MergeMapDeep(left, right)
				return
			}
		}
	}
	interim[key] = val
}

// asMap tries to coerce supported map-like types into map[string]interface{}.
func asMap(v interface{}) (map[string]interface{}, bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		return t, true
	case chartutil.Values:
		return map[string]interface{}(t), true
	default:
		return nil, false
	}
}

// removeAtPath deletes the value from the map object at the period-delimited path string
func removeAtPath(root map[string]interface{}, dotted string) {
	parts := strings.Split(dotted, ".")
	if len(parts) == 0 {
		return
	}

	m := root
	for i := 0; i < len(parts)-1; i++ {
		next, ok := GetPathMap(m[parts[i]])
		if !ok {
			// Path doesn’t exist — nothing to delete
			return
		}
		m = next
	}

	delete(m, parts[len(parts)-1])
}
