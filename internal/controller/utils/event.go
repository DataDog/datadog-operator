// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"

	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

// EventInfo contains the required information
// to create Kubernetes and Datadog events.
type EventInfo struct {
	objName      string
	objNamespace string
	objKind      string
	eventType    datadog.EventType
}

// BuildEventInfo creates a new EventInfo instance.
func BuildEventInfo(name, ns, kind string, eventType datadog.EventType) EventInfo {
	return EventInfo{
		objName:      name,
		objNamespace: ns,
		objKind:      kind,
		eventType:    eventType,
	}
}

// GetReason returns the event reason.
func (ei *EventInfo) GetReason() string {
	return fmt.Sprintf("%s %s", ei.eventType, ei.objKind)
}

// GetMessage returns the event message.
func (ei *EventInfo) GetMessage() string {
	return fmt.Sprintf("%s/%s", ei.objNamespace, ei.objName)
}

// GetDDEvent builds and returns a Datadog event.
func (ei *EventInfo) GetDDEvent() datadog.Event {
	reason := ei.GetReason()
	message := ei.GetMessage()

	return datadog.Event{
		Title: fmt.Sprintf("%s %s", reason, message),
		Type:  ei.eventType,
	}
}
