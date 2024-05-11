// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package v1alpha1

import "errors"

var (
	// ErrInvalidAutoFailRestarts is returned in case of a validation failure for maxRestarts in autoFail.
	ErrInvalidAutoFailRestarts = errors.New("canary autoFail.maxRestarts must be higher than autoPause.maxRestarts")
	// ErrDurationWithManualValidationMode is returned when validationMode=manual and duration is specified.
	ErrDurationWithManualValidationMode = errors.New("canary duration does not have effect with validationMode=manual")
	// ErrNoRestartsDurationWithManualValidationMode is returned when validationMode=manual and noRestartsDuration is specified.
	ErrNoRestartsDurationWithManualValidationMode = errors.New("canary noRestartsDuration does not have effect with validationMode=manual")
	// ErrInvalidCanaryTimeout is returned when the autoFail canaryTimeout is invalid.
	ErrInvalidCanaryTimeout = errors.New("canary autoFail.canaryTimeout must be greater than the canary duration")
)

// ValidateExtendedDaemonSetSpec validates an ExtendedDaemonSet spec
// returns true if yes, else no.
func ValidateExtendedDaemonSetSpec(spec *ExtendedDaemonSetSpec) error {
	if canary := spec.Strategy.Canary; canary != nil {
		if *canary.AutoFail.Enabled && *canary.AutoPause.Enabled && *canary.AutoFail.MaxRestarts < *canary.AutoPause.MaxRestarts {
			return ErrInvalidAutoFailRestarts
		}

		if *canary.AutoFail.Enabled && canary.AutoFail.CanaryTimeout != nil && canary.AutoFail.CanaryTimeout.Duration <= canary.Duration.Duration {
			return ErrInvalidCanaryTimeout
		}

		if canary.ValidationMode == ExtendedDaemonSetSpecStrategyCanaryValidationModeManual {
			if canary.Duration != nil {
				return ErrDurationWithManualValidationMode
			}
			if canary.NoRestartsDuration != nil {
				return ErrNoRestartsDurationWithManualValidationMode
			}
		}
	}

	return nil
}
