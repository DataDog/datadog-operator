// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Windows node-agent support (CONTP-1448).
//
// Windows nodes are enabled through a DatadogAgentProfile that targets Windows nodes and
// carries the datadoghq.com/provider=windows annotation. The profile machinery generates a
// dedicated DDAI (and therefore a dedicated DaemonSet) scoped to those nodes; reconcileV2Agent
// then detects provider==windows and Windows-ifies the built pod template via
// componentagent.ApplyWindowsPodTransformation. This file holds the Windows-specific inputs to
// that path: the feature allowlist and the configured log paths.

package datadogagentinternal

import (
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

// windowsSupportedFeatures is the allowlist of feature IDs whose ManageNodeAgent hook runs
// against a Windows (provider==windows) DaemonSet. Features absent here are gated out so they
// don't inject env vars / config the Windows agent can't use (eBPF/system-probe in particular).
// This is an allowlist (not a denylist) so new Linux-only features don't silently leak onto
// Windows. The `default` feature is required — it configures the base agent. Expand as Windows
// support for more features is verified.
//
// NOTE when adding a feature here: the Windows strip removes env vars by NAME (Unix-socket
// names, known Linux-only globals — see stripWindowsIncompatibleEnvVars) and mounts by Linux
// path prefix, but it does NOT inspect env-var VALUES. Verify the feature does not hand the
// agent a Linux path/transport VALUE under an innocuous (non-SOCKET) env-var name; if it does,
// extend the strip accordingly before allowlisting it.
var windowsSupportedFeatures = map[feature.IDType]bool{
	feature.DefaultIDType:              true, // base agent config (required)
	feature.APMIDType:                  true, // trace-agent runs on Windows (TCP; see non-local-traffic)
	feature.LogCollectionIDType:        true, // log collection — Windows host-log mounts added post-strip
	feature.LiveContainerIDType:        true, // container collection
	feature.LiveProcessIDType:          true, // process collection (no eBPF on Windows)
	feature.ProcessDiscoveryIDType:     true,
	feature.DogstatsdIDType:            true, // dogstatsd (UDP non-local; named pipe future)
	feature.OrchestratorExplorerIDType: true, // node-side config is env-only
	feature.RemoteConfigurationIDType:  true,
	feature.PrometheusScrapeIDType:     true,
	feature.EventCollectionIDType:      true,
}

// windowsLogPaths reads the configured logCollection host paths from the DDAI so custom Windows
// log paths take effect (Windows defaults are applied for empty/Linux values downstream).
func windowsLogPaths(ddai *datadoghqv1alpha1.DatadogAgentInternal) componentagent.WindowsLogPaths {
	var p componentagent.WindowsLogPaths
	if ddai.Spec.Features == nil || ddai.Spec.Features.LogCollection == nil {
		return p
	}
	lc := ddai.Spec.Features.LogCollection
	if lc.TempStoragePath != nil {
		p.TempStoragePath = *lc.TempStoragePath
	}
	if lc.PodLogsPath != nil {
		p.PodLogsPath = *lc.PodLogsPath
	}
	if lc.ContainerLogsPath != nil {
		p.ContainerLogsPath = *lc.ContainerLogsPath
	}
	return p
}
