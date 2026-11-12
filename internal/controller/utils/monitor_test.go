// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const baseMonitor = `{
	"id": 1, "name": "m", "type": "query alert", "query": "avg(last_5m):avg:system.load.1{*} > 1",
	"message": "msg", "tags": ["a:1", "b:2"], "priority": null, "restricted_roles": ["r1", "r2"],
	"draft_status": "published", "options": {"thresholds": {"critical": 1}, "notify_no_data": false, "notify_by": ["env", "service"], "renotify_statuses": ["alert", "warn"]},
	"overall_state": "OK", "modified": "2026-11-06T20:56:31.851443+00:00",
	"state": {"groups": {}}, "assets": [{"name": "runbook", "url": "https://example.com"}]
}`

func monitorFromJSON(t *testing.T, edits map[string]any) datadogV1.Monitor {
	t.Helper()
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(baseMonitor), &fields))
	maps.Copy(fields, edits)
	data, err := json.Marshal(fields)
	require.NoError(t, err)
	var monitor datadogV1.Monitor
	require.NoError(t, json.Unmarshal(data, &monitor))
	return monitor
}

func TestMonitorDefinitionHash(t *testing.T) {
	base, err := MonitorDefinitionHash(monitorFromJSON(t, nil))
	require.NoError(t, err)

	for _, tt := range []struct {
		name     string
		edits    map[string]any
		extra    []string
		wantSame bool
	}{
		{name: "state changes", edits: map[string]any{"overall_state": "Alert", "state": map[string]any{"groups": map[string]any{"host:a": map[string]any{"status": "Alert"}}}}, wantSame: true},
		{name: "modified timestamp", edits: map[string]any{"modified": "2026-11-07T00:00:00+00:00"}, wantSame: true},
		{name: "tag order", edits: map[string]any{"tags": []string{"b:2", "a:1"}}, wantSame: true},
		{name: "restricted role order", edits: map[string]any{"restricted_roles": []string{"r2", "r1"}}, wantSame: true},
		{name: "notify_by and renotify_statuses order", edits: map[string]any{"options": map[string]any{"thresholds": map[string]any{"critical": 1}, "notify_no_data": false, "notify_by": []string{"service", "env"}, "renotify_statuses": []string{"warn", "alert"}}}, wantSame: true},
		{name: "assets without asking for them", edits: map[string]any{"assets": nil}, wantSame: true},
		{name: "message", edits: map[string]any{"message": "edited"}},
		{name: "options sub-field", edits: map[string]any{"options": map[string]any{"thresholds": map[string]any{"critical": 1}, "notify_no_data": false, "notify_by": []string{"env", "service"}, "renotify_statuses": []string{"alert", "warn"}, "renotify_interval": 60}}},
		{name: "priority", edits: map[string]any{"priority": 3}},
		{name: "restricted roles", edits: map[string]any{"restricted_roles": []string{"role"}}},
		{name: "draft status", edits: map[string]any{"draft_status": "draft"}},
		{name: "tags", edits: map[string]any{"tags": []string{"a:1"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := MonitorDefinitionHash(monitorFromJSON(t, tt.edits))
			require.NoError(t, err)
			if tt.wantSame {
				assert.Equal(t, base, hash)
			} else {
				assert.NotEqual(t, base, hash)
			}
		})
	}

	t.Run("extra fields", func(t *testing.T) {
		withAssets, err := MonitorDefinitionHash(monitorFromJSON(t, nil), "assets")
		require.NoError(t, err)
		assert.NotEqual(t, base, withAssets)

		writeResponse, err := MonitorDefinitionHash(monitorFromJSON(t, map[string]any{"assets": []map[string]any{{"name": "runbook", "url": "https://example.com", "monitor_id": 1}}}), "assets")
		require.NoError(t, err)
		assert.Equal(t, withAssets, writeResponse, "write responses add monitor_id to assets")

		editedAssets, err := MonitorDefinitionHash(monitorFromJSON(t, map[string]any{"assets": []map[string]any{{"name": "runbook", "url": "https://example.com/other"}}}), "assets")
		require.NoError(t, err)
		assert.NotEqual(t, withAssets, editedAssets)
	})
}

func TestMonitorDefinitionHashLargeIntegers(t *testing.T) {
	for _, tt := range []struct {
		name   string
		nested bool
	}{
		{name: "top-level field"},
		{name: "nested option", nested: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hashNumber := func(value json.Number) string {
				definition := map[string]any{"future_setting": value}
				fields := []string{"future_setting"}
				if tt.nested {
					definition = map[string]any{"options": definition}
					fields = nil
				}
				definition["query"] = "q"
				definition["type"] = "query alert"
				data, err := json.Marshal(definition)
				require.NoError(t, err)
				var monitor datadogV1.Monitor
				require.NoError(t, json.Unmarshal(data, &monitor))
				require.Nil(t, monitor.UnparsedObject, "the fixture must decode through the typed SDK model")
				digest, err := MonitorDefinitionHash(monitor, fields...)
				require.NoError(t, err)
				return digest
			}

			firstHash := hashNumber(json.Number("9007199254740992"))
			secondHash := hashNumber(json.Number("9007199254740993"))
			assert.NotEqual(t, firstHash, secondHash, "adjacent large integers must remain distinguishable")
		})
	}
}
