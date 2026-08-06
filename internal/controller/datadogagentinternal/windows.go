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
// that path: the configured log paths. The Windows feature-support matrix (which features run on
// a Windows DaemonSet) lives in the centralized table in feature/provider_support.go.

package datadogagentinternal

import (
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
)

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
