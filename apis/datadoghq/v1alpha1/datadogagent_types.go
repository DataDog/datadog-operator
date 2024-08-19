// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

// TODO move these to a different file

// DatadogAgentConditionType type use to represent a DatadogAgent condition.
type DatadogAgentConditionType string

const (
	// DatadogMetricsActive forwarding metrics and events to Datadog is active.
	DatadogMetricsActive DatadogAgentConditionType = "ActiveDatadogMetrics"
	// DatadogMetricsError cannot forward deployment metrics and events to Datadog.
	DatadogMetricsError DatadogAgentConditionType = "DatadogMetricsError"
)
