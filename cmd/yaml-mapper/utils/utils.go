// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import "helm.sh/helm/v3/pkg/chartutil"

// GetPathVal safely traverses nested maps and retrieves the value at the given path.
// It supports map[string]interface{} and chartutil.Values.
// Returns (value, true) if found, otherwise (nil, false).
func GetPathVal(obj interface{}, keys ...string) (interface{}, bool) {
	if obj == nil {
		return nil, false
	}

	current := obj
	for _, key := range keys {
		var m map[string]interface{}
		switch typed := current.(type) {
		case map[string]interface{}:
			m = typed
		case chartutil.Values: // alias of map[string]interface{}
			m = map[string]interface{}(typed)
		default:
			return nil, false
		}

		next, exists := m[key]
		if !exists {
			return nil, false
		}
		current = next
	}
	return current, true
}

// GetPathString returns the string value at the nested path, if present.
func GetPathString(obj interface{}, keys ...string) (string, bool) {
	v, ok := GetPathVal(obj, keys...)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetPathSlice returns the []interface{} value at the nested path, if present.
func GetPathSlice(obj interface{}, keys ...string) ([]interface{}, bool) {
	v, ok := GetPathVal(obj, keys...)
	if !ok {
		return nil, false
	}
	s, ok := v.([]interface{})
	return s, ok
}

// GetPathBool returns the boolean value at the nested path, if present.
func GetPathBool(obj interface{}, keys ...string) (bool, bool) {
	val, ok := GetPathVal(obj, keys...)
	if !ok {
		return false, false
	}
	bVal, ok := val.(bool)
	return bVal, ok
}

// GetPathMap returns the map[string]interface{} value at the nested path, if present.
func GetPathMap(obj interface{}, keys ...string) (map[string]interface{}, bool) {
	v, ok := GetPathVal(obj, keys...)
	if !ok || v == nil {
		return nil, false
	}

	switch typed := v.(type) {
	case map[string]interface{}:
		return typed, true
	case chartutil.Values: // alias of map[string]interface{}
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

// GetPathInt returns the int value at the nested path, if present.
// Handles various numeric types (int, int32, int64, float64) that YAML parsers may produce.
func GetPathInt(obj interface{}, keys ...string) (int, bool) {
	v, ok := GetPathVal(obj, keys...)
	if !ok {
		return 0, false
	}
	switch typed := v.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}
