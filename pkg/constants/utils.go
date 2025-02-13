// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package constants

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
)

// GetConfName get the name of the Configmap for a CustomConfigSpec
func GetConfName(owner metav1.Object, conf *v2alpha1.CustomConfig, defaultName string) string {
	// `configData` and `configMap` can't be set together.
	// Return the default if the conf is not overridden or if it is just overridden with the ConfigData.
	if conf != nil && conf.ConfigMap != nil {
		return conf.ConfigMap.Name
	}
	return fmt.Sprintf("%s-%s", owner.GetName(), defaultName)
}

// GetClusterAgentServiceAccount return the cluster-agent serviceAccountName
func GetClusterAgentServiceAccount(dda *v2alpha1.DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, DefaultClusterAgentResourceSuffix)
	if dda.Spec.Override[v2alpha1.ClusterAgentComponentName] != nil && dda.Spec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountName != nil {
		return *dda.Spec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountName
	}
	return saDefault
}

// GetAgentServiceAccount returns the agent service account name
func GetAgentServiceAccount(dda *v2alpha1.DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, DefaultAgentResourceSuffix)
	if dda.Spec.Override[v2alpha1.NodeAgentComponentName] != nil && dda.Spec.Override[v2alpha1.NodeAgentComponentName].ServiceAccountName != nil {
		return *dda.Spec.Override[v2alpha1.NodeAgentComponentName].ServiceAccountName
	}
	return saDefault
}

// GetClusterChecksRunnerServiceAccount return the cluster-checks-runner service account name
func GetClusterChecksRunnerServiceAccount(dda *v2alpha1.DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, DefaultClusterChecksRunnerResourceSuffix)
	if dda.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName] != nil && dda.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName].ServiceAccountName != nil {
		return *dda.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName].ServiceAccountName
	}
	return saDefault
}

// GetClusterAgentServiceAccountAnnotations returns the annotations for the cluster-agent service account.
func GetClusterAgentServiceAccountAnnotations(dda *v2alpha1.DatadogAgent) map[string]string {
	defaultAnnotations := map[string]string{}
	if dda.Spec.Override[v2alpha1.ClusterAgentComponentName] != nil && dda.Spec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountAnnotations != nil {
		return dda.Spec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountAnnotations
	}
	return defaultAnnotations
}

// GetAgentServiceAccountAnnotations returns the annotations for the agent service account.
func GetAgentServiceAccountAnnotations(dda *v2alpha1.DatadogAgent) map[string]string {
	defaultAnnotations := map[string]string{}
	if dda.Spec.Override[v2alpha1.NodeAgentComponentName] != nil && dda.Spec.Override[v2alpha1.NodeAgentComponentName].ServiceAccountAnnotations != nil {
		return dda.Spec.Override[v2alpha1.NodeAgentComponentName].ServiceAccountAnnotations
	}
	return defaultAnnotations
}

// GetClusterChecksRunnerServiceAccountAnnotations returns the annotations for the cluster-checks-runner service account.
func GetClusterChecksRunnerServiceAccountAnnotations(dda *v2alpha1.DatadogAgent) map[string]string {
	defaultAnnotations := map[string]string{}
	if dda.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName] != nil && dda.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName].ServiceAccountAnnotations != nil {
		return dda.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName].ServiceAccountAnnotations
	}
	return defaultAnnotations
}

// IsHostNetworkEnabled returns whether the pod should use the host's network namespace
func IsHostNetworkEnabled(dda *v2alpha1.DatadogAgent, component v2alpha1.ComponentName) bool {
	if dda.Spec.Override != nil {
		if c, ok := dda.Spec.Override[component]; ok {
			return apiutils.BoolValue(c.HostNetwork)
		}
	}
	return false
}

// IsClusterChecksEnabled returns whether the DDA should use cluster checks
func IsClusterChecksEnabled(dda *v2alpha1.DatadogAgent) bool {
	return dda.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(dda.Spec.Features.ClusterChecks.Enabled)
}

