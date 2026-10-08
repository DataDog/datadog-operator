// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func obj(t *testing.T, doc string) *unstructured.Unstructured {
	t.Helper()
	u := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(doc), &u.Object); err != nil {
		t.Fatal(err)
	}
	return u
}

// TestSummaryExtractors covers the status extraction of every summary kind.
func TestSummaryExtractors(t *testing.T) {
	tests := []struct {
		kind string
		doc  string
		want itemStatus
	}{
		{"DatadogMonitor", `status: {monitorState: Warn, monitorStateSyncStatus: OK}`, itemStatus{state: itemOK, bucket: "Warn"}},
		{"DatadogMonitor", `status: {monitorState: "No Data"}`, itemStatus{bucket: "No Data"}},
		{"DatadogMonitor", `status: {monitorState: Ignored, monitorStateSyncStatus: error getting monitor}`, itemStatus{state: itemError, message: "error getting monitor", bucket: "Other"}},
		{"DatadogMonitor", `status: {conditions: [{type: Error, status: "True", message: bad}]}`, itemStatus{state: itemError, message: "bad"}},
		{"DatadogDashboard", `status: {syncStatus: OK}`, itemStatus{state: itemOK}},
		{"DatadogDashboard", `status: {syncStatus: OK, conditions: [{type: Error, status: "True", reason: Failed}]}`, itemStatus{state: itemError, message: "Failed"}},
		{"DatadogSLO", `status: {syncStatus: error updating SLO}`, itemStatus{state: itemError, message: "error updating SLO"}},
		{"DatadogSLO", `{}`, itemStatus{}},
		{"DatadogGenericResource", `{spec: {type: notebook}, status: {syncStatus: OK}}`, itemStatus{state: itemOK, bucket: "notebook"}},
		{"DatadogMetric", `status: {conditions: [{type: Error, status: "True", message: query failed}]}`, itemStatus{state: itemError, message: "query failed"}},
		{"DatadogMetric", `status: {conditions: [{type: Valid, status: "False"}]}`, itemStatus{state: itemError, message: "invalid"}},
		{"DatadogInstrumentation", `status: {conditions: [{type: ChecksReady, status: "False", reason: NotReady}]}`, itemStatus{state: itemError, message: "NotReady"}},
		{"DatadogCSIDriver", `status: {conditions: [{type: Ready, status: "False", reason: DaemonSetError, message: ds failed}]}`, itemStatus{state: itemError, message: "ds failed"}},
		{"DatadogCSIDriver", `{}`, itemStatus{}},
		{"DatadogBYOCCluster", `status: {conditions: [{type: IndexerReady, status: "True"}, {type: SearcherReady, status: "True"}]}`, itemStatus{state: itemOK}},
		{"DatadogBYOCCluster", `status: {conditions: [{type: IndexerReady, status: "True"}, {type: SearcherReady, status: "False", reason: Pending}]}`, itemStatus{state: itemError, message: "Pending"}},
		{"DatadogBYOCCluster", `status: {conditions: [{type: Degraded, status: "True"}]}`, itemStatus{state: itemError, message: "Degraded"}},
		{"DatadogBYOCCluster", `{}`, itemStatus{}},
	}
	for _, tt := range tests {
		t.Run(tt.kind+" "+tt.doc, func(t *testing.T) {
			assert.Equal(t, tt.want, summaryExtractors[tt.kind](obj(t, tt.doc)))
		})
	}
	for _, k := range SummaryKinds {
		assert.Contains(t, summaryExtractors, k.Kind)
	}
}

func TestBuildSummaryStates(t *testing.T) {
	s := &Snapshot{
		Objects: map[string][]*unstructured.Unstructured{
			"sum/DatadogBYOCCluster": {obj(t, `{metadata: {name: c, namespace: ns}, status: {conditions: [{type: Ready, status: "True"}]}}`)},
		},
		Sources: map[string]SourceInfo{
			"sum/DatadogMonitor":     {State: SourceForbidden},
			"sum/DatadogDashboard":   {State: SourceNotInstalled},
			"sum/DatadogBYOCCluster": {State: SourceOK, Notice: "listed in the DDA namespace only"},
			"sum/DatadogSLO":         {State: SourceError, Err: "timeout"},
		},
	}
	got := map[string]SummaryView{}
	for _, row := range buildSummary(s) {
		got[row.Kind] = row
	}
	assert.Equal(t, SourceForbidden, got["DatadogMonitor"].State)
	assert.NotContains(t, got, "DatadogDashboard")
	assert.Equal(t, SourceError, got["DatadogSLO"].State)
	assert.Equal(t, SummaryView{Kind: "DatadogBYOCCluster", Label: "BYOC clusters", State: SourceOK, Notice: "listed in the DDA namespace only", Total: 1, OK: 1}, got["DatadogBYOCCluster"])
	// Never acquired: Loading, shown.
	assert.Equal(t, SourceLoading, got["DatadogMetric"].State)
}
