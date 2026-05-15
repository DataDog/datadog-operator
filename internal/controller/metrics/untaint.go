// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// TaintRemovalsTotal is the total number of taints removed from nodes.
	TaintRemovalsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: untaintSubsystem,
			Name:      "taint_removals_total",
			Help:      "Total number of taints removed from nodes",
		},
	)

	// TaintRemovalLatency is the time between agent pod becoming Ready and taint removal.
	TaintRemovalLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Subsystem: untaintSubsystem,
			Name:      "taint_removal_latency_seconds",
			Help:      "Time between agent pod becoming Ready and taint removal from the node",
			Buckets:   prometheus.DefBuckets,
		},
	)
)

func init() {
	metrics.Registry.MustRegister(TaintRemovalsTotal)
	metrics.Registry.MustRegister(TaintRemovalLatency)
}
