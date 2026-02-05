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

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
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
func GetServiceAccountByComponent(objName string, ddaSpec *v2alpha1.DatadogAgentSpec, component v2alpha1.ComponentName) string {
	switch component {
	case v2alpha1.ClusterAgentComponentName:
		return GetClusterAgentServiceAccount(objName, ddaSpec)
	case v2alpha1.NodeAgentComponentName:
		return GetAgentServiceAccount(objName, ddaSpec)
	case v2alpha1.ClusterChecksRunnerComponentName:
		return GetClusterChecksRunnerServiceAccount(objName, ddaSpec)
	default:
		return ""
	}
}

// GetClusterAgentServiceAccount return the cluster-agent serviceAccountName
func GetClusterAgentServiceAccount(objName string, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	saDefault := fmt.Sprintf("%s-%s", objName, DefaultClusterAgentResourceSuffix)
	if ddaSpec.Override[v2alpha1.ClusterAgentComponentName] != nil && ddaSpec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountName != nil {
		return *ddaSpec.Override[v2alpha1.ClusterAgentComponentName].ServiceAccountName
	}
	return saDefault
}

// GetAgentServiceAccount returns the agent service account name
func GetAgentServiceAccount(objName string, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	saDefault := fmt.Sprintf("%s-%s", objName, DefaultAgentResourceSuffix)
	if ddaSpec.Override[v2alpha1.NodeAgentComponentName] != nil && ddaSpec.Override[v2alpha1.NodeAgentComponentName].ServiceAccountName != nil {
		return *ddaSpec.Override[v2alpha1.NodeAgentComponentName].ServiceAccountName
	}
	return saDefault
}

// GetClusterChecksRunnerServiceAccount return the cluster-checks-runner service account name
func GetClusterChecksRunnerServiceAccount(objName string, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	saDefault := fmt.Sprintf("%s-%s", objName, DefaultClusterChecksRunnerResourceSuffix)
	if ddaSpec.Override[v2alpha1.ClusterChecksRunnerComponentName] != nil && ddaSpec.Override[v2alpha1.ClusterChecksRunnerComponentName].ServiceAccountName != nil {
		return *ddaSpec.Override[v2alpha1.ClusterChecksRunnerComponentName].ServiceAccountName
	}
	return saDefault
}

// GetOtelAgentGatewayServiceAccount return the otel-agent-gateway service account name
func GetOtelAgentGatewayServiceAccount(objName string, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	saDefault := fmt.Sprintf("%s-%s", objName, DefaultOtelAgentGatewayResourceSuffix)
	if ddaSpec.Override[v2alpha1.OtelAgentGatewayComponentName] != nil && ddaSpec.Override[v2alpha1.OtelAgentGatewayComponentName].ServiceAccountName != nil {
		return *ddaSpec.Override[v2alpha1.OtelAgentGatewayComponentName].ServiceAccountName
	}
	return saDefault
}

// IsHostNetworkEnabled returns whether the pod should use the host's network namespace
func IsHostNetworkEnabled(ddaSpec *v2alpha1.DatadogAgentSpec, component v2alpha1.ComponentName) bool {
	if ddaSpec.Override != nil {
		if c, ok := ddaSpec.Override[component]; ok {
			return apiutils.BoolValue(c.HostNetwork)
		}
	}
	return false
}

// IsClusterChecksEnabled returns whether the DDA should use cluster checks
func IsClusterChecksEnabled(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return ddaSpec.Features.ClusterChecks != nil && apiutils.BoolValue(ddaSpec.Features.ClusterChecks.Enabled)
}

// IsCCREnabled returns whether the DDA should use Cluster Checks Runners
func IsCCREnabled(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return ddaSpec.Features.ClusterChecks != nil && apiutils.BoolValue(ddaSpec.Features.ClusterChecks.UseClusterChecksRunners)
}

// GetLocalAgentServiceName returns the name used for the local agent service
func GetLocalAgentServiceName(objName string, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	if ddaSpec.Global.LocalService != nil && ddaSpec.Global.LocalService.NameOverride != nil {
		return *ddaSpec.Global.LocalService.NameOverride
	}
	return fmt.Sprintf("%s-%s", objName, DefaultAgentResourceSuffix)
}

// GetOTelAgentGatewayServiceName returns the name used for the OTel Agent Gateway service
func GetOTelAgentGatewayServiceName(objName string) string {
	return fmt.Sprintf("%s-%s", objName, DefaultOtelAgentGatewayResourceSuffix)
}

// IsNetworkPolicyEnabled returns whether a network policy should be created and which flavor to use
func IsNetworkPolicyEnabled(ddaSpec *v2alpha1.DatadogAgentSpec) (bool, v2alpha1.NetworkPolicyFlavor) {
	if ddaSpec.Global != nil && ddaSpec.Global.NetworkPolicy != nil && apiutils.BoolValue(ddaSpec.Global.NetworkPolicy.Create) {
		if ddaSpec.Global.NetworkPolicy.Flavor != "" {
			return true, ddaSpec.Global.NetworkPolicy.Flavor
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

// GetDDAName returns the name of the DDA from DDAI labels or directly from the DDA
func GetDDAName(dda metav1.Object) string {
	if val, ok := dda.GetLabels()[apicommon.DatadogAgentNameLabelKey]; ok && val != "" {
		return val
	}
	return dda.GetName()
}

func GetOperatorComponentLabelKey(component v2alpha1.ComponentName) string {
	return OperatorComponentLabelKeyPrefix + string(component)
}
