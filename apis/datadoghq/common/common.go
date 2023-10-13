// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// GetDefaultLivenessProbe creates a defaulted LivenessProbe
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

// GetDefaultReadinessProbe creates a defaulted ReadinessProbe
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

// GetDefaultTraceAgentProbe creates a defaulted liveness/readiness probe for the Trace Agent
func GetDefaultTraceAgentProbe() *corev1.Probe {
	probe := &corev1.Probe{
		InitialDelaySeconds: DefaultLivenessProbeInitialDelaySeconds,
		PeriodSeconds:       DefaultLivenessProbePeriodSeconds,
		TimeoutSeconds:      DefaultLivenessProbeTimeoutSeconds,
	}
	probe.TCPSocket = &corev1.TCPSocketAction{
		Port: intstr.IntOrString{
			IntVal: DefaultApmPort,
		},
	}
	return probe
}

// GetImage builds the image string based on ImageConfig and the registry configuration.
func GetImage(imageSpec *commonv1.AgentImageConfig, registry *string) string {
	if defaulting.IsImageNameContainsTag(imageSpec.Name) {
		return imageSpec.Name
	}

	img := defaulting.NewImage(imageSpec.Name, imageSpec.Tag, imageSpec.JMXEnabled)

	if registry != nil && *registry != "" {
		defaulting.WithRegistry(defaulting.ContainerRegistry(*registry))(img)
	}

	return img.String()
}
