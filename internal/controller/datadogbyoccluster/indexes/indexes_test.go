// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package indexes

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/pomsky"
)

type indexSpec = datadoghqv1alpha1.DatadogBYOCClusterIndexSpec

func TestBuildPlan(t *testing.T) {
	mainWithCatchAll := []pomsky.RoutingRule{{Filter: "*", IndexID: "datadog"}}
	tests := []struct {
		name         string
		desired      []indexSpec
		current      []pomsky.Index
		currentRules []pomsky.RoutingRule
		want         Plan
	}{
		{
			name:    "main only",
			desired: nil,
			current: []pomsky.Index{{ID: "datadog", Retention: "30 days"}},
			want:    Plan{RoutingTable: mainWithCatchAll},
		},
		{
			name:    "creates missing indexes and routes to main last",
			desired: []indexSpec{{Name: "payment", Filter: "service:payment", Retention: "90 days"}, {Name: "security", Filter: "source:cloudtrail"}},
			current: []pomsky.Index{{ID: "datadog", Retention: "30 days"}},
			currentRules: []pomsky.RoutingRule{
				{Filter: "*", IndexID: "datadog"},
			},
			want: Plan{
				Create: []pomsky.Index{{ID: "datadog-payment", Retention: "90 days"}, {ID: "datadog-security", Retention: "30 days"}},
				RoutingTable: []pomsky.RoutingRule{
					{Filter: "service:payment", IndexID: "datadog-payment"},
					{Filter: "source:cloudtrail", IndexID: "datadog-security"},
					{Filter: "*", IndexID: "datadog"},
				},
			},
		},
		{
			name:    "up to date",
			desired: []indexSpec{{Name: "payment", Filter: "service:payment", Retention: "90 days"}},
			current: []pomsky.Index{{ID: "datadog", Retention: "30 days"}, {ID: "datadog-payment", Retention: "90 days"}},
			currentRules: []pomsky.RoutingRule{
				{Filter: "service:payment", IndexID: "datadog-payment"},
				{Filter: "*", IndexID: "datadog"},
			},
			want: Plan{},
		},
		{
			name:    "updates the retention of existing indexes",
			desired: []indexSpec{{Name: "payment", Filter: "service:payment", Retention: "7 days"}},
			current: []pomsky.Index{{ID: "datadog", Retention: "30 days"}, {ID: "datadog-payment", Retention: "90 days"}},
			currentRules: []pomsky.RoutingRule{
				{Filter: "service:payment", IndexID: "datadog-payment"},
				{Filter: "*", IndexID: "datadog"},
			},
			want: Plan{UpdateRetention: []pomsky.Index{{ID: "datadog-payment", Retention: "7 days"}}},
		},
		{
			name:    "reorders the routing table",
			desired: []indexSpec{{Name: "security", Filter: "source:cloudtrail", Retention: "30 days"}, {Name: "payment", Filter: "service:payment", Retention: "30 days"}},
			current: []pomsky.Index{{ID: "datadog", Retention: "30 days"}, {ID: "datadog-payment", Retention: "30 days"}, {ID: "datadog-security", Retention: "30 days"}},
			currentRules: []pomsky.RoutingRule{
				{Filter: "service:payment", IndexID: "datadog-payment"},
				{Filter: "source:cloudtrail", IndexID: "datadog-security"},
				{Filter: "*", IndexID: "datadog"},
			},
			want: Plan{RoutingTable: []pomsky.RoutingRule{
				{Filter: "source:cloudtrail", IndexID: "datadog-security"},
				{Filter: "service:payment", IndexID: "datadog-payment"},
				{Filter: "*", IndexID: "datadog"},
			}},
		},
		{
			name:    "deletes unlisted indexes but not main, spans, metrics, sketches or foreign indexes",
			desired: []indexSpec{{Name: "payment", Filter: "service:payment", Retention: "30 days"}},
			current: []pomsky.Index{
				{ID: "datadog", Retention: "30 days"},
				{ID: "datadog-payment", Retention: "30 days"},
				{ID: "datadog-old", Retention: "30 days"},
				{ID: "datadog-spans"},
				{ID: "datadog-metrics"},
				{ID: "datadog-sketches"},
				{ID: "other"},
			},
			currentRules: []pomsky.RoutingRule{
				{Filter: "service:payment", IndexID: "datadog-payment"},
				{Filter: "*", IndexID: "datadog"},
			},
			want: Plan{Delete: []string{"datadog-old"}},
		},
		{
			name:    "listed main keeps its filter and position and is not appended",
			desired: []indexSpec{{Name: "main", Filter: "env:prod", Retention: "7 days"}, {Name: "payment", Filter: "*", Retention: "30 days"}},
			current: []pomsky.Index{{ID: "datadog", Retention: "30 days"}},
			currentRules: []pomsky.RoutingRule{
				{Filter: "*", IndexID: "datadog"},
			},
			want: Plan{
				Create:          []pomsky.Index{{ID: "datadog-payment", Retention: "30 days"}},
				UpdateRetention: []pomsky.Index{{ID: "datadog", Retention: "7 days"}},
				RoutingTable: []pomsky.RoutingRule{
					{Filter: "env:prod", IndexID: "datadog"},
					{Filter: "*", IndexID: "datadog-payment"},
				},
			},
		},
		{
			name:    "listed main is not created",
			desired: []indexSpec{{Name: "main", Filter: "*", Retention: "30 days"}},
			current: nil,
			want:    Plan{RoutingTable: mainWithCatchAll},
		},
		{
			name:    "an empty retention is the default",
			desired: []indexSpec{{Name: "payment", Filter: "service:payment"}},
			current: []pomsky.Index{{ID: "datadog", Retention: "30 days"}, {ID: "datadog-payment", Retention: "30 days"}},
			currentRules: []pomsky.RoutingRule{
				{Filter: "service:payment", IndexID: "datadog-payment"},
				{Filter: "*", IndexID: "datadog"},
			},
			want: Plan{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, BuildPlan(tt.desired, tt.current, tt.currentRules)); diff != "" {
				t.Errorf("plan mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

var errFake = errors.New("fake error")

// fakeClient records the writes and fails the call named by failOn.
type fakeClient struct {
	indexes      []pomsky.Index
	routingTable []pomsky.RoutingRule
	failOn       string
	calls        []string
}

func (f *fakeClient) fail(call string) error {
	if f.failOn == call {
		return errFake
	}
	return nil
}

func (f *fakeClient) ListIndexes(context.Context) ([]pomsky.Index, error) {
	return f.indexes, f.fail("list indexes")
}

func (f *fakeClient) CreateIndex(_ context.Context, id, retention string) error {
	f.calls = append(f.calls, fmt.Sprintf("create %s %s", id, retention))
	return f.fail("create index")
}

func (f *fakeClient) UpdateIndexRetention(_ context.Context, id, retention string) error {
	f.calls = append(f.calls, fmt.Sprintf("update %s %s", id, retention))
	return f.fail("update index")
}

func (f *fakeClient) DeleteIndex(_ context.Context, id string) error {
	f.calls = append(f.calls, "delete "+id)
	return f.fail("delete index")
}

func (f *fakeClient) GetRoutingTable(context.Context) ([]pomsky.RoutingRule, error) {
	return f.routingTable, f.fail("get routing table")
}

func (f *fakeClient) SetRoutingTable(_ context.Context, rules []pomsky.RoutingRule) error {
	f.calls = append(f.calls, fmt.Sprintf("set routing table %v", rules))
	return f.fail("set routing table")
}

func TestSync(t *testing.T) {
	tests := []struct {
		name    string
		desired []indexSpec
		client  *fakeClient
		want    []string
	}{
		{
			name:    "no indexes in the spec leaves the cluster to the UI",
			desired: nil,
			client:  &fakeClient{indexes: []pomsky.Index{{ID: "datadog"}, {ID: "datadog-ui"}}},
			want:    nil,
		},
		{
			name:    "creates, routes, updates and deletes in this order",
			desired: []indexSpec{{Name: "new", Filter: "service:new", Retention: "7 days"}, {Name: "kept", Filter: "service:kept", Retention: "7 days"}},
			client: &fakeClient{
				indexes:      []pomsky.Index{{ID: "datadog", Retention: "30 days"}, {ID: "datadog-kept", Retention: "30 days"}, {ID: "datadog-old", Retention: "30 days"}},
				routingTable: []pomsky.RoutingRule{{Filter: "service:old", IndexID: "datadog-old"}, {Filter: "*", IndexID: "datadog"}},
			},
			want: []string{
				"create datadog-new 7 days",
				"set routing table [{service:new datadog-new} {service:kept datadog-kept} {* datadog}]",
				"update datadog-kept 7 days",
				"delete datadog-old",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Sync(context.Background(), tt.client, tt.desired); err != nil {
				t.Fatalf("Sync() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, tt.client.calls); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSyncFailure(t *testing.T) {
	desired := []indexSpec{{Name: "new", Filter: "service:new", Retention: "7 days"}, {Name: "kept", Filter: "service:kept", Retention: "7 days"}}
	tests := []struct {
		name   string
		failOn string
		want   []string
	}{
		{name: "list indexes", failOn: "list indexes", want: nil},
		{name: "get routing table", failOn: "get routing table", want: nil},
		{name: "create index", failOn: "create index", want: []string{"create datadog-new 7 days"}},
		{
			name:   "set routing table",
			failOn: "set routing table",
			want: []string{
				"create datadog-new 7 days",
				"set routing table [{service:new datadog-new} {service:kept datadog-kept} {* datadog}]",
			},
		},
		{
			name:   "update index",
			failOn: "update index",
			want: []string{
				"create datadog-new 7 days",
				"set routing table [{service:new datadog-new} {service:kept datadog-kept} {* datadog}]",
				"update datadog-kept 7 days",
			},
		},
		{
			name:   "delete index",
			failOn: "delete index",
			want: []string{
				"create datadog-new 7 days",
				"set routing table [{service:new datadog-new} {service:kept datadog-kept} {* datadog}]",
				"update datadog-kept 7 days",
				"delete datadog-old",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{
				indexes:      []pomsky.Index{{ID: "datadog", Retention: "30 days"}, {ID: "datadog-kept", Retention: "30 days"}, {ID: "datadog-old", Retention: "30 days"}},
				routingTable: []pomsky.RoutingRule{{Filter: "*", IndexID: "datadog"}},
				failOn:       tt.failOn,
			}
			if err := Sync(context.Background(), client, desired); !errors.Is(err, errFake) {
				t.Errorf("Sync() error = %v, want %v", err, errFake)
			}
			if diff := cmp.Diff(tt.want, client.calls); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