// IsCCREnabled returns whether the DDA should use Cluster Checks Runners
func IsCCREnabled(dda *v2alpha1.DatadogAgent) bool {
	return dda.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(dda.Spec.Features.ClusterChecks.UseClusterChecksRunners)
}

// GetLocalAgentServiceName returns the name used for the local agent service
func GetLocalAgentServiceName(dda *v2alpha1.DatadogAgent) string {
	if dda.Spec.Global.LocalService != nil && dda.Spec.Global.LocalService.NameOverride != nil {
		return *dda.Spec.Global.LocalService.NameOverride
	}
	return fmt.Sprintf("%s-%s", dda.Name, DefaultAgentResourceSuffix)
}

// IsNetworkPolicyEnabled returns whether a network policy should be created and which flavor to use
func IsNetworkPolicyEnabled(dda *v2alpha1.DatadogAgent) (bool, v2alpha1.NetworkPolicyFlavor) {
	if dda.Spec.Global != nil && dda.Spec.Global.NetworkPolicy != nil && apiutils.BoolValue(dda.Spec.Global.NetworkPolicy.Create) {
		if dda.Spec.Global.NetworkPolicy.Flavor != "" {
			return true, dda.Spec.Global.NetworkPolicy.Flavor
		}
		return true, v2alpha1.NetworkPolicyFlavorKubernetes
	}
	return false, ""
}

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

// GetDefaultStartupProbe creates a defaulted StartupProbe
func GetDefaultStartupProbe() *corev1.Probe {
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
			IntVal: DefaultAgentHealthPort,
		},
	}
	return startupProbe
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

// GetDefaultAgentDataPlaneLivenessProbe creates a defaulted liveness probe for Agent Data Plane
func GetDefaultAgentDataPlaneLivenessProbe() *corev1.Probe {
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: DefaultADPLivenessProbeInitialDelaySeconds,
		PeriodSeconds:       DefaultADPLivenessProbePeriodSeconds,
		TimeoutSeconds:      DefaultADPLivenessProbeTimeoutSeconds,
		SuccessThreshold:    DefaultADPLivenessProbeSuccessThreshold,
		FailureThreshold:    DefaultADPLivenessProbeFailureThreshold,
	}
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: DefaultLivenessProbeHTTPPath,
		Port: intstr.IntOrString{
			IntVal: DefaultADPHealthPort,
		},
	}
	return livenessProbe
}

// GetDefaultAgentDataPlaneReadinessProbe creates a defaulted readiness probe for Agent Data Plane
func GetDefaultAgentDataPlaneReadinessProbe() *corev1.Probe {
	readinessProbe := &corev1.Probe{
		InitialDelaySeconds: DefaultADPReadinessProbeInitialDelaySeconds,
		PeriodSeconds:       DefaultADPReadinessProbePeriodSeconds,
		TimeoutSeconds:      DefaultADPReadinessProbeTimeoutSeconds,
		SuccessThreshold:    DefaultADPReadinessProbeSuccessThreshold,
		FailureThreshold:    DefaultADPReadinessProbeFailureThreshold,
	}
	readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: DefaultReadinessProbeHTTPPath,
		Port: intstr.IntOrString{
			IntVal: DefaultADPHealthPort,
		},
	}
	return readinessProbe
}

// GetImage builds the image string based on ImageConfig and the registry configuration.
func GetImage(imageSpec *v2alpha1.AgentImageConfig, registry *string) string {
	if defaulting.IsImageNameContainsTag(imageSpec.Name) {
		return imageSpec.Name
	}

	img := defaulting.NewImage(imageSpec.Name, imageSpec.Tag, imageSpec.JMXEnabled)

	if registry != nil && *registry != "" {
		defaulting.WithRegistry(defaulting.ContainerRegistry(*registry))(img)
	}

	return img.String()
}
