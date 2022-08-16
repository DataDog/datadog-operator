// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

// IDType use to identify a Feature
type IDType string

const (
	// DefaultIDType enable default component feature.
	DefaultIDType IDType = "default"
	// DogstatsdIDType Dogstatsd feature.
	DogstatsdIDType = "dogstatsd"
	// EventCollectionIDType Event Collection feature.
	EventCollectionIDType = "event_collection"
	// KubernetesStateCoreIDType Kubernetes state core check feature.
	KubernetesStateCoreIDType = "ksm"
	// LiveProcessIDType Live Process feature.
	LiveProcessIDType = "live_process"
	// OrchestratorExplorerIDType Orchestrator Explorer feature.
	OrchestratorExplorerIDType = "orchestrator_explorer"
	// LogCollectionIDType Log Collection feature.
	LogCollectionIDType = "log_collection"
	// NPMIDType NPM feature.
	NPMIDType = "npm"
	// CSPMIDType CSPM feature.
	CSPMIDType = "cspm"
	// CWSIDType CWS feature.
	CWSIDType = "csw"
	// USMIDType USM feature.
	USMIDType = "usm"
	// OOMKillIDType OOM Kill check feature
	OOMKillIDType = "oom_kill"
	// PrometheusScrapeIDType Prometheus Scrape feature
	PrometheusScrapeIDType = "prometheus_scrape"
	// TCPQueueLengthIDType TCP Queue length check feature
	TCPQueueLengthIDType = "tcp_queue_length"
	// ClusterChecksIDType Cluster checks feature
	ClusterChecksIDType = "cluster_checks"
	// APMIDType APM feature
	APMIDType = "apm"
	// DummyIDType Dummy feature.
	DummyIDType = "dummy"
)
