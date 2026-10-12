// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCondSeverity(t *testing.T) {
	tests := []struct {
		typ     string
		status  metav1.ConditionStatus
		visible bool
		want    Severity
	}{
		{"DatadogAgentReconcileError", metav1.ConditionTrue, true, SeverityError},
		{"DatadogAgentReconcileError", metav1.ConditionFalse, false, ""},
		{"DatadogAgentInternalReconcileError", metav1.ConditionTrue, true, SeverityError},
		{"Error", metav1.ConditionTrue, true, SeverityError},
		{"AgentReconcile", metav1.ConditionFalse, true, SeverityError},
		{"AgentReconcile", metav1.ConditionTrue, false, ""},
		{"Valid", metav1.ConditionFalse, true, SeverityError},
		{"Applied", metav1.ConditionFalse, true, SeverityWarning},
		{"Applied", metav1.ConditionTrue, false, ""},
		{"DeprecatedConfigInUse", metav1.ConditionTrue, true, SeverityWarning},
		{"FeatureNotSupportedOnProvider", metav1.ConditionTrue, true, SeverityWarning},
		{"OverrideReconcileConflict", metav1.ConditionTrue, true, SeverityWarning},
		{"OverrideReconcileConflict", metav1.ConditionFalse, false, ""},
		{"ClusterProviderDetected", metav1.ConditionTrue, false, ""},
		{"ActiveDatadogMetrics", metav1.ConditionFalse, false, ""},
		{"DatadogMetricsError", metav1.ConditionTrue, false, ""},
		{"Progressing", metav1.ConditionFalse, true, SeverityInfo},
		{"ReplicaFailure", metav1.ConditionTrue, true, SeverityInfo},
		{"SomethingNew", metav1.ConditionTrue, true, SeverityInfo},
		{"SomethingNew", metav1.ConditionFalse, true, SeverityInfo},
	}
	for _, tt := range tests {
		t.Run(tt.typ+"="+string(tt.status), func(t *testing.T) {
			c := metav1.Condition{Type: tt.typ, Status: tt.status}
			require.Equal(t, tt.visible, condVisible(c))
			if tt.visible {
				assert.Equal(t, tt.want, condSeverity(c))
			}
		})
	}
}

// testTree is DDA -> {DDAI a (default), DAP p -> DDAI p, DDAI b (sibling)}.
func testTree() (dda, ddaiA, dap, ddaiP, ddaiB *condOwner) {
	newOwner := func(kind, name string, parent *condOwner, ddai string, isDDAI bool) *condOwner {
		o := &condOwner{ref: ObjectRef{Kind: kind, Namespace: "ns", Name: name}, ddai: ddai, isDDAI: isDDAI}
		if parent != nil {
			o.path = append(o.path, parent.path...)
		}
		o.path = append(o.path, o.id())
		return o
	}
	dda = newOwner(kindDDA, "dda", nil, "a", false)
	ddaiA = newOwner(kindDDAI, "a", dda, "a", true)
	dap = newOwner(kindDAP, "p", dda, "p", false)
	ddaiP = newOwner(kindDDAI, "p", dap, "p", true)
	ddaiB = newOwner(kindDDAI, "b", dda, "b", true)
	return dda, ddaiA, dap, ddaiP, ddaiB
}

func cond(typ, reason, msg string) metav1.Condition {
	return metav1.Condition{Type: typ, Status: metav1.ConditionTrue, Reason: reason, Message: msg}
}

func objects(conds []Cond) []string {
	out := make([]string, len(conds))
	for i, c := range conds {
		out[i] = c.Object.Name + ":" + c.Type + ":" + c.Message
	}
	return out
}

