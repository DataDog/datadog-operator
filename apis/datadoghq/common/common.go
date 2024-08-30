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

// GetDefaultLivenessProbeWithPort creates a liveness probe with defaulted values, using the provided port
func GetDefaultLivenessProbeWithPort(port int32) *corev1.Probe {
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
			IntVal: port,
		},
	}
	return livenessProbe
}

// GetDefaultReadinessProbeWithPort creates a readiness probe with defaulted values, using the provided port
func GetDefaultReadinessProbeWithPort(port int32) *corev1.Probe {
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
			IntVal: port,
		},
	}
	return readinessProbe
}

// GetDefaultStartupProbeWithPort creates a startup probe with defaulted values, using the provided port
func GetDefaultStartupProbeWithPort(port int32) *corev1.Probe {
	startupProbe := &corev1.Probe{
		InitialDelaySeconds: DefaultStartupProbeInitialDelaySeconds,
		PeriodSeconds:       DefaultStartupProbePeriodSeconds,
		TimeoutSeconds:      DefaultStartupProbeTimeoutSeconds,
		SuccessThreshold:    DefaultStartupProbeSuccessThreshold,
		FailureThreshold:    DefaultStartupProbeFailureThreshold,
	}
	startupProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: DefaultStartupProbeHTTPPath,
		Port: intstr.IntOrString{
			IntVal: port,
		},
	}
	return startupProbe
}

// GetAgentLivenessProbe creates a liveness probe configured for the core Agent
func GetAgentLivenessProbe() *corev1.Probe {
	return GetDefaultLivenessProbeWithPort(DefaultAgentHealthPort)
}

// GetAgentLivenessProbe creates a readiness probe configured for the core Agent
func GetAgentReadinessProbe() *corev1.Probe {
	return GetDefaultReadinessProbeWithPort(DefaultAgentHealthPort)
}

// GetAgentStartupProbe creates a startup probe configured for the core Agent
func GetAgentStartupProbe() *corev1.Probe {
	return GetDefaultStartupProbeWithPort(DefaultAgentHealthPort)
}

// GetAgentDataPlaneLivenessProbe creates a liveness probe configured for the Agent Data Plane
func GetAgentDataPlaneLivenessProbe() *corev1.Probe {
	return GetDefaultLivenessProbeWithPort(DefaultAgentDataPlanetHealthPort)
}

// GetAgentDataPlaneLivenessProbe creates a readiness probe configured for the Agent Data Plane
func GetAgentDataPlaneReadinessProbe() *corev1.Probe {
	return GetDefaultReadinessProbeWithPort(DefaultAgentDataPlanetHealthPort)
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
