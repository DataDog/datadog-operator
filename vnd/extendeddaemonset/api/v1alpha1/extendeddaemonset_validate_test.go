// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestValidateExtendedDaemonSetSpec(t *testing.T) {
	validNoCanary := DefaultExtendedDaemonSetSpec(&ExtendedDaemonSetSpec{}, ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)
	validWithCanary := DefaultExtendedDaemonSetSpec(&ExtendedDaemonSetSpec{
		Strategy: ExtendedDaemonSetSpecStrategy{
			Canary: &ExtendedDaemonSetSpecStrategyCanary{},
		},
	}, ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)
	validWithCanaryManualValidationMode := DefaultExtendedDaemonSetSpec(&ExtendedDaemonSetSpec{
		Strategy: ExtendedDaemonSetSpecStrategy{
			Canary: &ExtendedDaemonSetSpecStrategyCanary{},
		},
	}, ExtendedDaemonSetSpecStrategyCanaryValidationModeManual)

	validAutoFail := validWithCanary.DeepCopy()
	*validAutoFail.Strategy.Canary.AutoPause.Enabled = true
	*validAutoFail.Strategy.Canary.AutoPause.MaxRestarts = 2
	*validAutoFail.Strategy.Canary.AutoFail.Enabled = true
	*validAutoFail.Strategy.Canary.AutoFail.MaxRestarts = 3

	invalidAutoFail := validWithCanary.DeepCopy()
	*invalidAutoFail.Strategy.Canary.AutoPause.Enabled = true
	*invalidAutoFail.Strategy.Canary.AutoPause.MaxRestarts = 2
	*invalidAutoFail.Strategy.Canary.AutoFail.Enabled = true
	*invalidAutoFail.Strategy.Canary.AutoFail.MaxRestarts = 1

	validAutoFailNoAutoPause := validWithCanary.DeepCopy()
	*validAutoFailNoAutoPause.Strategy.Canary.AutoPause.Enabled = false
	*validAutoFailNoAutoPause.Strategy.Canary.AutoPause.MaxRestarts = 2
	*validAutoFailNoAutoPause.Strategy.Canary.AutoFail.Enabled = true
	*validAutoFailNoAutoPause.Strategy.Canary.AutoFail.MaxRestarts = 2

	validAutoPauseNoAutoFail := validWithCanary.DeepCopy()
	*validAutoPauseNoAutoFail.Strategy.Canary.AutoPause.Enabled = true
	*validAutoPauseNoAutoFail.Strategy.Canary.AutoPause.MaxRestarts = 1
	*validAutoPauseNoAutoFail.Strategy.Canary.AutoFail.Enabled = false
	*validAutoPauseNoAutoFail.Strategy.Canary.AutoFail.MaxRestarts = 1

	invalidCanaryTimeout := validWithCanary.DeepCopy()
	*invalidCanaryTimeout.Strategy.Canary.AutoPause.Enabled = true
	*invalidCanaryTimeout.Strategy.Canary.AutoFail.Enabled = true
	invalidCanaryTimeout.Strategy.Canary.AutoFail.CanaryTimeout = &metav1.Duration{
		Duration: 1 * time.Minute,
	}

	validManualValidationMode := validWithCanaryManualValidationMode.DeepCopy()
	validManualValidationMode.Strategy.Canary.ValidationMode = ExtendedDaemonSetSpecStrategyCanaryValidationModeManual

	invalidManualValidationDuration := validWithCanaryManualValidationMode.DeepCopy()
	invalidManualValidationDuration.Strategy.Canary.ValidationMode = ExtendedDaemonSetSpecStrategyCanaryValidationModeManual
	invalidManualValidationDuration.Strategy.Canary.Duration = &metav1.Duration{}

	invalidManualValidationNoRestartsDuration := validWithCanaryManualValidationMode.DeepCopy()
	invalidManualValidationNoRestartsDuration.Strategy.Canary.ValidationMode = ExtendedDaemonSetSpecStrategyCanaryValidationModeManual
	invalidManualValidationNoRestartsDuration.Strategy.Canary.NoRestartsDuration = &metav1.Duration{}

	tests := []struct {
		name string
		spec *ExtendedDaemonSetSpec
		err  error
	}{
		{
			name: "valid no canary",
			spec: validNoCanary,
		},
		{
			name: "valid with canary",
			spec: validWithCanary,
		},
		{
			name: "valid autoFail maxRestarts",
			spec: validAutoFail,
		},
		{
			name: "invalid autoFail maxRestarts",
			spec: invalidAutoFail,
			err:  ErrInvalidAutoFailRestarts,
		},
		{
			name: "invalid autoFail canaryTimeout",
			spec: invalidCanaryTimeout,
			err:  ErrInvalidCanaryTimeout,
		},
		{
			name: "valid autoFail no autoPause",
			spec: validAutoFailNoAutoPause,
		},
		{
			name: "valid autoPause no autoFail",
			spec: validAutoPauseNoAutoFail,
		},
		{
			name: "valid manual validation mode",
			spec: validManualValidationMode,
		},
		{
			name: "invalid manual validation mode with duration",
			spec: invalidManualValidationDuration,
			err:  ErrDurationWithManualValidationMode,
		},
		{
			name: "invalid manual validation mode with noRestartsDuration",
			spec: invalidManualValidationNoRestartsDuration,
			err:  ErrNoRestartsDurationWithManualValidationMode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateExtendedDaemonSetSpec(test.spec)
			assert.Equal(t, test.err, err)
		})
	}
}
