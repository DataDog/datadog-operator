// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

// This file tracks constants closely related to the DatadogAgent CRD

const (

	// ClusterAgentReconcileConditionType ReconcileConditionType for Cluster Agent component
	ClusterAgentReconcileConditionType = "ClusterAgentReconcile"
	// AgentReconcileConditionType ReconcileConditionType for Agent component
	AgentReconcileConditionType = "AgentReconcile"
	// ClusterChecksRunnerReconcileConditionType ReconcileConditionType for Cluster Checks Runner component
	ClusterChecksRunnerReconcileConditionType = "ClusterChecksRunnerReconcile"
	// OverrideReconcileConflictConditionType ReconcileConditionType for override conflict
	OverrideReconcileConflictConditionType = "OverrideReconcileConflict"
	// DatadogAgentReconcileErrorConditionType ReconcileConditionType for DatadogAgent reconcile error
	DatadogAgentReconcileErrorConditionType = "DatadogAgentReconcileError"

	// ExtraConfdConfigMapName is the name of the ConfigMap storing Custom Confd data
	ExtraConfdConfigMapName = "%s-extra-confd"
	// ExtraChecksdConfigMapName is the name of the ConfigMap storing Custom Checksd data
	ExtraChecksdConfigMapName = "%s-extra-checksd"

	// DefaultAgentHealthPort default agent health port
	DefaultAgentHealthPort int32 = 5555

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
	// DefaultMetricsServerResourceSuffix use as suffix for cluster-agent metrics-server resource naming
	DefaultMetricsServerResourceSuffix = "cluster-agent-metrics-server"
	// DefaultAPPKeyKey default app-key key (use in secret for instance).
	DefaultAPPKeyKey = "app_key"
	// DefaultAPIKeyKey default api-key key (use in secret for instance).
	DefaultAPIKeyKey = "api_key"
	// DefaultTokenKey default token key (use in secret for instance).
	DefaultTokenKey = "token"
	// DefaultClusterAgentReplicas default cluster-agent deployment replicas
	DefaultClusterAgentReplicas = 1
	// DefaultClusterAgentServicePort default cluster-agent service port
	DefaultClusterAgentServicePort = 5005
	// DefaultClusterChecksRunnerReplicas default cluster checks runner deployment replicas
	DefaultClusterChecksRunnerReplicas = 1
	// DefaultAdmissionControllerServicePort default admission controller service port
	DefaultAdmissionControllerServicePort = 443
	// DefaultAdmissionControllerTargetPort default admission controller pod port
	DefaultAdmissionControllerTargetPort = 8000
	// DefaultAdmissionControllerWebhookName default admission controller webhook name
	DefaultAdmissionControllerWebhookName string = "datadog-webhook"
	// DefaultDogstatsdOriginDetection default Origin Detection
	// DefaultDogstatsdOriginDetection = "false" // CELENE NOT USED!
	// DefaultDogstatsdPort default dogstatsd port
	DefaultDogstatsdPort = 8125
	// DefaultDogstatsdPortName default dogstatsd port name
	DefaultDogstatsdPortName = "dogstatsdport"
	// DefaultOTelAgentConf default otel agent ConfigMap name
	DefaultOTelAgentConf string = "otel-agent-config"
	// DefaultKubeStateMetricsCoreConf default ksm core ConfigMap name
	DefaultKubeStateMetricsCoreConf string = "kube-state-metrics-core-config"
	// DefaultOrchestratorExplorerConf default orchestrator explorer ConfigMap name
	DefaultOrchestratorExplorerConf string = "orchestrator-explorer-config"
	// DefaultKubeAPIServerConf default Kubernetes APIServer ConfigMap name
	DefaultKubeAPIServerConf string = "kube-apiserver-config"
	// DefaultSystemProbeSocketPath default System Probe socket path
	DefaultSystemProbeSocketPath string = "/var/run/sysprobe/sysprobe.sock"
	// DefaultCSPMConf default CSPM ConfigMap name
	DefaultCSPMConf string = "cspm-config"
	// DefaultCWSConf default CWS ConfigMap name
	DefaultCWSConf string = "cws-config"
	// DefaultHelmCheckConf default Helm Check ConfigMap name
	DefaultHelmCheckConf string = "helm-check-config"

	// Default Image name
	DefaultAgentImageName        string = "agent"
	DefaultClusterAgentImageName string = "cluster-agent"
	DefaultImageRegistry         string = "gcr.io/datadoghq"
	DefaultAzureImageRegistry    string = "datadoghq.azurecr.io"
	DefaultEuropeImageRegistry   string = "eu.gcr.io/datadoghq"
	DefaultAsiaImageRegistry     string = "asia.gcr.io/datadoghq"
	DefaultGovImageRegistry      string = "public.ecr.aws/datadog"

	KubeServicesAndEndpointsConfigProviders = "kube_services kube_endpoints"
	KubeServicesAndEndpointsListeners       = "kube_services kube_endpoints"
	EndpointsChecksConfigProvider           = "endpointschecks"
	ClusterAndEndpointsConfigProviders      = "clusterchecks endpointschecks"
)
