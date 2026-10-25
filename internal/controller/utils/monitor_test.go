// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// remoteMonitorJSON mimics a GET /api/v1/monitor/{id} response for a monitor
// created from desiredMonitorUpdate: Datadog adds read-only fields and option
// defaults, reorders tags, trims the message, reports the unset priority as
// null and omits the default draft status.
const remoteMonitorJSON = `{
	"id": 12345,
	"created": "2026-01-01T00:00:00Z",
	"creator": {"email": "user@example.com"},
	"modified": "2026-01-01T00:00:00Z",
	"overall_state": "OK",
	"multi": true,
	"name": "High CPU",
	"message": "CPU is high @slack-team",
	"priority": null,
	"query": "avg(last_5m):avg:system.cpu.user{*} by {host} > 80",
	"restricted_roles": null,
	"tags": ["team:a", "generated:kubernetes"],
	"type": "query alert",
	"options": {
		"thresholds": {"critical": 80.0, "warning": 70.0},
		"notify_no_data": false,
		"notify_audit": false,
		"new_host_delay": 300,
		"include_tags": true,
		"silenced": {},
		"renotify_interval": 30,
		"notify_by": ["host", "cluster"]
	}
}`

func desiredMonitorUpdate() datadogV1.MonitorUpdateRequest {
	thresholds := datadogV1.MonitorThresholds{}
	thresholds.SetCritical(80)
	thresholds.SetWarning(70)
	options := datadogV1.MonitorOptions{}
	options.SetThresholds(thresholds)
	options.SetRenotifyInterval(30)
	options.SetNotifyBy([]string{"cluster", "host"})

	u := datadogV1.NewMonitorUpdateRequest()
	u.SetType(datadogV1.MONITORTYPE_METRIC_ALERT)
	u.SetName("High CPU")
	u.SetMessage("CPU is high @slack-team\n")
	u.SetPriority(0)
	u.SetQuery("avg(last_5m):avg:system.cpu.user{*} by {host} > 80")
	u.SetRestrictedRoles(nil)
	u.SetOptions(options)
	u.SetTags([]string{"generated:kubernetes", "team:a"})
	return *u
}

func remoteMonitor(t *testing.T, mutate func(m map[string]any)) datadogV1.Monitor {
	t.Helper()
	raw := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(remoteMonitorJSON), &raw))
	if mutate != nil {
		mutate(raw)
	}
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	m := datadogV1.Monitor{}
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func TestMonitorMatchesUpdateRequest(t *testing.T) {
	options := func(m map[string]any) map[string]any { return m["options"].(map[string]any) }

	tests := []struct {
		name   string
		mutate func(m map[string]any)
		want   bool
	}{
		{
			name: "remote normalized by Datadog matches",
			want: true,
		},
		{
			name:   "remote reporting the default draft status matches",
			mutate: func(m map[string]any) { m["draft_status"] = "published" },
			want:   true,
		},
		{
			name:   "draft status drift",
			mutate: func(m map[string]any) { m["draft_status"] = "draft" },
		},
		{
			name:   "name drift",
			mutate: func(m map[string]any) { m["name"] = "Renamed in the UI" },
		},
		{
			name:   "message drift",
			mutate: func(m map[string]any) { m["message"] = "CPU is high" },
		},
		{
			name:   "query drift",
			mutate: func(m map[string]any) { m["query"] = "avg(last_5m):avg:system.cpu.user{*} by {host} > 90" },
		},
		{
			name:   "type drift",
			mutate: func(m map[string]any) { m["type"] = "log alert" },
		},
		{
			name:   "priority drift",
			mutate: func(m map[string]any) { m["priority"] = 1 },
		},
		{
			name:   "tag added out of band",
			mutate: func(m map[string]any) { m["tags"] = []any{"team:a", "generated:kubernetes", "extra"} },
		},
		{
			name:   "restricted role added out of band",
			mutate: func(m map[string]any) { m["restricted_roles"] = []any{"00000000-0000-0000-0000-000000000000"} },
		},
		{
			name:   "managed threshold drift",
			mutate: func(m map[string]any) { options(m)["thresholds"].(map[string]any)["critical"] = 90 },
		},
		{
			name:   "managed option drift",
			mutate: func(m map[string]any) { options(m)["renotify_interval"] = 60 },
		},
		{
			name:   "managed option removed",
			mutate: func(m map[string]any) { delete(options(m), "notify_by") },
		},
		{
			name:   "unmanaged option differs",
			mutate: func(m map[string]any) { options(m)["new_host_delay"] = 600 },
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MonitorMatchesUpdateRequest(desiredMonitorUpdate(), remoteMonitor(t, tt.mutate))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMonitorMatchesUpdateRequest_FromJSONSpec(t *testing.T) {
	// A DatadogGenericResource jsonSpec only manages the fields it lists.
	desired := datadogV1.MonitorUpdateRequest{}
	require.NoError(t, json.Unmarshal([]byte(`{
		"name": "High CPU",
		"type": "query alert",
		"query": "avg(last_5m):avg:system.cpu.user{*} by {host} > 80",
		"options": {"thresholds": {"critical": 80}}
	}`), &desired))

	got, err := MonitorMatchesUpdateRequest(desired, remoteMonitor(t, nil))
	require.NoError(t, err)
	assert.True(t, got)

	got, err = MonitorMatchesUpdateRequest(desired, remoteMonitor(t, func(m map[string]any) {
		m["options"].(map[string]any)["thresholds"].(map[string]any)["critical"] = 95
	}))
	require.NoError(t, err)
	assert.False(t, got)
}
