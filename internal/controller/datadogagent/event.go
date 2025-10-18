// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/internal/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

// buildEventInfo creates a new EventInfo instance
func buildEventInfo(name, ns, kind string, eventType datadog.EventType) utils.EventInfo {
	return utils.BuildEventInfo(name, ns, kind, eventType)
}

// recordEvent wraps the manager event recorder
// recordEvent calls the metric forwarders to send Datadog events
func (r *Reconciler) recordEvent(dda client.Object, info utils.EventInfo) {
	r.recorder.Event(dda, corev1.EventTypeNormal, info.GetReason(), info.GetMessage())
	if r.options.OperatorMetricsEnabled {
		r.forwarders.ProcessEvent(dda, info.GetDDEvent())
	}
}
