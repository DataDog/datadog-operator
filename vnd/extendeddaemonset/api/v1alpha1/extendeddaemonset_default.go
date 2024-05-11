// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	defaultCanaryReplica              = 1
	defaultCanaryDuration             = 10
	defaultCanaryNoRestartsDuration   = 5
	defaultCanaryAutoPauseEnabled     = true
	defaultCanaryAutoPauseMaxRestarts = 2
	defaultCanaryAutoFailEnabled      = true
	defaultCanaryAutoFailMaxRestarts  = 5
	defaultSlowStartIntervalDuration  = 1
	defaultMaxParallelPodCreation     = 250
	defaultReconcileFrequency         = 10 * time.Second
)

// IsDefaultedExtendedDaemonSet used to know if a ExtendedDaemonSet is already defaulted
// returns true if yes, else no.
func IsDefaultedExtendedDaemonSet(dd *ExtendedDaemonSet) bool {
	if !IsDefaultedExtendedDaemonSetSpecStrategyRollingUpdate(&dd.Spec.Strategy.RollingUpdate) {
		return false
	}

	if dd.Spec.Strategy.Canary != nil {
		if defaulted := IsDefaultedExtendedDaemonSetSpecStrategyCanary(dd.Spec.Strategy.Canary); !defaulted {
			return false
		}
	}

	if dd.Spec.Strategy.ReconcileFrequency == nil {
		return false
	}

	if dd.Spec.Template.Name != "" {
		// this field needs to be cleaned up as we can't deploy multiple
		// pods with the same name
		return false
	}

	return true
}

// IsDefaultedExtendedDaemonSetSpecStrategyRollingUpdate used to know if a ExtendedDaemonSetSpecStrategyRollingUpdate is already defaulted
// returns true if yes, else no.
func IsDefaultedExtendedDaemonSetSpecStrategyRollingUpdate(rollingupdate *ExtendedDaemonSetSpecStrategyRollingUpdate) bool {
	if rollingupdate.MaxUnavailable == nil {
		return false
	}

	if rollingupdate.MaxParallelPodCreation == nil {
		return false
	}

	if rollingupdate.MaxPodSchedulerFailure == nil {
		return false
	}

	if rollingupdate.SlowStartIntervalDuration == nil {
		return false
	}

	if rollingupdate.SlowStartAdditiveIncrease == nil {
		return false
	}

	return true
}

// IsDefaultedExtendedDaemonSetSpecStrategyCanary used to know if a ExtendedDaemonSetSpecStrategyCanary is already defaulted
// returns true if yes, else no.
func IsDefaultedExtendedDaemonSetSpecStrategyCanary(canary *ExtendedDaemonSetSpecStrategyCanary) bool {
	if canary.Replicas == nil {
		return false
	}
	if canary.ValidationMode == "" {
		return false
	}
	if canary.Duration == nil && canary.ValidationMode == ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto {
		return false
	}
	if canary.NodeSelector == nil {
		return false
	}
	if canary.AutoPause == nil || canary.AutoPause.Enabled == nil || canary.AutoPause.MaxRestarts == nil {
		return false
	}

	if canary.AutoFail == nil || canary.AutoFail.Enabled == nil || canary.AutoFail.MaxRestarts == nil {
		return false
	}

	return true
}

// DefaultExtendedDaemonSet used to default an ExtendedDaemonSet
// return a list of errors in case of invalid fields.
func DefaultExtendedDaemonSet(dd *ExtendedDaemonSet, defaultValidationMode ExtendedDaemonSetSpecStrategyCanaryValidationMode) *ExtendedDaemonSet {
	defaultedDD := dd.DeepCopy()
	DefaultExtendedDaemonSetSpec(&defaultedDD.Spec, defaultValidationMode)

	return defaultedDD
}

// DefaultExtendedDaemonSetSpec used to default an ExtendedDaemonSetSpec.
func DefaultExtendedDaemonSetSpec(spec *ExtendedDaemonSetSpec, defaultValidationMode ExtendedDaemonSetSpecStrategyCanaryValidationMode) *ExtendedDaemonSetSpec {
	// reset template name
	spec.Template.Name = ""

	DefaultExtendedDaemonSetSpecStrategyRollingUpdate(&spec.Strategy.RollingUpdate)

	if spec.Strategy.Canary != nil {
		DefaultExtendedDaemonSetSpecStrategyCanary(spec.Strategy.Canary, defaultValidationMode)
	}

	if spec.Strategy.ReconcileFrequency == nil {
		spec.Strategy.ReconcileFrequency = &metav1.Duration{Duration: defaultReconcileFrequency}
	}

	return spec
}

