// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"

	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

// ValidateDatadogAgent is used to check if a DatadogAgent is valid
func ValidateDatadogAgent(dda *DatadogAgent) error {
	// TODO
	// Ensure required credentials are configured.
	if dda.Spec.Global == nil || dda.Spec.Global.Credentials == nil {
		return fmt.Errorf("credentials not configured in the DatadogAgent, can't reconcile")
	}

	if err := validateVSock(&dda.Spec); err != nil {
		return err
	}

	return nil
}

// validateVSock ensures the VSock configuration is consistent with the features that rely on it.
func validateVSock(spec *DatadogAgentSpec) error {
	vsockEnabled, vsockMode := spec.Global.GetVSockConfig()
	if !vsockEnabled || vsockMode != VSockModeSystemProbe {
		return nil
	}

	// In SystemProbe mode the host system-probe hosts the runtime-security event server over
	// VSock and no longer exposes the unix socket the security-agent connects to, so CWS must
	// send payloads directly from the system-probe.
	if spec.Features == nil || spec.Features.CWS == nil || !apiutils.BoolValue(spec.Features.CWS.Enabled) {
		return nil
	}
	if !apiutils.BoolValue(spec.Features.CWS.DirectSendFromSystemProbe) {
		return fmt.Errorf("global.vsock.mode %q requires features.cws.directSendFromSystemProbe to be enabled", VSockModeSystemProbe)
	}

	return nil
}
