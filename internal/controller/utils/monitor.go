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

// MonitorMatchesSpec ignores fields absent from desired, including defaults and
// read-only metadata returned by Datadog. desired must describe the supplied spec,
// without API-client constructor defaults.
func MonitorMatchesSpec(desired any, remote datadogV1.Monitor) (bool, error) {
	return comparison.ManagedFieldsMatch(desired, remote, normalizeMonitorField)
}

func normalizeMonitorField(path string, value any) any {
	switch path {
	case "priority":
		// Datadog canonicalizes an explicit zero (clear priority) to null.
		if n, ok := value.(json.Number); ok && n.String() == "0" {
			return nil
		}
	case "type":
		if value == string(datadogV1.MONITORTYPE_METRIC_ALERT) {
			return string(datadogV1.MONITORTYPE_QUERY_ALERT)
		}
	case "message", "query", "options.escalation_message":
		if s, ok := value.(string); ok {
			return strings.TrimSpace(s)
		}
	case "tags", "restricted_roles", "options.notify_by", "options.renotify_statuses":
		// These lists are sets; Datadog may return a different order or null
		// for an empty set. Never reorder other lists, such as recurrences.
		if value == nil {
			return []any{}
		}
		if values, ok := value.([]any); ok {
			items := make([]string, 0, len(values))
			for _, item := range values {
				s, ok := item.(string)
				if !ok {
					return value
				}
				items = append(items, s)
			}
			slices.Sort(items)
			items = slices.Compact(items)
			result := make([]any, len(items))
			for i, item := range items {
				result[i] = item
			}
			return result
		}
	}
	return value
}
