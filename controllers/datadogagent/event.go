// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	corev1 "k8s.io/api/core/v1"
)

// eventInfo contains the required information
// to create kubernetes and datadog events
type eventInfo struct {
	objName      string
	objNamespace string
	objKind      string
	eventType    datadog.EventType
}

// buildEventInfo creates a new eventInfo instance
func buildEventInfo(name, ns, kind string, eventType datadog.EventType) eventInfo {
	return eventInfo{
		objName:      name,
		objNamespace: ns,
		objKind:      kind,
		eventType:    eventType,
	}
}

// getReason returns the event reason
func (ei *eventInfo) getReason() string {
	return fmt.Sprintf("%s %s", ei.eventType, ei.objKind)
}

// getMessage returns the event message
func (ei *eventInfo) getMessage() string {
	return fmt.Sprintf("%s/%s", ei.objNamespace, ei.objName)
}

// getDDEvent builds and returns a Datadog event
func (ei *eventInfo) getDDEvent() datadog.Event {
	reason := ei.getReason()
	message := ei.getMessage()
	return datadog.Event{
		Title: fmt.Sprintf("%s %s", reason, message),
		Type:  ei.eventType,
	}
}

// recordEvent wraps the manager event recorder
// recordEvent calls the metric forwarders to send Datadog events
func (r *Reconciler) recordEvent(dda *datadoghqv1alpha1.DatadogAgent, info eventInfo) {
	r.recorder.Event(dda, corev1.EventTypeNormal, info.getReason(), info.getMessage())
	r.forwarders.ProcessEvent(dda, info.getDDEvent())
}
