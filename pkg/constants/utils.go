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

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
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

// GetServiceAccountByComponent returns the service account name for a given component
func GetServiceAccountByComponent(dda *v2alpha1.DatadogAgent, component v2alpha1.ComponentName) string {
	switch component {
	case v2alpha1.ClusterAgentComponentName:
		return GetClusterAgentServiceAccount(dda)
	case v2alpha1.NodeAgentComponentName:
		return GetAgentServiceAccount(dda)
	case v2alpha1.ClusterChecksRunnerComponentName:
		return GetClusterChecksRunnerServiceAccount(dda)
	default:
		return ""
	}
}

// GetServiceAccountByComponentDDAI returns the service account name for a given component (DDAI)
func GetServiceAccountByComponentDDAI(ddai *v1alpha1.DatadogAgentInternal, component v2alpha1.ComponentName) string {
	switch component {
	case v2alpha1.ClusterAgentComponentName:
		return GetClusterAgentServiceAccountDDAI(ddai)
	case v2alpha1.NodeAgentComponentName:
		return GetAgentServiceAccountDDAI(ddai)
	case v2alpha1.ClusterChecksRunnerComponentName:
		return GetClusterChecksRunnerServiceAccountDDAI(ddai)
	default:
		return ""
	}
}

// GetClusterAgentServiceAccount return the cluster-agent serviceAccountName
func GetClusterAgentServiceAccount(dda *v2alpha1.DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, DefaultClusterAgentResourceSuffix)
	if dda.Spec.Override[v2alpha1.ClusterAgentComponentName] != nil && dda.Spec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountName != nil {
		return *dda.Spec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountName
	}
	return saDefault
}

// GetClusterAgentServiceAccountDDAI returns the cluster-agent service account name
func GetClusterAgentServiceAccountDDAI(ddai *v1alpha1.DatadogAgentInternal) string {
	saDefault := fmt.Sprintf("%s-%s", ddai.Name, DefaultClusterAgentResourceSuffix)
	if ddai.Spec.Override[v2alpha1.ClusterAgentComponentName] != nil && ddai.Spec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountName != nil {
		return *ddai.Spec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountName
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

// GetAgentServiceAccountDDAI returns the agent service account name (DDAI)
func GetAgentServiceAccountDDAI(ddai *v1alpha1.DatadogAgentInternal) string {
	saDefault := fmt.Sprintf("%s-%s", ddai.Name, DefaultAgentResourceSuffix)
	if ddai.Spec.Override[v2alpha1.NodeAgentComponentName] != nil && ddai.Spec.Override[v2alpha1.NodeAgentComponentName].ServiceAccountName != nil {
		return *ddai.Spec.Override[v2alpha1.NodeAgentComponentName].ServiceAccountName
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

// GetClusterChecksRunnerServiceAccountDDAI returns the cluster-checks-runner service account name (DDAI)
func GetClusterChecksRunnerServiceAccountDDAI(ddai *v1alpha1.DatadogAgentInternal) string {
	saDefault := fmt.Sprintf("%s-%s", ddai.Name, DefaultClusterChecksRunnerResourceSuffix)
	if ddai.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName] != nil && ddai.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName].ServiceAccountName != nil {
		return *ddai.Spec.Override[v2alpha1.ClusterChecksRunnerComponentName].ServiceAccountName
	}
	return saDefault
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

// IsHostNetworkEnabledDDAI returns whether the pod should use the host's network namespace (DDAI)
func IsHostNetworkEnabledDDAI(ddai *v1alpha1.DatadogAgentInternal, component v2alpha1.ComponentName) bool {
	if ddai.Spec.Override != nil {
		if c, ok := ddai.Spec.Override[component]; ok {
			return apiutils.BoolValue(c.HostNetwork)
		}
	}
	return false
}

// IsClusterChecksEnabled returns whether the DDA should use cluster checks
func IsClusterChecksEnabled(dda *v2alpha1.DatadogAgent) bool {
	return dda.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(dda.Spec.Features.ClusterChecks.Enabled)
}

// IsClusterChecksEnabledDDAI returns whether the DDAI should use cluster checks (DDAI)
func IsClusterChecksEnabledDDAI(ddai *v1alpha1.DatadogAgentInternal) bool {
	return ddai.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(ddai.Spec.Features.ClusterChecks.Enabled)
}

// IsCCREnabled returns whether the DDA should use Cluster Checks Runners
func IsCCREnabled(dda *v2alpha1.DatadogAgent) bool {
	return dda.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(dda.Spec.Features.ClusterChecks.UseClusterChecksRunners)
}

// IsCCREnabledDDAI returns whether the DDAI should use Cluster Checks Runners (DDAI)
func IsCCREnabledDDAI(ddai *v1alpha1.DatadogAgentInternal) bool {
	return ddai.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(ddai.Spec.Features.ClusterChecks.UseClusterChecksRunners)
}

// GetLocalAgentServiceName returns the name used for the local agent service
func GetLocalAgentServiceName(dda *v2alpha1.DatadogAgent) string {
	if dda.Spec.Global.LocalService != nil && dda.Spec.Global.LocalService.NameOverride != nil {
		return *dda.Spec.Global.LocalService.NameOverride
	}
	return fmt.Sprintf("%s-%s", dda.Name, DefaultAgentResourceSuffix)
}

// GetLocalAgentServiceNameDDAI returns the name used for the local agent service (DDAI)
func GetLocalAgentServiceNameDDAI(ddai *v1alpha1.DatadogAgentInternal) string {
	if ddai.Spec.Global.LocalService != nil && ddai.Spec.Global.LocalService.NameOverride != nil {
		return *ddai.Spec.Global.LocalService.NameOverride
	}
	return fmt.Sprintf("%s-%s", ddai.Name, DefaultAgentResourceSuffix)
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

// IsNetworkPolicyEnabledDDAI returns whether a network policy should be created and which flavor to use (DDAI)
func IsNetworkPolicyEnabledDDAI(ddai *v1alpha1.DatadogAgentInternal) (bool, v2alpha1.NetworkPolicyFlavor) {
	if ddai.Spec.Global != nil && ddai.Spec.Global.NetworkPolicy != nil && apiutils.BoolValue(ddai.Spec.Global.NetworkPolicy.Create) {
		if ddai.Spec.Global.NetworkPolicy.Flavor != "" {
			return true, ddai.Spec.Global.NetworkPolicy.Flavor
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
