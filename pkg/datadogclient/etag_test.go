// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorClientSendsIfNoneMatch(t *testing.T) {
	var gotMethod, gotIfNoneMatch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"type":"metric alert","query":"avg(last_5m):avg:system.load.1{*} > 1"}`))
	}))
	t.Cleanup(server.Close)

	client := InitMonitorClient()
	client.Client.GetConfig().Servers = datadogapi.ServerConfigurations{{URL: server.URL}}
	baseCtx := context.Background()

	tests := []struct {
		name            string
		ctx             context.Context
		call            func(ctx context.Context) error
		wantMethod      string
		wantIfNoneMatch string
	}{
		{
			name: "GET with an ETag",
			ctx:  WithIfNoneMatch(baseCtx, `"abc"`),
			call: func(ctx context.Context) error {
				_, _, err := client.GetMonitor(ctx, 1)
				return err
			},
			wantMethod:      http.MethodGet,
			wantIfNoneMatch: `"abc"`,
		},
		{
			name: "GET without an ETag",
			ctx:  WithIfNoneMatch(baseCtx, ""),
			call: func(ctx context.Context) error {
				_, _, err := client.GetMonitor(ctx, 1)
				return err
			},
			wantMethod: http.MethodGet,
		},
		{
			name: "PUT ignores the ETag",
			ctx:  WithIfNoneMatch(baseCtx, `"abc"`),
			call: func(ctx context.Context) error {
				_, _, err := client.UpdateMonitor(ctx, 1, datadogV1.MonitorUpdateRequest{})
				return err
			},
			wantMethod: http.MethodPut,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMethod, gotIfNoneMatch = "", ""
			require.NoError(t, tt.call(tt.ctx))
			assert.Equal(t, tt.wantMethod, gotMethod)
			assert.Equal(t, tt.wantIfNoneMatch, gotIfNoneMatch)
		})
	}
}
