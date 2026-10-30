// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

var monitorManagedFieldsOptions = comparison.ManagedFieldsOptions{
	UnorderedLists: []string{
		"tags",
		"restricted_roles",
		"options.notify_by",
		"options.renotify_statuses",
	},
	Normalize: normalizeMonitorField,
}

// MonitorMatchesUpdateRequest reports whether the remote monitor already holds
// every field that sending desired as a monitor update would set, so that the
// update can be skipped. Fields the request omits are ignored, because Datadog
// fills in defaults for them.
func MonitorMatchesUpdateRequest(desired datadogV1.MonitorUpdateRequest, remote datadogV1.Monitor) (bool, error) {
	return comparison.ManagedFieldsMatch(desired, remote, monitorManagedFieldsOptions)
}

// normalizeMonitorField folds together monitor field representations that
// Datadog treats as equivalent.
func normalizeMonitorField(path string, value any) any {
	switch path {
	case "type":
		// "metric alert" and "query alert" are interchangeable, and Datadog may
		// report a monitor created with one as the other.
		if value == string(datadogV1.MONITORTYPE_METRIC_ALERT) {
			return string(datadogV1.MONITORTYPE_QUERY_ALERT)
		}
	case "draft_status":
		// The API client fills in the documented default of update requests,
		// so a monitor that does not report a draft status is published.
		if value == nil {
			return string(datadogV1.MONITORDRAFTSTATUS_PUBLISHED)
		}
	case "priority":
		// The DatadogMonitor controller always sends a priority and uses 0 when
		// the spec leaves it unset; Datadog reports an unset priority as null.
		if value == float64(0) {
			return nil
		}
	case "message", "query", "options.escalation_message":
		// Surrounding whitespace is not significant in these fields, and YAML
		// block scalars commonly add a trailing newline.
		if s, ok := value.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return value
}
