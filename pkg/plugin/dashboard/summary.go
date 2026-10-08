// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"cmp"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// itemState is the summary state of one CR.
type itemState int

const (
	itemUnknown itemState = iota
	itemOK
	itemError
)

// itemStatus is the summary status of one CR: its state, the message of an
// error, and its breakdown bucket if the kind has one.
type itemStatus struct {
	state   itemState
	message string
	bucket  string
}

// summaryExtractors read the status of each summary kind.
// Objects are read unstructured because the kinds use different condition
// structs.
var summaryExtractors = map[string]func(*unstructured.Unstructured) itemStatus{
	"DatadogMonitor":         monitorStatus,
	"DatadogDashboard":       syncStatus,
	"DatadogSLO":             syncStatus,
	"DatadogGenericResource": genericResourceStatus,
	"DatadogMetric":          metricStatus,
	"DatadogInstrumentation": instrumentationStatus,
	"DatadogCSIDriver":       csiDriverStatus,
	"DatadogBYOCCluster":     byocClusterStatus,
}

// Monitor state buckets.
const (
	monitorBucketOther = "Other"
)

var monitorBuckets = map[string]bool{"OK": true, "Alert": true, "Warn": true, "No Data": true}

// buildSummary builds one row per installed summary kind.
func buildSummary(s *Snapshot) []SummaryView {
	out := make([]SummaryView, 0, len(SummaryKinds))
	for _, k := range SummaryKinds {
		info := s.source(k.SourceKey())
		if info.State == SourceNotInstalled {
			continue // not installed: hidden
		}
		row := SummaryView{Kind: k.Kind, Label: k.Label, State: info.State, Notice: info.Notice}
		if info.State == SourceOK {
			fillSummary(&row, s.Objects[k.SourceKey()], summaryExtractors[k.Kind])
		}
		out = append(out, row)
	}
	return out
}

