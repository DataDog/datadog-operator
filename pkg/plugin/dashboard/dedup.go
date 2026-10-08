// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"cmp"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types written by the operator.
const (
	condDDAReconcileError          = "DatadogAgentReconcileError"
	condDDAIReconcileError         = "DatadogAgentInternalReconcileError"
	condClusterProviderDetected    = "ClusterProviderDetected"
	condDeprecatedConfigInUse      = "DeprecatedConfigInUse"
	condFeatureNotSupported        = "FeatureNotSupportedOnProvider"
	condOverrideReconcileConflict  = "OverrideReconcileConflict"
	condExperimentConfigStranded   = "ExperimentConfigStranded"
	condValid                      = "Valid"
	condApplied                    = "Applied"
	condWindowsLogCollectionSkiped = "WindowsLogCollectionPathSkipped"

	// ddaReconcileErrorMessage is the constant message of the DDA
	// DatadogAgentReconcileError condition: it only rolls up DDAI errors.
	ddaReconcileErrorMessage = "DatadogAgent reconcile error"
)

// Synthesized condition types, for facts that are not conditions.
const (
	condRollout     = "Rollout"
	condUnavailable = "Unavailable"
)

// positiveConditions are healthy when True; they are hidden then.
var positiveConditions = map[string]bool{
	"AgentReconcile":               true,
	"ClusterAgentReconcile":        true,
	"ClusterChecksRunnerReconcile": true,
	"OtelAgentGatewayReconcile":    true,
	"WindowsAgentReconcile":        true,
	"OpenShiftSCC":                 true,
	condValid:                      true,
	condApplied:                    true,
	"Available":                    true,
	"Progressing":                  true,
}

// negativeConditions report a problem when True; they are hidden otherwise.
var negativeConditions = map[string]bool{
	condDDAReconcileError:          true,
	condDDAIReconcileError:         true,
	"Error":                        true,
	condDeprecatedConfigInUse:      true,
	condFeatureNotSupported:        true,
	condOverrideReconcileConflict:  true,
	condExperimentConfigStranded:   true,
	condWindowsLogCollectionSkiped: true,
	"ReplicaFailure":               true,
}

// hiddenConditions are never shown: the metrics forwarder status, and
// ClusterProviderDetected, which is shown in the header.
var hiddenConditions = map[string]bool{
	"ActiveDatadogMetrics":      true,
	"DatadogMetricsError":       true,
	condClusterProviderDetected: true,
}

// condVisible reports whether a condition is worth showing. Healthy states
// of known conditions are hidden. Unknown types are always shown, so that new conditions are
// never silently dropped.
func condVisible(c metav1.Condition) bool {
	switch {
	case hiddenConditions[c.Type]:
		return false
	case positiveConditions[c.Type]:
		return c.Status != metav1.ConditionTrue
	case negativeConditions[c.Type]:
		return c.Status == metav1.ConditionTrue
	default:
		return true
	}
}

