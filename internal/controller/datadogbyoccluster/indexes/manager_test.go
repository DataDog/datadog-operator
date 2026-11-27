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

func TestManagerSync(t *testing.T) {
	// Validation requires main in every non-empty spec.
	mainSpec := indexSpec{Name: "main", Filter: "*", Retention: "30 days"}
	// changesDesired against changesIndexes and changesRoutingTable needs every kind of change.
	changesDesired := []indexSpec{{Name: "new", Filter: "service:new", Retention: "7 days"}, {Name: "kept", Filter: "service:kept", Retention: "7 days"}, mainSpec}
	changesIndexes := []pomsky.Index{{ID: "datadog", Retention: "30 days"}, {ID: "datadog-kept", Retention: "30 days"}, {ID: "datadog-old", Retention: "30 days"}}
	changesRoutingTable := []pomsky.RoutingRule{{Filter: "service:old", IndexID: "datadog-old"}, {Filter: "*", IndexID: "datadog"}}

	tests := []struct {
		name         string
		desired      []indexSpec
		indexes      []pomsky.Index
		routingTable []pomsky.RoutingRule
		failOn       string
		wantErr      error
		want         []string
	}{
		{
			name:    "no indexes in the spec leaves the cluster to the UI",
			desired: nil,
			indexes: []pomsky.Index{{ID: "datadog"}, {ID: "datadog-ui"}},
			want:    nil,
		},
		{
			name:         "up to date",
			desired:      []indexSpec{{Name: "payment", Filter: "service:payment", Retention: "90 days"}, mainSpec},
			indexes:      []pomsky.Index{{ID: "datadog", Retention: "30 days"}, {ID: "datadog-payment", Retention: "90 days"}},
			routingTable: []pomsky.RoutingRule{{Filter: "service:payment", IndexID: "datadog-payment"}, {Filter: "*", IndexID: "datadog"}},
			want:         nil,
		},
		{
			name:         "creates, routes, updates and deletes in this order",
			desired:      changesDesired,
			indexes:      changesIndexes,
			routingTable: changesRoutingTable,
			want: []string{
				"create datadog-new 7 days",
				"set routing table [{service:new datadog-new} {service:kept datadog-kept} {* datadog}]",
				"update datadog-kept 7 days",
				"delete datadog-old",
			},
		},
		{
			name:    "deletes unlisted indexes but not spans, metrics, sketches or foreign indexes",
			desired: []indexSpec{{Name: "payment", Filter: "service:payment", Retention: "30 days"}, mainSpec},
			indexes: []pomsky.Index{
				{ID: "datadog", Retention: "30 days"},
				{ID: "datadog-payment", Retention: "30 days"},
				{ID: "datadog-old", Retention: "30 days"},
				{ID: "datadog-spans"},
				{ID: "datadog-metrics"},
				{ID: "datadog-sketches"},
				{ID: "other"},
			},
			routingTable: []pomsky.RoutingRule{{Filter: "service:payment", IndexID: "datadog-payment"}, {Filter: "*", IndexID: "datadog"}},
			want:         []string{"delete datadog-old"},
		},
		{
			name:         "main keeps its filter and position",
			desired:      []indexSpec{{Name: "main", Filter: "env:prod", Retention: "7 days"}, {Name: "payment", Filter: "*", Retention: "30 days"}},
			indexes:      []pomsky.Index{{ID: "datadog", Retention: "7 days"}},
			routingTable: []pomsky.RoutingRule{{Filter: "*", IndexID: "datadog"}},
			want: []string{
				"create datadog-payment 30 days",
				"set routing table [{env:prod datadog} {* datadog-payment}]",
			},
		},
		{
			name:         "stops while main does not exist",
			desired:      []indexSpec{{Name: "payment", Filter: "service:payment", Retention: "30 days"}, mainSpec},
			indexes:      []pomsky.Index{{ID: "datadog-payment", Retention: "30 days"}},
			routingTable: []pomsky.RoutingRule{{Filter: "service:payment", IndexID: "datadog-payment"}},
			wantErr:      errMainIndexMissing,
			want:         nil,
		},
		{
			name:         "stops when listing the indexes fails",
			desired:      changesDesired,
			indexes:      changesIndexes,
			routingTable: changesRoutingTable,
			failOn:       "list indexes",
			wantErr:      errFake,
			want:         nil,
		},
		{
			name:         "stops when getting the routing table fails",
			desired:      changesDesired,
			indexes:      changesIndexes,
			routingTable: changesRoutingTable,
			failOn:       "get routing table",
			wantErr:      errFake,
			want:         nil,
		},
		{
			name:         "stops when creating an index fails",
			desired:      changesDesired,
			indexes:      changesIndexes,
			routingTable: changesRoutingTable,
			failOn:       "create index",
			wantErr:      errFake,
			want:         []string{"create datadog-new 7 days"},
		},
		{
			name:         "stops when setting the routing table fails",
			desired:      changesDesired,
			indexes:      changesIndexes,
			routingTable: changesRoutingTable,
			failOn:       "set routing table",
			wantErr:      errFake,
			want: []string{
				"create datadog-new 7 days",
				"set routing table [{service:new datadog-new} {service:kept datadog-kept} {* datadog}]",
			},
		},
		{
			name:         "stops when updating an index fails",
			desired:      changesDesired,
			indexes:      changesIndexes,
			routingTable: changesRoutingTable,
			failOn:       "update index",
			wantErr:      errFake,
			want: []string{
				"create datadog-new 7 days",
				"set routing table [{service:new datadog-new} {service:kept datadog-kept} {* datadog}]",
				"update datadog-kept 7 days",
			},
		},
		{
			name:         "returns the error when deleting an index fails",
			desired:      changesDesired,
			indexes:      changesIndexes,
			routingTable: changesRoutingTable,
			failOn:       "delete index",
			wantErr:      errFake,
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
			client := &fakeClient{indexes: tt.indexes, routingTable: tt.routingTable, failOn: tt.failOn}
			if err := NewManager(client).Sync(context.Background(), tt.desired); !errors.Is(err, tt.wantErr) {
				t.Errorf("Sync() error = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, client.calls); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
