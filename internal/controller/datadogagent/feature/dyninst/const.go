// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dyninst

import "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"

const (
	// stateDirVolumeName backs the directory the Dynamic Instrumentation
	// system-probe module uses for its writable state (the debugger-probes
	// tombstone file and the SymDB upload cache). This must stay in sync with
	// dynamicInstrumentationStateDir() in pkg/dyninst/module/config.go in
	// datadog-agent.
	stateDirVolumeName = "dynamicinstrumentationstate"
	stateDirVolumePath = common.RunPathVolumeMount + "/system-probe/dynamic-instrumentation"
)
