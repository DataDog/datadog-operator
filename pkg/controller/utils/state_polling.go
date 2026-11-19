// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"os"
	"strconv"

	"github.com/go-logr/logr"
)

const (
	// StatePollingEnabledEnvVar gates the periodic live-state polling of the
	// DatadogMonitor and DatadogGenericResource controllers.
	StatePollingEnabledEnvVar = "DD_MONITOR_STATE_POLLING_ENABLED"
	// StatePollingDisabledState is written to the state field and used as the
	// StateSynced condition reason while state polling is disabled.
	StatePollingDisabledState = "PollingDisabled"
)

// StatePollingEnabledFromEnv returns whether state polling is enabled. It
// defaults to true when the env var is unset or invalid.
func StatePollingEnabledFromEnv(logger logr.Logger) bool {
	enabled := true
	if val, ok := os.LookupEnv(StatePollingEnabledEnvVar); ok {
		parsed, err := strconv.ParseBool(val)
		if err != nil {
			logger.Error(err, "Invalid value for state polling enabled. Defaulting to true.", "envVar", StatePollingEnabledEnvVar)
		} else {
			enabled = parsed
		}
	}
	logger.Info("Setting state polling", "enabled", enabled)
	return enabled
}
