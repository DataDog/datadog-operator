// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"net/http"
	"strings"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

func Test_applyResourceState(t *testing.T) {
	earlier := metav1.NewTime(time.Unix(1612244495, 0))
	now := metav1.NewTime(time.Unix(1612244795, 0)) // 5 minutes later

	state := func(s v1alpha1.DatadogMonitorState) string {
		return string(s)
	}

	tests := []struct {
		name               string
		newState           string
		initialStatus      *v1alpha1.DatadogGenericResourceStatus
		wantState          string
		wantTransitionTime *metav1.Time
	}{
		{
			name:               "empty status -> OK sets state and transitions",
			newState:           state(v1alpha1.DatadogMonitorStateOK),
			initialStatus:      &v1alpha1.DatadogGenericResourceStatus{},
			wantState:          string(v1alpha1.DatadogMonitorStateOK),
			wantTransitionTime: &now,
		},
		{
			name:               "empty status -> Alert sets state and transitions",
			newState:           state(v1alpha1.DatadogMonitorStateAlert),
			initialStatus:      &v1alpha1.DatadogGenericResourceStatus{},
			wantState:          string(v1alpha1.DatadogMonitorStateAlert),
			wantTransitionTime: &now,
		},
		{
			name:     "OK -> OK preserves the existing transition time",
			newState: state(v1alpha1.DatadogMonitorStateOK),
			initialStatus: &v1alpha1.DatadogGenericResourceStatus{
				State:                   string(v1alpha1.DatadogMonitorStateOK),
				StateLastTransitionTime: &earlier,
			},
			wantState:          string(v1alpha1.DatadogMonitorStateOK),
			wantTransitionTime: &earlier,
		},
		{
			name:     "OK -> Alert bumps transition time",
			newState: state(v1alpha1.DatadogMonitorStateAlert),
			initialStatus: &v1alpha1.DatadogGenericResourceStatus{
				State:                   string(v1alpha1.DatadogMonitorStateOK),
				StateLastTransitionTime: &earlier,
			},
			wantState:          string(v1alpha1.DatadogMonitorStateAlert),
			wantTransitionTime: &now,
		},
		{
			name:               "No Data state",
			newState:           state(v1alpha1.DatadogMonitorStateNoData),
			initialStatus:      &v1alpha1.DatadogGenericResourceStatus{},
			wantState:          string(v1alpha1.DatadogMonitorStateNoData),
			wantTransitionTime: &now,
		},
		{
			name:               "Warn state",
			newState:           state(v1alpha1.DatadogMonitorStateWarn),
			initialStatus:      &v1alpha1.DatadogGenericResourceStatus{},
			wantState:          string(v1alpha1.DatadogMonitorStateWarn),
			wantTransitionTime: &now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.initialStatus
			applyResourceState(tt.newState, status, now)

			assert.Equal(t, tt.wantState, status.State, "unexpected state")
			assert.Equal(t, tt.wantTransitionTime, status.StateLastTransitionTime, "unexpected transition time")
			assert.NotNil(t, status.StateLastUpdateTime, "expected StateLastUpdateTime to be set")
			assert.Equal(t, now, *status.StateLastUpdateTime, "expected StateLastUpdateTime == now")
		})
	}
}

func Test_monitorHandler_remoteMatchesSpec(t *testing.T) {
	const jsonSpec = `{"name":"High CPU","type":"query alert","query":"avg(last_5m):avg:system.cpu.user{*} > 80","options":{"thresholds":{"critical":80}}}`
	const remote = `{"id":12345,"name":"High CPU","type":"query alert","query":"avg(last_5m):avg:system.cpu.user{*} > 80",` +
		`"overall_state":"OK","tags":["team:a"],"options":{"thresholds":{"critical":80},"notify_audit":false,"include_tags":true}}`

	tests := []struct {
		name       string
		jsonSpec   string
		statusCode int
		body       string
		want       bool
		wantErr    bool
	}{
		{name: "remote matches the fields the spec manages", jsonSpec: jsonSpec, statusCode: http.StatusOK, body: remote, want: true},
		{name: "remote drifted", jsonSpec: strings.Replace(jsonSpec, "High CPU", "Renamed", 1), statusCode: http.StatusOK, body: remote},
		{name: "invalid spec falls back to update", jsonSpec: `{"name":`, statusCode: http.StatusOK, body: remote},
		{name: "remote not found", jsonSpec: jsonSpec, statusCode: http.StatusNotFound, body: `{"errors":["Monitor not found"]}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestHTTPServer(tt.statusCode, tt.body)
			defer server.Close()

			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			h := &MonitorHandler{client: datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))}
			instance := &v1alpha1.DatadogGenericResource{
				Spec:   v1alpha1.DatadogGenericResourceSpec{Type: v1alpha1.Monitor, JsonSpec: tt.jsonSpec},
				Status: v1alpha1.DatadogGenericResourceStatus{Id: "12345"},
			}

			got, err := h.remoteMatchesSpec(setupTestAuth(server.URL), instance)
			if tt.wantErr {
				assert.ErrorContains(t, err, ctrutils.NotFoundString)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
