// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import "fmt"

func (d *Daemon) validateLifecycleRequestEnvelope(req remoteAPIRequest) error {
	if req.ID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent lifecycle task ID is required"}
	}
	if req.Params.OperationID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent lifecycle operation ID is required"}
	}
	if !d.lifecycleIdentity.Configured() {
		return &stateDoesntMatchError{msg: "DatadogAgent lifecycle identity is not configured"}
	}
	if err := d.lifecycleIdentity.Validate(); err != nil {
		return &stateDoesntMatchError{msg: "DatadogAgent lifecycle identity is invalid"}
	}
	if d.rcClient == nil || d.rcClient.GetClientID() == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent lifecycle local RC client ID is unavailable"}
	}
	if req.ExpectedState.ClientID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent lifecycle expected RC client ID is required"}
	}
	if req.ExpectedState.ClientID != d.rcClient.GetClientID() {
		return &stateDoesntMatchError{msg: "DatadogAgent lifecycle expected RC client ID does not match the local client"}
	}
	if req.Params.InstallationID != d.lifecycleIdentity.InstallationID {
		return &stateDoesntMatchError{msg: "DatadogAgent lifecycle installation ID does not match the local add-on configuration"}
	}

	expectedOperation, err := lifecycleMethodOperation(req.Method)
	if err != nil {
		return err
	}
	_, err = d.resolveLifecycleOperation(req, expectedOperation)
	return err
}

func lifecycleMethodOperation(method string) (Operation, error) {
	switch method {
	case methodInstallDatadogAgent:
		return OperationCreate, nil
	case methodUninstallDatadogAgent, methodVerifyDatadogAgentUninstalled, methodClearDatadogAgentUninstallFence:
		return OperationDelete, nil
	default:
		return "", fmt.Errorf("method %q is not a DatadogAgent lifecycle mutation", method)
	}
}