// condSeverity classifies a visible condition.
func condSeverity(c metav1.Condition) Severity {
	isTrue := c.Status == metav1.ConditionTrue
	switch {
	case isTrue && (strings.HasSuffix(c.Type, "ReconcileError") || c.Type == "Error"):
		return SeverityError
	case !isTrue && c.Type == condValid:
		return SeverityError
	case !isTrue && positiveConditions[c.Type] && strings.HasSuffix(c.Type, "Reconcile"):
		// A component failed to reconcile, e.g. AgentReconcile=False with
		// "agent feature error".
		return SeverityError
	case !isTrue && c.Type == condApplied:
		return SeverityWarning
	case isTrue && (c.Type == condDeprecatedConfigInUse || c.Type == condFeatureNotSupported || c.Type == condOverrideReconcileConflict):
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// condOwner is an object of the tree that reports conditions.
type condOwner struct {
	ref ObjectRef
	// path is the chain of owner IDs from the root to this owner, included.
	path []string
	// ddai is the DDAI whose errors this owner mirrors: the default DDAI
	// for the DDA, the profile DDAI for a DAP, itself for a DDAI.
	ddai string
	// isDDAI is true for DDAI owners.
	isDDAI bool
}

func (o *condOwner) id() string { return o.ref.String() }

// isAncestorOf reports whether o is a strict ancestor of other.
func (o *condOwner) isAncestorOf(other *condOwner) bool {
	return len(o.path) < len(other.path) && slices.Contains(other.path, o.id())
}

// rawCond is a condition before dedup.
type rawCond struct {
	owner *condOwner
	cond  metav1.Condition
	// severity overrides the classification of synthesized conditions.
	severity Severity
}

type dedupEntry struct {
	raw     rawCond
	sev     Severity
	dropped bool
	also    []ObjectRef
}

// dedup shows each condition once, on the lowest object reporting it:
//
//  1. DDAI DatadogAgentReconcileError conditions are indexed by DDAI and
//     message.
//  2. DDA (case a) and DAP (case b) DatadogAgentInternalReconcileError
//     mirrors "<ddai>: X" are dropped when the DDA's default DDAI, or the
//     DAP's profile DDAI, reports X.
//  3. The DDA's constant-message DatadogAgentReconcileError is dropped when
//     a DDAI under the DDA reports an error (case c).
//  4. Identical (type, reason, message) conditions are kept on the lowest
//     owner when one owner is an ancestor of the other.
//
// Mirrors in an unknown format are kept rather than hiding an error. A
// dropped condition always has a copy deeper in the same subtree, so health
// is unchanged. The result is sorted most severe first.
func dedup(raws []rawCond) []Cond {
	entries := make([]*dedupEntry, 0, len(raws))
	for _, r := range raws {
		if r.severity == "" && !condVisible(r.cond) {
			continue
		}
		sev := r.severity
		if sev == "" {
			sev = condSeverity(r.cond)
		}
		entries = append(entries, &dedupEntry{raw: r, sev: sev})
	}

	// 1. Index DDAI errors.
	ddaiErrors := map[string]*dedupEntry{}
	var ddaiErrorList []*dedupEntry
	for _, e := range entries {
		if e.raw.owner.isDDAI && e.raw.cond.Type == condDDAReconcileError && e.raw.cond.Status == metav1.ConditionTrue {
			ddaiErrors[e.raw.owner.ddai+"\x00"+e.raw.cond.Message] = e
			ddaiErrorList = append(ddaiErrorList, e)
		}
	}

	for _, e := range entries {
		o, c := e.raw.owner, e.raw.cond
		switch {
		case !o.isDDAI && c.Type == condDDAIReconcileError && o.ddai != "":
			// 2. Mirrors, cases (a) and (b).
			name, msg, ok := strings.Cut(c.Message, ": ")
			if !ok || name != o.ddai {
				continue
			}
			if kept, found := ddaiErrors[name+"\x00"+msg]; found {
				e.dropped = true
				kept.also = append(kept.also, o.ref)
			}
		case !o.isDDAI && c.Type == condDDAReconcileError && c.Message == ddaReconcileErrorMessage:
			// 3. Constant rollup, case (c): attached to the default DDAI
			// error if any, else to the first DDAI error in tree order.
			var target *dedupEntry
			for _, kept := range ddaiErrorList {
				if !o.isAncestorOf(kept.raw.owner) {
					continue
				}
				if target == nil {
					target = kept
				}
				if kept.raw.owner.ddai == o.ddai {
					target = kept
					break
				}
			}
			if target != nil {
				e.dropped = true
				target.also = append(target.also, o.ref)
			}
		}
	}

	// 4. Exact dedup, deepest owner first.
	byKey := map[string][]*dedupEntry{}
	for _, e := range entries {
		if e.dropped {
			continue
		}
		c := e.raw.cond
		key := c.Type + "\x00" + c.Reason + "\x00" + c.Message
		byKey[key] = append(byKey[key], e)
	}
	for _, group := range byKey {
		slices.SortStableFunc(group, func(a, b *dedupEntry) int {
			return cmp.Compare(len(b.raw.owner.path), len(a.raw.owner.path))
		})
		for i, e := range group {
			for _, kept := range group[:i] {
				if !kept.dropped && e.raw.owner.isAncestorOf(kept.raw.owner) {
					e.dropped = true
					kept.also = append(kept.also, e.raw.owner.ref)
					break
				}
			}
		}
	}

	out := make([]Cond, 0, len(entries))
	for _, e := range entries {
		if e.dropped {
			continue
		}
		c := e.raw.cond
		also := e.also
		slices.SortFunc(also, func(a, b ObjectRef) int { return cmp.Compare(a.String(), b.String()) })
		also = slices.Compact(also)
		out = append(out, Cond{
			Severity:           e.sev,
			Object:             e.raw.owner.ref,
			Type:               c.Type,
			Status:             string(c.Status),
			Reason:             c.Reason,
			Message:            c.Message,
			LastTransitionTime: timePtr(c.LastTransitionTime.Time),
			AlsoReportedBy:     also,
		})
	}
	sortConds(out)
	return out
}

// sortConds sorts conditions by severity, then object, then type.
func sortConds(conds []Cond) {
	slices.SortStableFunc(conds, func(a, b Cond) int {
		if c := cmp.Compare(b.Severity.rank(), a.Severity.rank()); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Object.String(), b.Object.String()); c != 0 {
			return c
		}
		return cmp.Compare(a.Type, b.Type)
	})
}
