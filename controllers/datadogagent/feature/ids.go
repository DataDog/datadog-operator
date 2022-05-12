// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

// IDType use to identify a Feature
type IDType int

const (
	// DogStatsDIDType Dogstatsd check feature.
	DogStatsDIDType IDType = iota
	// KubernetesStateCoreIDType Kubernetes state core check feature.
	KubernetesStateCoreIDType
	// OrchestratorExplorerIDType Orchestrator Explorer feature.
	OrchestratorExplorerIDType
	// LogCollectionIDType Log Collection check feature
	LogCollectionIDType
	// NPMIDType NPM feature.
	NPMIDType
	// OOMKillIDType OOM Kill check feature
	OOMKillIDType
	// TCPQueueLengthIDType TCP Queue length check feature
	TCPQueueLengthIDType
	// DummyIDType Dummy feature.
	DummyIDType
)
