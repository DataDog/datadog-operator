// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

// recordEvent wraps the manager event recorder
// recordEvent calls the metric forwarders to send Datadog events
// the reason argument should contain the action and the object type e.g: Create DaemonSet
// the message argument should contain object namespace and name e.g: default/datadog-agent
func (r *ReconcileDatadogAgentDeployment) recordEvent(dad *datadoghqv1alpha1.DatadogAgentDeployment, eventtype, reason, message string, ddEventType datadog.EventType) {
	r.recorder.Event(dad, eventtype, reason, message)
	ddEvent := datadog.Event{
		Title: fmt.Sprintf("%s %s", reason, message),
		Type:  ddEventType,
	}
	r.forwarders.ProcessEvent(getNamespacedName(dad), ddEvent)
}
