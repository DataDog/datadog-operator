// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Label values for TaintTimeoutsTotal.
const (
	// UntaintTimeoutReasonReadiness signals that a pod existed on the node but
	// never became Ready within --untaintControllerTimeout.
	UntaintTimeoutReasonReadiness = "readiness"
	// UntaintTimeoutReasonScheduling signals that no agent pod was scheduled on
	// the node within --untaintControllerSchedulingTimeout.
	UntaintTimeoutReasonScheduling = "scheduling"

	// UntaintTimeoutPolicyRemove untaints the node despite the agent not being ready.
	UntaintTimeoutPolicyRemove = "remove"
	// UntaintTimeoutPolicyKeep leaves the taint in place but emits observability signals.
	UntaintTimeoutPolicyKeep = "keep"

	// UntaintRemovalReasonAgentReady signals the taint was removed because the
	// readiness criteria were met (agent — and optionally CSI — pods Ready).
	UntaintRemovalReasonAgentReady = "agent_ready"
	// UntaintRemovalReasonTimeout signals the taint was removed because a
	// readiness or scheduling timeout fired under policy=remove.
	UntaintRemovalReasonTimeout = "timeout"

	// untaintNodeLabel is the label key carrying the node name on the
	// node-scoped untaint metrics.
	untaintNodeLabel = "node"
)

var (
	// TaintRemovalsTotal is the total number of taints removed from nodes,
	// broken down by node and removal reason (agent_ready or timeout).
	TaintRemovalsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: untaintSubsystem,
			Name:      "taint_removals_total",
			Help:      "Total number of taints removed from nodes, by node and reason",
		},
		[]string{untaintNodeLabel, "reason"},
	)

	// TaintRemovalLatency is the time between agent pod becoming Ready and taint
	// removal, broken down by node.
	TaintRemovalLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: untaintSubsystem,
			Name:      "taint_removal_latency_seconds",
			Help:      "Time between agent pod becoming Ready and taint removal from the node, by node",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{untaintNodeLabel},
	)

	// TaintTimeoutsTotal counts timeout decisions broken down by reason and policy.
	TaintTimeoutsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: untaintSubsystem,
			Name:      "taint_timeouts_total",
			Help:      "Total number of untaint-controller timeout decisions, by reason and policy",
		},
		[]string{"reason", "policy"},
	)

	// TaintRemovalErrorsTotal counts hard errors encountered while attempting to
	// remove the taint (apiserver Patch failures, JSON marshal failures, …).
	// Benign optimistic-concurrency races (IsConflict/IsInvalid) are NOT counted
	// here — they're handled by requeueing. Inspect the operator's ERROR-level
	// logs for the specific failure cause.
	TaintRemovalErrorsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: untaintSubsystem,
			Name:      "taint_removal_errors_total",
			Help:      "Total number of errors encountered while attempting to remove the agent-not-ready taint from a node",
		},
	)
)

// DeleteNodeSeries removes every child series labeled with the given node from
// the node-scoped untaint metrics: TaintRemovalsTotal (across all reason values)
// and TaintRemovalLatency (across all histogram buckets). It is a no-op when the
// node has no series. Call it when a node is deleted to prevent unbounded growth
// of per-node series for the operator's lifetime as clusters autoscale or
// replace nodes.
func DeleteNodeSeries(node string) {
	match := prometheus.Labels{untaintNodeLabel: node}
	TaintRemovalsTotal.DeletePartialMatch(match)
	TaintRemovalLatency.DeletePartialMatch(match)
}

func init() {
	metrics.Registry.MustRegister(TaintRemovalsTotal)
	metrics.Registry.MustRegister(TaintRemovalLatency)
	metrics.Registry.MustRegister(TaintTimeoutsTotal)
	metrics.Registry.MustRegister(TaintRemovalErrorsTotal)
}