// DefaultExtendedDaemonSetSpecStrategyCanary used to default an ExtendedDaemonSetSpecStrategyCanary.
func DefaultExtendedDaemonSetSpecStrategyCanary(c *ExtendedDaemonSetSpecStrategyCanary, defaultValidationMode ExtendedDaemonSetSpecStrategyCanaryValidationMode) *ExtendedDaemonSetSpecStrategyCanary {
	if c.ValidationMode == "" {
		c.ValidationMode = defaultValidationMode
	}
	if c.Duration == nil && c.ValidationMode == ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto {
		c.Duration = &metav1.Duration{
			Duration: defaultCanaryDuration * time.Minute,
		}
	}
	if c.Replicas == nil {
		replicas := intstr.FromInt(defaultCanaryReplica)
		c.Replicas = &replicas
	}
	if c.NodeSelector == nil {
		c.NodeSelector = &metav1.LabelSelector{
			MatchLabels: map[string]string{},
		}
	}
	if c.AutoPause == nil {
		c.AutoPause = &ExtendedDaemonSetSpecStrategyCanaryAutoPause{}
	}
	DefaultExtendedDaemonSetSpecStrategyCanaryAutoPause(c.AutoPause)

	if c.AutoFail == nil {
		c.AutoFail = &ExtendedDaemonSetSpecStrategyCanaryAutoFail{}
	}
	DefaultExtendedDaemonSetSpecStrategyCanaryAutoFail(c.AutoFail)

	if c.NoRestartsDuration == nil && c.ValidationMode == ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto {
		c.NoRestartsDuration = &metav1.Duration{
			Duration: defaultCanaryNoRestartsDuration * time.Minute,
		}
	}

	return c
}

// DefaultExtendedDaemonSetSpecStrategyCanaryAutoPause used to default an ExtendedDaemonSetSpecStrategyCanaryAutoPause.
func DefaultExtendedDaemonSetSpecStrategyCanaryAutoPause(a *ExtendedDaemonSetSpecStrategyCanaryAutoPause) *ExtendedDaemonSetSpecStrategyCanaryAutoPause {
	if a.Enabled == nil {
		enabled := defaultCanaryAutoPauseEnabled
		a.Enabled = &enabled
	}

	if a.MaxRestarts == nil {
		a.MaxRestarts = NewInt32(defaultCanaryAutoPauseMaxRestarts)
	}

	return a
}

// DefaultExtendedDaemonSetSpecStrategyCanaryAutoFail used to default an ExtendedDaemonSetSpecStrategyCanaryAutoFail.
func DefaultExtendedDaemonSetSpecStrategyCanaryAutoFail(a *ExtendedDaemonSetSpecStrategyCanaryAutoFail) *ExtendedDaemonSetSpecStrategyCanaryAutoFail {
	if a.Enabled == nil {
		enabled := defaultCanaryAutoFailEnabled
		a.Enabled = &enabled
	}

	if a.MaxRestarts == nil {
		a.MaxRestarts = NewInt32(defaultCanaryAutoFailMaxRestarts)
	}

	return a
}

// DefaultExtendedDaemonSetSpecStrategyRollingUpdate used to default an ExtendedDaemonSetSpecStrategyRollingUpdate.
func DefaultExtendedDaemonSetSpecStrategyRollingUpdate(rollingupdate *ExtendedDaemonSetSpecStrategyRollingUpdate) *ExtendedDaemonSetSpecStrategyRollingUpdate {
	rollingupdate.MaxUnavailable = intstr.ValueOrDefault(rollingupdate.MaxUnavailable, intstr.FromInt(1))

	if rollingupdate.MaxParallelPodCreation == nil {
		rollingupdate.MaxParallelPodCreation = NewInt32(defaultMaxParallelPodCreation)
	}

	rollingupdate.MaxPodSchedulerFailure = intstr.ValueOrDefault(rollingupdate.MaxPodSchedulerFailure, intstr.FromInt(0))

	if rollingupdate.SlowStartIntervalDuration == nil {
		rollingupdate.SlowStartIntervalDuration = &metav1.Duration{
			Duration: defaultSlowStartIntervalDuration * time.Minute,
		}
	}

	rollingupdate.SlowStartAdditiveIncrease = intstr.ValueOrDefault(rollingupdate.SlowStartAdditiveIncrease, intstr.FromInt(1))

	return rollingupdate
}
