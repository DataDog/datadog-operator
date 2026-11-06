// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package comparison

import (
	"bytes"
	"encoding/json"
	"math/big"
	"reflect"
)

// ManagedFieldsMatch compares only the JSON fields present in desired, recursively.
// Fields added by the server are ignored. Lists must have the same length and
// order, but objects within them are also compared using only desired fields.
// An explicit null, false, zero or empty list is still compared.
// normalize can canonicalize API-specific representations on both sides. Paths
// use dot-separated JSON keys, with [] for list elements.
func ManagedFieldsMatch(desired, actual any, normalize func(path string, value any) any) (bool, error) {
	desiredJSON, err := jsonValue(desired)
	if err != nil {
		return false, err
	}
	actualJSON, err := jsonValue(actual)
	if err != nil {
		return false, err
	}
	return managedFieldsMatch("", desiredJSON, actualJSON, normalize), nil
}

func jsonValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	err = decoder.Decode(&result)
	return result, err
}

func managedFieldsMatch(path string, desired, actual any, normalize func(string, any) any) bool {
	if normalize != nil {
		desired = normalize(path, desired)
		actual = normalize(path, actual)
	}
	switch desiredValue := desired.(type) {
	case map[string]any:
		actualValue, ok := actual.(map[string]any)
		if !ok && actual != nil {
			return false
		}
		for key, value := range desiredValue {
			fieldPath := key
			if path != "" {
				fieldPath = path + "." + key
			}
			if !managedFieldsMatch(fieldPath, value, actualValue[key], normalize) {
				return false
			}
		}
		return true
	case []any:
		actualValue, ok := actual.([]any)
		if !ok || len(desiredValue) != len(actualValue) {
			return false
		}
		for i, value := range desiredValue {
			if !managedFieldsMatch(path+"[]", value, actualValue[i], normalize) {
				return false
			}
		}
		return true
	case json.Number:
		actualValue, ok := actual.(json.Number)
		if !ok {
			return false
		}
		// Preserve integer precision while treating 80 and 80.0 as equal.
		d, desiredOK := new(big.Rat).SetString(string(desiredValue))
		a, actualOK := new(big.Rat).SetString(string(actualValue))
		return desiredOK && actualOK && d.Cmp(a) == 0
	default:
		return reflect.DeepEqual(desired, actual)
	}
}
