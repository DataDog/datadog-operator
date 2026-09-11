// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkrunner

const (
	// ddCheckRunnerEnabled is owned by the Core Agent, not by ACR. The Core Agent reads it,
	// decides which checks it delegates rather than running itself, and publishes the result
	// to ACR in its config snapshot over IPC. Hence the Go single-underscore spelling.
	ddCheckRunnerEnabled = "DD_CHECK_RUNNER_ENABLED"

	// Whether to register as a sub-agent of the Core Agent.
	ddCheckRunnerStandaloneMode = "DD_CHECK_RUNNER__STANDALONE_MODE"

	// Communication with the Data Plane, over the gRPC IPC endpoint.
	ddCheckRunnerEndpointsIPCEnabled  = "DD_CHECK_RUNNER__ENDPOINTS__IPC__ENABLED"
	ddCheckRunnerEndpointsIPCEndpoint = "DD_CHECK_RUNNER__ENDPOINTS__IPC__ENDPOINT"
)
