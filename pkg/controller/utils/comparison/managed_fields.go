// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package comparison

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
)

// ManagedFieldsOptions customizes how ManagedFieldsMatch canonicalizes values.
//
// Paths are dot-separated JSON keys from the document root. List elements add
// a "[]" suffix to the path of their list, for example "tags[]" or
// "options.scheduling_options.custom_schedule.recurrences[].rrule".
type ManagedFieldsOptions struct {
	// UnorderedLists contains the paths of lists whose element order and
	// duplicates are not significant. They are compared as sets.
	UnorderedLists []string
	// Normalize, when set, returns the canonical form of the value at path.
	// It is applied to the desired and actual values alike, before they are
	// compared, so it should only fold together representations that a
	// remote API is known to treat as equivalent.
	Normalize func(path string, value any) any
}

// ManagedFieldsMatch reports whether every field present in desired has an
// equivalent value in actual. Both values are compared through their JSON
// representation.
//
// Only fields present in desired are compared, recursively: fields that only
// appear in actual are ignored because remote APIs commonly populate defaults
// for fields a request omitted. Within the compared fields, null, empty
// strings, empty lists and empty objects are equivalent, and lists must have
// the same elements, in the same order unless their path is listed in
// opts.UnorderedLists.
//
// Callers should treat a false result as "possibly different": the comparison
// only folds together representations it was told are equivalent.
func ManagedFieldsMatch(desired, actual any, opts ManagedFieldsOptions) (bool, error) {
	desiredValue, err := toJSONValue(desired)
	if err != nil {
		return false, err
	}
	actualValue, err := toJSONValue(actual)
	if err != nil {
		return false, err
	}

	m := managedFieldsMatcher{
		unorderedLists: make(map[string]bool, len(opts.UnorderedLists)),
		normalize:      opts.Normalize,
	}
	for _, path := range opts.UnorderedLists {
		m.unorderedLists[path] = true
	}

	return m.match("", desiredValue, actualValue), nil
}

type managedFieldsMatcher struct {
	unorderedLists map[string]bool
	normalize      func(path string, value any) any
}

func (m managedFieldsMatcher) match(path string, desired, actual any) bool {
	if m.normalize != nil {
		desired = m.normalize(path, desired)
		actual = m.normalize(path, actual)
	}

	switch desiredValue := desired.(type) {
	case map[string]any:
		actualValue, ok := actual.(map[string]any)
		if !ok && !isEmptyJSONValue(actual) {
			return false
		}
		for key, desiredField := range desiredValue {
			if !m.match(joinPath(path, key), desiredField, actualValue[key]) {
				return false
			}
		}
		return true

	case []any:
		actualValue, ok := actual.([]any)
		if !ok && !isEmptyJSONValue(actual) {
			return false
		}
		elementPath := path + "[]"
		if m.unorderedLists[path] {
			desiredValue = m.canonicalSet(elementPath, desiredValue)
			actualValue = m.canonicalSet(elementPath, actualValue)
		}
		if len(desiredValue) != len(actualValue) {
			return false
		}
		for i := range desiredValue {
			if !m.match(elementPath, desiredValue[i], actualValue[i]) {
				return false
			}
		}
		return true
	}

	if isEmptyJSONValue(desired) || isEmptyJSONValue(actual) {
		return isEmptyJSONValue(desired) && isEmptyJSONValue(actual)
	}
	return reflect.DeepEqual(desired, actual)
}

// canonicalSet normalizes, sorts and deduplicates list elements so that two
// lists holding the same set of values compare equal element by element.
func (m managedFieldsMatcher) canonicalSet(elementPath string, values []any) []any {
	type keyedValue struct {
		key   string
		value any
	}

	keyed := make([]keyedValue, 0, len(values))
	for _, value := range values {
		if m.normalize != nil {
			value = m.normalize(elementPath, value)
		}
		key, _ := json.Marshal(value) //nolint:errchkjson // Values decoded by encoding/json cannot fail JSON marshaling.
		keyed = append(keyed, keyedValue{key: string(key), value: value})
	}
	slices.SortFunc(keyed, func(a, b keyedValue) int { return strings.Compare(a.key, b.key) })
	keyed = slices.CompactFunc(keyed, func(a, b keyedValue) bool { return a.key == b.key })

	canonical := make([]any, 0, len(keyed))
	for _, kv := range keyed {
		canonical = append(canonical, kv.value)
	}
	return canonical
}

func toJSONValue(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(b, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func isEmptyJSONValue(v any) bool {
	switch value := v.(type) {
	case nil:
		return true
	case string:
		return value == ""
	case []any:
		return len(value) == 0
	case map[string]any:
		return len(value) == 0
	default:
		return false
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
