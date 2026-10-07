// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package orderedrollout holds the pure logic of ordered DatadogAgentInternal
// rollouts: configuration resolution, target hashing, DaemonSet completion and
// the gate decision. It performs no API calls and reads no clock.
package orderedrollout

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	// HoldAnnotation on a DatadogAgent or DatadogAgentProfile holds steps with pending changes.
	HoldAnnotation = "agent.datadoghq.com/rollout-hold"
	// SkipAnnotation on a DatadogAgentProfile leaves its step on the live spec.
	SkipAnnotation = "agent.datadoghq.com/rollout-skip"
	// BypassAnnotation on a DatadogAgent rolls the current changes in parallel once per new value.
	BypassAnnotation = "agent.datadoghq.com/rollout-bypass"
	// TargetHashAnnotation stores the rollout target hash on a DatadogAgentInternal.
	TargetHashAnnotation = "agent.datadoghq.com/rollout-target-hash"
)

// Config is the resolved rollout configuration of one step.
type Config struct {
	Priority       int32
	StepTimeout    time.Duration
	StepSoak       time.Duration
	MaxUnavailable intstr.IntOrString
	OnStepTimeout  v2alpha1.RolloutTimeoutAction
}

// DefaultConfig returns the default-compatible configuration.
func DefaultConfig() Config {
	return Config{
		MaxUnavailable: intstr.FromInt32(0),
		OnStepTimeout:  v2alpha1.RolloutTimeoutActionContinue,
	}
}

// ResolveDDAConfig resolves the configuration of the default step.
func ResolveDDAConfig(strategy *v2alpha1.RolloutStrategy) Config {
	return overlay(DefaultConfig(), strategy)
}

// ResolveDAPConfig resolves a profile step configuration. Priority defaults to
// 0; other unset fields inherit from the DatadogAgent configuration.
func ResolveDAPConfig(parent Config, strategy *v2alpha1.RolloutStrategy) Config {
	cfg := parent
	cfg.Priority = 0
	return overlay(cfg, strategy)
}

func overlay(cfg Config, strategy *v2alpha1.RolloutStrategy) Config {
	if strategy == nil {
		return cfg
	}
	if strategy.Priority != nil {
		cfg.Priority = *strategy.Priority
	}
	if strategy.StepTimeout != nil {
		cfg.StepTimeout = strategy.StepTimeout.Duration
	}
	if strategy.StepSoak != nil {
		cfg.StepSoak = strategy.StepSoak.Duration
	}
	if strategy.MaxUnavailable != nil {
		cfg.MaxUnavailable = *strategy.MaxUnavailable
	}
	if strategy.OnStepTimeout != nil {
		cfg.OnStepTimeout = *strategy.OnStepTimeout
	}
	return cfg
}

// ResolveMaxUnavailable resolves maxUnavailable against the desired pod count,
// rounding percentages up.
func ResolveMaxUnavailable(value intstr.IntOrString, desired int32) (int32, error) {
	v, err := intstr.GetScaledValueFromIntOrPercent(&value, int(desired), true)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("maxUnavailable must not be negative: %s", value.String())
	}
	return int32(v), nil
}

// IsAnnotationTrue reports whether a rollout control annotation is set to "true".
func IsAnnotationTrue(annotations map[string]string, key string) bool {
	return annotations[key] == "true"
}
