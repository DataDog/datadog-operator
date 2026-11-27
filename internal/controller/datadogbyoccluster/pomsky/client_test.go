// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pomsky

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestIndexID(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "main", want: "datadog"},
		{name: "payment", want: "datadog-payment"},
		{name: "my-index", want: "datadog-my-index"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IndexID(tt.name); got != tt.want {
				t.Errorf("IndexID(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

type recordedRequest struct {
	Method string
	Path   string
	Body   any
}

const mainIndexMetadata = `{
	"version": "0.9",
	"index_uid": "datadog:01",
	"index_config": {
		"version": "0.8",
		"index_id": "datadog",
		"index_uri": "s3://bucket/datadog",
		"doc_mapping": {"mode": "dynamic"},
		"retention": {"period": "30 days", "schedule": "hourly"}
	}
}`

func TestClient(t *testing.T) {
	tests := []struct {
		name      string
		responses map[string]string
		call      func(Client) (any, error)
		want      any
		wantErr   bool
		wantReqs  []recordedRequest
	}{
		{
			name: "list indexes",
			responses: map[string]string{
				"GET /api/v1/indexes": `[
					{"index_config": {"index_id": "datadog", "retention": {"period": "30 days"}}},
					{"index_config": {"index_id": "datadog-payment"}}
				]`,
			},
			call: func(c Client) (any, error) { return c.ListIndexes(context.Background()) },
			want: []Index{{ID: "datadog", Retention: "30 days"}, {ID: "datadog-payment"}},
			wantReqs: []recordedRequest{
				{Method: "GET", Path: "/api/v1/indexes"},
			},
		},
		{
			name: "create index clones the main index",
			responses: map[string]string{
				"GET /api/v1/indexes/datadog": mainIndexMetadata,
				"POST /api/v1/indexes":        `{}`,
			},
			call: func(c Client) (any, error) {
				return nil, c.CreateIndex(context.Background(), "datadog-payment", "90 days")
			},
			wantReqs: []recordedRequest{
				{Method: "GET", Path: "/api/v1/indexes/datadog"},
				{Method: "POST", Path: "/api/v1/indexes", Body: map[string]any{
					"version":     "0.8",
					"index_id":    "datadog-payment",
					"doc_mapping": map[string]any{"mode": "dynamic"},
					"retention":   map[string]any{"period": "90 days", "schedule": "hourly"},
				}},
			},
		},
		{
			name: "create index fails without the main index",
			responses: map[string]string{
				"GET /api/v1/indexes/datadog": `{}`,
			},
			call: func(c Client) (any, error) {
				return nil, c.CreateIndex(context.Background(), "datadog-payment", "90 days")
			},
			wantErr: true,
			wantReqs: []recordedRequest{
				{Method: "GET", Path: "/api/v1/indexes/datadog"},
			},
		},
		{
			name: "update retention keeps the rest of the configuration",
			responses: map[string]string{
				"GET /api/v1/indexes/datadog": mainIndexMetadata,
				"PUT /api/v1/indexes/datadog": `{}`,
			},
			call: func(c Client) (any, error) {
				return nil, c.UpdateIndexRetention(context.Background(), "datadog", "1 year")
			},
			wantReqs: []recordedRequest{
				{Method: "GET", Path: "/api/v1/indexes/datadog"},
				{Method: "PUT", Path: "/api/v1/indexes/datadog", Body: map[string]any{
					"version":     "0.8",
					"index_id":    "datadog",
					"index_uri":   "s3://bucket/datadog",
					"doc_mapping": map[string]any{"mode": "dynamic"},
					"retention":   map[string]any{"period": "1 year", "schedule": "hourly"},
				}},
			},
		},
		{
			name: "update retention of an index without retention",
			responses: map[string]string{
				"GET /api/v1/indexes/datadog-payment": `{"index_config": {"index_id": "datadog-payment"}}`,
				"PUT /api/v1/indexes/datadog-payment": `{}`,
			},
			call: func(c Client) (any, error) {
				return nil, c.UpdateIndexRetention(context.Background(), "datadog-payment", "7 days")
			},
			wantReqs: []recordedRequest{
				{Method: "GET", Path: "/api/v1/indexes/datadog-payment"},
				{Method: "PUT", Path: "/api/v1/indexes/datadog-payment", Body: map[string]any{
					"index_id":  "datadog-payment",
					"retention": map[string]any{"period": "7 days"},
				}},
			},
		},
		{
			name: "get routing table",
			responses: map[string]string{
				"GET /api/v1/index-routing-table": `[{"filter": "service:payment", "index_id": "datadog-payment"}, {"filter": "*", "index_id": "datadog"}]`,
			},
			call: func(c Client) (any, error) { return c.GetRoutingTable(context.Background()) },
			want: []RoutingRule{{Filter: "service:payment", IndexID: "datadog-payment"}, {Filter: "*", IndexID: "datadog"}},
			wantReqs: []recordedRequest{
				{Method: "GET", Path: "/api/v1/index-routing-table"},
			},
		},
		{
			name:      "set routing table",
			responses: map[string]string{"PUT /api/v1/index-routing-table": `[]`},
			call: func(c Client) (any, error) {
				return nil, c.SetRoutingTable(context.Background(), []RoutingRule{{Filter: "*", IndexID: "datadog"}})
			},
			wantReqs: []recordedRequest{
				{Method: "PUT", Path: "/api/v1/index-routing-table", Body: []any{map[string]any{"filter": "*", "index_id": "datadog"}}},
			},
		},
		{
			name:      "list indexes fails",
			responses: map[string]string{},
			call:      func(c Client) (any, error) { return c.ListIndexes(context.Background()) },
			want:      []Index(nil),
			wantErr:   true,
			wantReqs:  []recordedRequest{{Method: "GET", Path: "/api/v1/indexes"}},
		},
		{
			name:      "undecodable response",
			responses: map[string]string{"GET /api/v1/index-routing-table": `{`},
			call:      func(c Client) (any, error) { return c.GetRoutingTable(context.Background()) },
			want:      []RoutingRule(nil),
			wantErr:   true,
			wantReqs:  []recordedRequest{{Method: "GET", Path: "/api/v1/index-routing-table"}},
		},
		{
			name: "create index fails",
			responses: map[string]string{
				"GET /api/v1/indexes/datadog": mainIndexMetadata,
			},
			call: func(c Client) (any, error) {
				return nil, c.CreateIndex(context.Background(), "datadog-payment", "90 days")
			},
			wantErr: true,
			wantReqs: []recordedRequest{
				{Method: "GET", Path: "/api/v1/indexes/datadog"},
				{Method: "POST", Path: "/api/v1/indexes", Body: map[string]any{
					"version":     "0.8",
					"index_id":    "datadog-payment",
					"doc_mapping": map[string]any{"mode": "dynamic"},
					"retention":   map[string]any{"period": "90 days", "schedule": "hourly"},
				}},
			},
		},
		{
			name:      "update retention of a missing index",
			responses: map[string]string{},
			call: func(c Client) (any, error) {
				return nil, c.UpdateIndexRetention(context.Background(), "datadog-payment", "7 days")
			},
			wantErr:  true,
			wantReqs: []recordedRequest{{Method: "GET", Path: "/api/v1/indexes/datadog-payment"}},
		},
		{
			name: "update retention fails",
			responses: map[string]string{
				"GET /api/v1/indexes/datadog-payment": `{"index_config": {"index_id": "datadog-payment"}}`,
			},
			call: func(c Client) (any, error) {
				return nil, c.UpdateIndexRetention(context.Background(), "datadog-payment", "7 days")
			},
			wantErr: true,
			wantReqs: []recordedRequest{
				{Method: "GET", Path: "/api/v1/indexes/datadog-payment"},
				{Method: "PUT", Path: "/api/v1/indexes/datadog-payment", Body: map[string]any{
					"index_id":  "datadog-payment",
					"retention": map[string]any{"period": "7 days"},
				}},
			},
		},
		{
			name:      "set routing table fails",
			responses: map[string]string{},
			call: func(c Client) (any, error) {
				return nil, c.SetRoutingTable(context.Background(), []RoutingRule{{Filter: "*", IndexID: "datadog"}})
			},
			wantErr: true,
			wantReqs: []recordedRequest{
				{Method: "PUT", Path: "/api/v1/index-routing-table", Body: []any{map[string]any{"filter": "*", "index_id": "datadog"}}},
			},
		},
		{
			name:      "unsuccessful status",
			responses: map[string]string{},
			call:      func(c Client) (any, error) { return c.GetRoutingTable(context.Background()) },
			want:      []RoutingRule(nil),
			wantErr:   true,
			wantReqs: []recordedRequest{
				{Method: "GET", Path: "/api/v1/index-routing-table"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []recordedRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request := recordedRequest{Method: r.Method, Path: r.URL.Path}
				if data, _ := io.ReadAll(r.Body); len(data) > 0 {
					if err := json.Unmarshal(data, &request.Body); err != nil {
						t.Errorf("request body is not JSON: %v", err)
					}
				}
				requests = append(requests, request)
				response, found := tt.responses[r.Method+" "+r.URL.Path]
				if !found {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				_, _ = io.WriteString(w, response)
			}))
			defer server.Close()

			got, err := tt.call(NewClient(server.URL))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantReqs, requests); diff != "" {
				t.Errorf("requests mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClientInvalidBaseURL(t *testing.T) {
	if _, err := NewClient("http://[::1").GetRoutingTable(context.Background()); err == nil {
		t.Error("GetRoutingTable() error = nil, want an error")
	}
}

func TestClientUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()

	if _, err := NewClient(url).GetRoutingTable(context.Background()); err == nil {
		t.Error("GetRoutingTable() error = nil, want an error")
	}
}
