// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package constants

const (
	// Liveness probe default config
	DefaultLivenessProbeInitialDelaySeconds int32 = 15
	DefaultLivenessProbePeriodSeconds       int32 = 15
	DefaultLivenessProbeTimeoutSeconds      int32 = 5
	DefaultLivenessProbeSuccessThreshold    int32 = 1
	DefaultLivenessProbeFailureThreshold    int32 = 6
	DefaultLivenessProbeHTTPPath                  = "/live"

	// Readiness probe default config
	DefaultReadinessProbeInitialDelaySeconds int32 = 15
	DefaultReadinessProbePeriodSeconds       int32 = 15
	DefaultReadinessProbeTimeoutSeconds      int32 = 5
	DefaultReadinessProbeSuccessThreshold    int32 = 1
	DefaultReadinessProbeFailureThreshold    int32 = 6
	DefaultReadinessProbeHTTPPath                  = "/ready"

	// Startup probe default config
	DefaultStartupProbeInitialDelaySeconds int32 = 15
	DefaultStartupProbePeriodSeconds       int32 = 15
	DefaultStartupProbeTimeoutSeconds      int32 = 5
	DefaultStartupProbeSuccessThreshold    int32 = 1
	DefaultStartupProbeFailureThreshold    int32 = 6
	DefaultStartupProbeHTTPPath                  = "/startup"

	// Agent Data plane default liveness/readiness probe configs
	DefaultADPLivenessProbeInitialDelaySeconds int32 = 5
	DefaultADPLivenessProbePeriodSeconds       int32 = 5
	DefaultADPLivenessProbeTimeoutSeconds      int32 = 5
	DefaultADPLivenessProbeSuccessThreshold    int32 = 1
	DefaultADPLivenessProbeFailureThreshold    int32 = 12

	DefaultADPReadinessProbeInitialDelaySeconds int32 = 5
	DefaultADPReadinessProbePeriodSeconds       int32 = 5
	DefaultADPReadinessProbeTimeoutSeconds      int32 = 5
	DefaultADPReadinessProbeSuccessThreshold    int32 = 1
	DefaultADPReadinessProbeFailureThreshold    int32 = 12

	// DefaultAgentHealthPort default agent health port
	DefaultAgentHealthPort int32 = 5555

	DefaultADPHealthPort = 5100

	// DefaultApmPort default apm port
	DefaultApmPort = 8126
	// DefaultApmPortName default apm port name
	DefaultApmPortName = "traceport"

	// DefaultAgentResourceSuffix use as suffix for agent resource naming
	DefaultAgentResourceSuffix = "agent"
	// DefaultClusterAgentResourceSuffix use as suffix for cluster-agent resource naming
	DefaultClusterAgentResourceSuffix = "cluster-agent"
	// DefaultClusterChecksRunnerResourceSuffix use as suffix for cluster-checks-runner resource naming
	DefaultClusterChecksRunnerResourceSuffix = "cluster-checks-runner"
	// DefaultOtelAgentGatewayResourceSuffix use as suffix for otel-agent-gateway resource naming
	DefaultOtelAgentGatewayResourceSuffix = "otel-agent-gateway"
)

// Labels
const (
	//MD5AgentDeploymentMigratedLabelKey label key is used to identify if a Helm-managed daemonset has been migrated
	MD5AgentDeploymentMigratedLabelKey = "agent.datadoghq.com/migrated"
	// MD5AgentDeploymentProviderLabelKey label key is used to identify which provider is being used
	MD5AgentDeploymentProviderLabelKey = "agent.datadoghq.com/provider"
	// MD5AgentDeploymentAnnotationKey annotation key used on a Resource in order to identify which AgentDeployment have been used to generate it.
	MD5AgentDeploymentAnnotationKey = "agent.datadoghq.com/agentspechash"
	// MD5DDAIDeploymentAnnotationKey annotation key is used on a DatadogAgentInternal resource to identify if changes have been made to the spec.
	MD5DDAIDeploymentAnnotationKey = "agent.datadoghq.com/ddaispechash"
	// MD5ChecksumAnnotationKey annotation key is used to identify customConfig configurations
	MD5ChecksumAnnotationKey = "checksum/%s-custom-config"
)

// Profiles
const (
	ProfileLabelKey = "agent.datadoghq.com/datadogagentprofile"
)

// DDAI finalizer
const (
	DatadogAgentInternalFinalizer = "finalizer.datadoghq.com/datadogagentinternal"
)
