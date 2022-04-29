// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

// IDType use to identify a Feature
type IDType int

const (
	// KubernetesStateCoreIDType Kubernetes state core check feature.
	KubernetesStateCoreIDType IDType = iota
	// OrchestratorExplorerIDType Orchestrator Explorer feature.
	OrchestratorExplorerIDType
	// DummyIDType Dummt feature.
	DummyIDType
)
