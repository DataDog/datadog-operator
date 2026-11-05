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

func TestMonitorMatchesSpec(t *testing.T) {
	const remoteJSON = `{
		"id":12345,"name":"CPU","message":"CPU high","query":"avg(last_5m):avg:system.cpu.user{*} > 80",
		"type":"query alert","priority":null,"draft_status":"published","overall_state":"OK",
		"tags":["team:a","generated:kubernetes"],"restricted_roles":null,
		"options":{"thresholds":{"critical":80.0,"warning":70},"include_tags":true,"notify_no_data":false,
			"notify_audit":false,"new_host_delay":300,"notify_by":["host","cluster"],
			"scheduling_options":{"custom_schedule":{"recurrences":[{"rrule":"RRULE:FREQ=DAILY","start":"2026-01-01","timezone":"UTC"}]}}}
	}`
	var remote datadogV1.Monitor
	require.NoError(t, json.Unmarshal([]byte(remoteJSON), &remote))
	tests := []struct {
		name, spec string
		want       bool
	}{
		{"omitted default fields", `{"name":"CPU","type":"metric alert","message":"CPU high\n","options":{"thresholds":{"critical":80}}}`, true},
		{"name drift", `{"name":"other"}`, false},
		{"type drift", `{"type":"log alert"}`, false},
		{"zero priority matches null", `{"priority":0}`, true},
		{"null priority matches null", `{"priority":null}`, true},
		{"priority drift", `{"priority":1}`, false},
		{"draft status drift", `{"draft_status":"draft"}`, false},
		{"false option matches", `{"options":{"notify_no_data":false}}`, true},
		{"explicit false drift", `{"options":{"include_tags":false}}`, false},
		{"explicit zero drift", `{"options":{"new_host_delay":0}}`, false},
		{"missing explicit option", `{"options":{"evaluation_delay":0}}`, false},
		{"nested threshold drift", `{"options":{"thresholds":{"critical":90}}}`, false},
		{"unordered sets", `{"tags":["generated:kubernetes","team:a","team:a"],"options":{"notify_by":["cluster","host"]}}`, true},
		{"extra tag", `{"tags":["team:a"]}`, false},
		{"explicit empty tags", `{"tags":[]}`, false},
		{"null role list", `{"restricted_roles":[]}`, true},
		{"role drift", `{"restricted_roles":["role"]}`, false},
		{"recurrence defaults", `{"options":{"scheduling_options":{"custom_schedule":{"recurrences":[{"rrule":"RRULE:FREQ=DAILY","timezone":"UTC"}]}}}}`, true},
		{"recurrence drift", `{"options":{"scheduling_options":{"custom_schedule":{"recurrences":[{"rrule":"RRULE:FREQ=WEEKLY"}]}}}}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := MonitorMatchesSpec(json.RawMessage(tt.spec), remote)
			require.NoError(t, err)
			assert.Equal(t, tt.want, matches)
		})
	}
}
