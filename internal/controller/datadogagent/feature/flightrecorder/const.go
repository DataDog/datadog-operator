// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorder

const (
	flightRecorderSocketVolumeName = "flightrecorder-socket"
	flightRecorderSocketPath       = "/var/run/datadog"
	flightRecorderDataVolumeName   = "flightrecorder-data"
	flightRecorderDataPath         = "/data/signals"
	flightRecorderSocketFile       = "/var/run/datadog/flightrecorder.sock"
)
