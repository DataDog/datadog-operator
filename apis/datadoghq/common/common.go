// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// GetDefaultLivenessProbe creates a all defaulted LivenessProbe
func GetDefaultLivenessProbe() *corev1.Probe {
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: DefaultLivenessProbeInitialDelaySeconds,
		PeriodSeconds:       DefaultLivenessProbePeriodSeconds,
		TimeoutSeconds:      DefaultLivenessProbeTimeoutSeconds,
		SuccessThreshold:    DefaultLivenessProbeSuccessThreshold,
		FailureThreshold:    DefaultLivenessProbeFailureThreshold,
	}
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: DefaultLivenessProbeHTTPPath,
		Port: intstr.IntOrString{
			IntVal: DefaultAgentHealthPort,
		},
	}
	return livenessProbe
}

// GetDefaultReadinessProbe creates a all defaulted ReadynessProbe
func GetDefaultReadinessProbe() *corev1.Probe {
	readinessProbe := &corev1.Probe{
		InitialDelaySeconds: DefaultReadinessProbeInitialDelaySeconds,
		PeriodSeconds:       DefaultReadinessProbePeriodSeconds,
		TimeoutSeconds:      DefaultReadinessProbeTimeoutSeconds,
		SuccessThreshold:    DefaultReadinessProbeSuccessThreshold,
		FailureThreshold:    DefaultReadinessProbeFailureThreshold,
	}
	readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: DefaultReadinessProbeHTTPPath,
		Port: intstr.IntOrString{
			IntVal: DefaultAgentHealthPort,
		},
	}
	return readinessProbe
}
