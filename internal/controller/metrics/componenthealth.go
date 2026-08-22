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
	// ComponentHealthIssuesDetected counts component-level health issues as they
	// first appear on the managed cluster-level components (cluster-agent,
	// cluster-checks-runner), labeled by component and issue type. It increments
	// once per component-level issue instance (an issue type going from absent to
	// present on a component), not once per affected pod, so it measures the rate
	// at which new problems surface — useful for spotting flapping.
	ComponentHealthIssuesDetected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: datadogAgentSubsystem,
			Name:      "component_health_issues_detected_total",
			Help:      "Number of component-level health issues detected for managed cluster-level components, by component and issue type.",
		},
		[]string{"component", "issue_type"},
	)

	// ComponentHealthIssuesActive is the number of pods currently exhibiting each
	// issue type per component. It is a gauge: it rises as pods hit an issue and
	// falls as they recover, reaching 0 when a component-level issue clears. Query
	// it as > 0 to know whether a component currently has an open issue of a given
	// type. Labels are kept to component and issue type (no pod/namespace) to bound
	// cardinality — the affected pods live in the emitted issue, not in tags.
	ComponentHealthIssuesActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: datadogAgentSubsystem,
			Name:      "component_health_issues_active",
			Help:      "Number of pods currently exhibiting each health issue type per managed cluster-level component.",
		},
		[]string{"component", "issue_type"},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(ComponentHealthIssuesDetected)
	metrics.Registry.MustRegister(ComponentHealthIssuesActive)
}