func TestDedup(t *testing.T) {
	dda, ddaiA, dap, ddaiP, ddaiB := testTree()
	tests := []struct {
		name string
		raws []rawCond
		want []string
	}{
		{
			name: "(a) DDA mirror of the default DDAI",
			raws: []rawCond{
				{owner: dda, cond: cond(condDDAIReconcileError, "r", "a: boom")},
				{owner: ddaiA, cond: cond(condDDAReconcileError, "r", "boom")},
			},
			want: []string{"a:DatadogAgentReconcileError:boom"},
		},
		{
			name: "(a) DDA mirror naming a non-default DDAI falls through",
			raws: []rawCond{
				{owner: dda, cond: cond(condDDAIReconcileError, "r", "b: boom")},
				{owner: ddaiB, cond: cond(condDDAReconcileError, "r", "boom")},
			},
			want: []string{"b:DatadogAgentReconcileError:boom", "dda:DatadogAgentInternalReconcileError:b: boom"},
		},
		{
			name: "(a) stale mirror with another message falls through",
			raws: []rawCond{
				{owner: dda, cond: cond(condDDAIReconcileError, "r", "a: old")},
				{owner: ddaiA, cond: cond(condDDAReconcileError, "r", "new")},
			},
			want: []string{"a:DatadogAgentReconcileError:new", "dda:DatadogAgentInternalReconcileError:a: old"},
		},
		{
			name: "(b) DAP mirror of its profile DDAI",
			raws: []rawCond{
				{owner: dap, cond: cond(condDDAIReconcileError, "r", "p: boom")},
				{owner: ddaiP, cond: cond(condDDAReconcileError, "r", "boom")},
			},
			want: []string{"p:DatadogAgentReconcileError:boom"},
		},
		{
			name: "(b) unknown mirror format falls through",
			raws: []rawCond{
				{owner: dap, cond: cond(condDDAIReconcileError, "r", "p failed: boom")},
				{owner: ddaiP, cond: cond(condDDAReconcileError, "r", "boom")},
			},
			want: []string{"p:DatadogAgentInternalReconcileError:p failed: boom", "p:DatadogAgentReconcileError:boom"},
		},
		{
			name: "(c) constant DDA error with a DDAI error",
			raws: []rawCond{
				{owner: dda, cond: cond(condDDAReconcileError, "r", ddaReconcileErrorMessage)},
				{owner: ddaiP, cond: cond(condDDAReconcileError, "r", "boom")},
			},
			want: []string{"p:DatadogAgentReconcileError:boom"},
		},
		{
			name: "(c) constant DDA error alone is kept",
			raws: []rawCond{
				{owner: dda, cond: cond(condDDAReconcileError, "r", ddaReconcileErrorMessage)},
			},
			want: []string{"dda:DatadogAgentReconcileError:DatadogAgent reconcile error"},
		},
		{
			name: "(c) DDA error with another message falls through",
			raws: []rawCond{
				{owner: dda, cond: cond(condDDAReconcileError, "r", "DatadogAgent failed")},
				{owner: ddaiA, cond: cond(condDDAReconcileError, "r", "boom")},
			},
			want: []string{"a:DatadogAgentReconcileError:boom", "dda:DatadogAgentReconcileError:DatadogAgent failed"},
		},
		{
			name: "exact duplicate kept on the lowest object",
			raws: []rawCond{
				{owner: dda, cond: cond("Custom", "r", "m")},
				{owner: dap, cond: cond("Custom", "r", "m")},
				{owner: ddaiP, cond: cond("Custom", "r", "m")},
			},
			want: []string{"p:Custom:m"},
		},
		{
			name: "exact duplicate on siblings is kept on both",
			raws: []rawCond{
				{owner: ddaiA, cond: cond("Custom", "r", "m")},
				{owner: ddaiB, cond: cond("Custom", "r", "m")},
			},
			want: []string{"a:Custom:m", "b:Custom:m"},
		},
		{
			name: "different reasons are not duplicates",
			raws: []rawCond{
				{owner: dda, cond: cond("Custom", "r1", "m")},
				{owner: ddaiA, cond: cond("Custom", "r2", "m")},
			},
			want: []string{"a:Custom:m", "dda:Custom:m"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatch(t, tt.want, objects(dedup(tt.raws)))
		})
	}
}

func TestDedupSortsBySeverity(t *testing.T) {
	dda, ddaiA, _, _, _ := testTree()
	got := dedup([]rawCond{
		{owner: dda, cond: cond("Custom", "r", "info")},
		{owner: ddaiA, cond: cond(condDeprecatedConfigInUse, "r", "warn")},
		{owner: dda, severity: SeverityWarning, cond: cond(condRollout, "Stalled", "synth")},
		{owner: ddaiA, cond: cond(condDDAReconcileError, "r", "err")},
	})
	var sev []Severity
	for _, c := range got {
		sev = append(sev, c.Severity)
	}
	assert.Equal(t, []Severity{SeverityError, SeverityWarning, SeverityWarning, SeverityInfo}, sev)
	assert.Equal(t, "dda", got[1].Object.Name, "same severity: sorted by object")
}
