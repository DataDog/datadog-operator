// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

// monitorDefinitionFields are the monitor fields that creates and updates set.
// State, timestamps and other server-managed fields are excluded so that only
// edits to the definition change the hash.
var monitorDefinitionFields = []string{
	"name", "type", "query", "message", "tags", "options", "priority", "restricted_roles", "draft_status",
}

// MonitorDefinitionHash returns a hash of the definition of a monitor returned
// by the Datadog API. A write response and a later read of the same definition
// hash the same, so the hash recorded after a write detects out-of-band edits.
// extraFields adds fields, such as "assets", that the caller reads on every
// request that it hashes.
func MonitorDefinitionHash(monitor datadogV1.Monitor, extraFields ...string) (string, error) {
	data, err := json.Marshal(monitor)
	if err != nil {
		return "", err
	}
	var fields map[string]any
	if err = json.Unmarshal(data, &fields); err != nil {
		return "", err
	}

	definition := make(map[string]any, len(monitorDefinitionFields)+len(extraFields))
	for _, field := range slices.Concat(monitorDefinitionFields, extraFields) {
		definition[field] = fields[field]
	}
	// These lists are sets, and Datadog does not always return them in the same
	// order: restricted roles come back as sent on create but in another order
	// on reads.
	sortStrings(definition["tags"])
	sortStrings(definition["restricted_roles"])
	if options, ok := definition["options"].(map[string]any); ok {
		sortStrings(options["notify_by"])
		sortStrings(options["renotify_statuses"])
	}
	// Write responses repeat the monitor ID in each asset; reads do not.
	if assets, ok := definition["assets"].([]any); ok {
		for _, asset := range assets {
			if fields, ok := asset.(map[string]any); ok {
				delete(fields, "monitor_id")
			}
		}
	}
	return comparison.GenerateMD5ForSpec(definition)
}

// sortStrings sorts a decoded JSON list of strings in place.
func sortStrings(list any) {
	if values, ok := list.([]any); ok {
		slices.SortFunc(values, func(a, b any) int {
			as, _ := a.(string)
			bs, _ := b.(string)
			return strings.Compare(as, bs)
		})
	}
}