func fillSummary(row *SummaryView, objs []*unstructured.Unstructured, extract func(*unstructured.Unstructured) itemStatus) {
	for _, obj := range objs {
		st := extract(obj)
		row.Total++
		switch st.state {
		case itemOK:
			row.OK++
		case itemError:
			row.Errors++
			row.Failures = append(row.Failures, SummaryFailure{Namespace: obj.GetNamespace(), Name: obj.GetName(), Message: st.message})
		default:
			row.Unknown++
		}
		if st.bucket != "" {
			if row.Breakdown == nil {
				row.Breakdown = map[string]int{}
			}
			row.Breakdown[st.bucket]++
		}
	}
	slices.SortFunc(row.Failures, func(a, b SummaryFailure) int {
		if c := cmp.Compare(a.Namespace, b.Namespace); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
}

// condition is a generic status condition read unstructured; it covers
// metav1.Condition and the custom monitor and metric condition structs.
type condition struct {
	Type, Status, Reason, Message string
}

func conditions(obj *unstructured.Unstructured) []condition {
	raw, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	out := make([]condition, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		str := func(k string) string { s, _ := m[k].(string); return s }
		out = append(out, condition{Type: str("type"), Status: str("status"), Reason: str("reason"), Message: str("message")})
	}
	return out
}

func findCondition(conds []condition, typ string) (condition, bool) {
	for _, c := range conds {
		if c.Type == typ {
			return c, true
		}
	}
	return condition{}, false
}

func statusString(obj *unstructured.Unstructured, fields ...string) string {
	s, _, _ := unstructured.NestedString(obj.Object, append([]string{"status"}, fields...)...)
	return s
}

// syncState classifies a syncStatus value: "OK", or an "error ..." string.
func syncState(sync string) itemState {
	switch {
	case sync == "OK":
		return itemOK
	case strings.HasPrefix(strings.ToLower(sync), "error"):
		return itemError
	default:
		return itemUnknown
	}
}

// errorCondition returns the message of an Error=True condition.
func errorCondition(conds []condition) (string, bool) {
	if c, ok := findCondition(conds, "Error"); ok && c.Status == "True" {
		return firstNonEmpty(c.Message, c.Reason, "Error"), true
	}
	return "", false
}

// syncStatus reads DatadogDashboard and DatadogSLO: status.syncStatus and
// an Error condition.
func syncStatus(obj *unstructured.Unstructured) itemStatus {
	sync := statusString(obj, "syncStatus")
	if msg, ok := errorCondition(conditions(obj)); ok {
		return itemStatus{state: itemError, message: msg}
	}
	st := syncState(sync)
	if st == itemError {
		return itemStatus{state: itemError, message: sync}
	}
	return itemStatus{state: st}
}

// monitorStatus reads DatadogMonitor: status.monitorStateSyncStatus, the
// Error condition, and status.monitorState for the breakdown.
func monitorStatus(obj *unstructured.Unstructured) itemStatus {
	st := itemStatus{}
	if state := statusString(obj, "monitorState"); state != "" {
		st.bucket = state
		if !monitorBuckets[state] {
			st.bucket = monitorBucketOther
		}
	}
	conds := conditions(obj)
	sync := statusString(obj, "monitorStateSyncStatus")
	switch msg, isErr := errorCondition(conds); {
	case isErr:
		st.state, st.message = itemError, msg
	case syncState(sync) == itemError:
		st.state, st.message = itemError, sync
	case syncState(sync) == itemOK:
		st.state = itemOK
	default:
		if c, ok := findCondition(conds, "Active"); ok && c.Status == "True" {
			st.state = itemOK
		}
	}
	return st
}

// genericResourceStatus reads DatadogGenericResource: status.syncStatus,
// StateSynced=False, and spec.type for the breakdown.
func genericResourceStatus(obj *unstructured.Unstructured) itemStatus {
	st := syncStatus(obj)
	if st.state != itemError {
		if c, ok := findCondition(conditions(obj), "StateSynced"); ok && c.Status == "False" {
			st = itemStatus{state: itemError, message: firstNonEmpty(c.Message, c.Reason)}
		}
	}
	st.bucket, _, _ = unstructured.NestedString(obj.Object, "spec", "type")
	return st
}

// metricStatus reads DatadogMetric: Valid=False (invalid), Error=True.
// Its status is written by the Cluster Agent.
func metricStatus(obj *unstructured.Unstructured) itemStatus {
	conds := conditions(obj)
	if c, ok := findCondition(conds, "Valid"); ok && c.Status == "False" {
		return itemStatus{state: itemError, message: firstNonEmpty(c.Message, c.Reason, "invalid")}
	}
	if msg, ok := errorCondition(conds); ok {
		return itemStatus{state: itemError, message: msg}
	}
	for _, typ := range []string{"Valid", "Active"} {
		if c, ok := findCondition(conds, typ); ok && c.Status == "True" {
			return itemStatus{state: itemOK}
		}
	}
	return itemStatus{}
}

// readyStatus classifies a Ready-like condition: True is OK, False an error,
// missing Unknown.
func readyStatus(obj *unstructured.Unstructured, typ string) itemStatus {
	c, ok := findCondition(conditions(obj), typ)
	switch {
	case !ok:
		return itemStatus{}
	case c.Status == "True":
		return itemStatus{state: itemOK}
	case c.Status == "False":
		return itemStatus{state: itemError, message: firstNonEmpty(c.Message, c.Reason)}
	default:
		return itemStatus{}
	}
}

// instrumentationStatus reads DatadogInstrumentation: ChecksReady. It is not
// written by this operator, so a missing status is Unknown.
func instrumentationStatus(obj *unstructured.Unstructured) itemStatus {
	return readyStatus(obj, "ChecksReady")
}

// csiDriverStatus reads DatadogCSIDriver: the Ready condition.
func csiDriverStatus(obj *unstructured.Unstructured) itemStatus {
	return readyStatus(obj, "Ready")
}

// byocClusterStatus reads DatadogBYOCCluster generically: any *Ready*=False
// or Degraded/Error=True is an error; a Ready condition otherwise is OK.
func byocClusterStatus(obj *unstructured.Unstructured) itemStatus {
	st := itemStatus{}
	for _, c := range conditions(obj) {
		switch {
		case strings.Contains(c.Type, "Ready") && c.Status == "False",
			(c.Type == "Degraded" || c.Type == "Error") && c.Status == "True":
			return itemStatus{state: itemError, message: firstNonEmpty(c.Message, c.Reason, c.Type)}
		case strings.Contains(c.Type, "Ready") && c.Status == "True":
			st.state = itemOK
		}
	}
	return st
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
