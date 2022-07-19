// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

// IDType use to identify a Feature
type IDType int

const (
	// DefaultIDType enable default component feature.
	DefaultIDType IDType = iota
	// DogstatsdIDType Dogstatsd feature.
	DogstatsdIDType
	// EventCollectionIDType Event Collection feature.
	EventCollectionIDType
	// KubernetesStateCoreIDType Kubernetes state core check feature.
	KubernetesStateCoreIDType
	// OrchestratorExplorerIDType Orchestrator Explorer feature.
	OrchestratorExplorerIDType
	// LogCollectionIDType Log Collection feature.
	LogCollectionIDType
	// NPMIDType NPM feature.
	NPMIDType
	// CSPMIDType CSPM feature.
	CSPMIDType
	// USMIDType USM feature.
	USMIDType
	// OOMKillIDType OOM Kill check feature
	OOMKillIDType
	// PrometheusScrapeIDType Prometheus Scrape feature
	PrometheusScrapeIDType
	// TCPQueueLengthIDType TCP Queue length check feature
	TCPQueueLengthIDType
	// ClusterChecksIDType Cluster checks feature
	ClusterChecksIDType
	// DummyIDType Dummy feature.
	DummyIDType
)
