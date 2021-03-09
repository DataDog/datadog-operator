// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import "fmt"

// Event contains the rquired information to send Datadog events
type Event struct {
	Title string
	Type  EventType
}

// EventType enumerates the possible event types to be sent
type EventType string

const (
	// CreationEvent should be used for resource creation events
	CreationEvent EventType = "Create"
	// DetectionEvent should be used for resource detection events
	DetectionEvent EventType = "Detect"
	// UpdateEvent should be used for resource update events
	UpdateEvent EventType = "Update"
	// DeletionEvent should be used for resource deletion events
	DeletionEvent EventType = "Delete"
)

// crDetected returns the detection event of a CR
func crDetected(id string) Event {
	return Event{
		Title: fmt.Sprintf("Detect Custom Resource %s", id),
		Type:  DetectionEvent,
	}
}

// crDeleted returns the delete event of a CR
func crDeleted(id string) Event {
	return Event{
		Title: fmt.Sprintf("Delete Custom Resource %s", id),
		Type:  DeletionEvent,
	}
}
