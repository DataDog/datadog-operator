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
	// datadogagentprofile enabled
	DAPEnabled = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: datadogAgentProfileSubsystem,
			Name:      "enabled",
			Help:      "1 if DatadogAgentProfiles are enabled. 0 if DatadogAgentProfiles are disabled",
		},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(DAPEnabled)
}
