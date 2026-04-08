// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorder

import "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"

const (
	flightRecorderSocketFile = common.FlightRecorderSocketPath + "/flightrecorder.sock"

	// Env var names for the flightrecorder feature
	ddFlightRecorderEnabled    = "DD_FLIGHTRECORDER_ENABLED"
	ddFlightRecorderSocketPath = "DD_FLIGHTRECORDER_SOCKET_PATH"
	ddFlightRecorderOutputDir  = "DD_FLIGHTRECORDER_OUTPUT_DIR"
)
