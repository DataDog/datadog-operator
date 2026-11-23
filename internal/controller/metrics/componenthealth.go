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
	// present on a component) so it measures the rate at which new problems surface.
	// A flap (the issue re-activating while its resolve is still within the grace
	// window) is not re-counted, so a crash-looping component does not inflate it.
	ComponentHealthIssuesDetected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: componentHealthSubsystem,
			Name:      "issues_detected_total",
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
			Subsystem: componentHealthSubsystem,
			Name:      "issues_active",
			Help:      "Number of pods currently exhibiting each health issue type per managed cluster-level component.",
		},
		[]string{"component", "issue_type"},
	)

	// ComponentHealthSnapshotsSent counts health-platform snapshots the operator
	// successfully delivered to the agenthealth intake (a 2xx response).
	ComponentHealthSnapshotsSent = prometheus.NewCounter(
		prometheus.CounterOpts{
			Subsystem: componentHealthSubsystem,
			Name:      "snapshots_sent_total",
			Help:      "Number of component health snapshots successfully delivered to the agenthealth intake.",
		},
	)

	// ComponentHealthSnapshotSendErrors counts failed snapshot deliveries, labeled
	// by reason (no_credentials, build_request, transport, http_status) so a stuck
	// delivery (e.g. a 403 from a wrong-site key) is both visible and diagnosable.
	ComponentHealthSnapshotSendErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: componentHealthSubsystem,
			Name:      "snapshot_send_errors_total",
			Help:      "Number of component health snapshot deliveries that failed, by reason.",
		},
		[]string{"reason"},
	)

	// ComponentHealthLastSuccessfulSendTimestamp is the Unix time of the most recent
	// successful snapshot delivery. Alert on staleness (now - value) to catch a
	// delivery pipeline that has silently stopped reporting.
	ComponentHealthLastSuccessfulSendTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: componentHealthSubsystem,
			Name:      "last_successful_send_timestamp_seconds",
			Help:      "Unix timestamp of the last component health snapshot successfully delivered to the agenthealth intake.",
		},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(ComponentHealthIssuesDetected)
	metrics.Registry.MustRegister(ComponentHealthIssuesActive)
	metrics.Registry.MustRegister(ComponentHealthSnapshotsSent)
	metrics.Registry.MustRegister(ComponentHealthSnapshotSendErrors)
	metrics.Registry.MustRegister(ComponentHealthLastSuccessfulSendTimestamp)
}
