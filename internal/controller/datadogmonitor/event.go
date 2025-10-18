// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

const datadogMonitorKind = "DatadogMonitor"

// buildEventInfo creates a new EventInfo instance.
func buildEventInfo(name, ns string, eventType datadog.EventType) utils.EventInfo {
	return utils.BuildEventInfo(name, ns, datadogMonitorKind, eventType)
}

// recordEvent wraps the manager event recorder.
func (r *Reconciler) recordEvent(dm *datadoghqv1alpha1.DatadogMonitor, info utils.EventInfo) {
	r.recorder.Event(dm, corev1.EventTypeNormal, info.GetReason(), info.GetMessage())
}
