// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dashboard builds the view model of the `kubectl datadog dashboard`
// command from a snapshot of cluster objects.
//
// Data flows one way: a Store acquires objects into an immutable Snapshot,
// Build turns the Snapshot into a View with no I/O, and renderers consume
// the View.
package dashboard

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-operator/pkg/plugin/helm"
)

// SourceState is the acquisition state of a source.
type SourceState string

const (
	SourceOK           SourceState = "OK"
	SourceLoading      SourceState = "Loading"
	SourceForbidden    SourceState = "Forbidden"
	SourceNotInstalled SourceState = "NotInstalled"
	SourceError        SourceState = "Error"
	SourceDisconnected SourceState = "Disconnected"
)

// RefreshMode is how a source is refreshed.
type RefreshMode string

const (
	RefreshWatch RefreshMode = "Watch"
	RefreshPoll  RefreshMode = "Poll"
	RefreshOnce  RefreshMode = "Once"
)

// SourceInfo is the acquisition status of one source.
type SourceInfo struct {
	State SourceState
	Mode  RefreshMode
	// LastOK is when the source was last read successfully.
	LastOK time.Time
	// NextPoll is when a polled source is read next; it drives the header
	// countdown.
	NextPoll time.Time
	// Notice is a user-facing note, e.g. "listed in the DDA namespace only".
	Notice string
	// Err is the last acquisition error.
	Err string
}

// HasData reports whether the source's objects were read: it is OK, or
// Disconnected with the objects of its last successful read.
func (i SourceInfo) HasData() bool {
	return i.State == SourceOK || i.State == SourceDisconnected
}

// ClusterInfo describes the cluster and the tool.
type ClusterInfo struct {
	Context       string
	ServerVersion string
	PluginVersion string
}

// Snapshot is an immutable copy of everything Build needs.
type Snapshot struct {
	Taken   time.Time
	Cluster ClusterInfo
	// Objects holds the objects of each source, by source key.
	Objects map[string][]*unstructured.Unstructured
	// Pods holds the stripped agent pods; nil unless pods are watched.
	Pods []*corev1.Pod
	// Helm holds release histories, newest first, by ReleaseKey.
	Helm map[string][]helm.Revision
	// Sources holds the acquisition status of each source, by source key.
	Sources map[string]SourceInfo
}

// ReleaseKey is the key of a Helm release in Snapshot.Helm.
func ReleaseKey(namespace, release string) string {
	return namespace + "/" + release
}

// source returns the status of a source; a source never acquired is Loading.
func (s *Snapshot) source(key string) SourceInfo {
	if info, ok := s.Sources[key]; ok {
		return info
	}
	return SourceInfo{State: SourceLoading}
}
