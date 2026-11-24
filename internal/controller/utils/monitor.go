// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

// MonitorRead is the result of a conditional monitor read.
type MonitorRead struct {
	// Monitor is the monitor returned by the API. It is empty when NotModified.
	Monitor datadogV1.Monitor
	// ETag identifies the response body: Datadog computes it from the whole
	// body, so it changes with the monitor state and the query parameters too.
	ETag string
	// NotModified reports a 304: the response would be the one that the sent
	// ETag was read from.
	NotModified bool
}

// GetMonitorIfChanged reads a monitor, sending etag in If-None-Match when it is
// not empty. A 304 is returned as NotModified rather than as an error. The
// caller closes the response body. The client must come from
// datadogclient.NewAPIClient or use datadogclient.NewETagTransport, or the
// ETag is not sent.
func GetMonitorIfChanged(auth context.Context, client *datadogV1.MonitorsApi, monitorID int64, etag string, params datadogV1.GetMonitorOptionalParameters) (MonitorRead, *http.Response, error) {
	m, httpResp, err := client.GetMonitor(datadogclient.WithIfNoneMatch(auth, etag), monitorID, params)
	if etag != "" && httpResp != nil && httpResp.StatusCode == http.StatusNotModified {
		return MonitorRead{ETag: etag, NotModified: true}, httpResp, nil
	}
	if err != nil {
		return MonitorRead{}, httpResp, err
	}
	return MonitorRead{Monitor: m, ETag: httpResp.Header.Get("ETag")}, httpResp, nil
}

// MonitorDefinitionFields are the top-level monitor fields that creates and
// updates set. State, timestamps and other server-managed fields are excluded so
// that only edits to the definition change the hash. DatadogMonitor tests check
// that this list matches the fields its update requests send.
var MonitorDefinitionFields = []string{
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
	// Preserve exact numbers in SDK additional properties instead of rounding to float64.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err = decoder.Decode(&fields); err != nil {
		return "", err
	}

	definition := make(map[string]any, len(MonitorDefinitionFields)+len(extraFields))
	for _, field := range slices.Concat(MonitorDefinitionFields, extraFields) {
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
