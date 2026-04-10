// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

const ClusterAgentMinVersion = "7.76.0"

// Appsec proxy injection annotations (Preview feature)
const (
	// AnnotationInjectorEnabled enables AppSec proxy integration
	AnnotationInjectorEnabled = "agent.datadoghq.com/appsec.injector.enabled"
	// AnnotationInjectorAutoDetect enables auto-detection of supported proxies
	AnnotationInjectorAutoDetect = "agent.datadoghq.com/appsec.injector.autoDetect"
	// AnnotationInjectorProxies is the JSON array of proxy types to inject
	AnnotationInjectorProxies = "agent.datadoghq.com/appsec.injector.proxies"
	// AnnotationInjectorProcessorAddress is the processor service address
	AnnotationInjectorProcessorAddress = "agent.datadoghq.com/appsec.injector.processor.address"
	// AnnotationInjectorProcessorPort is the processor service port
	AnnotationInjectorProcessorPort = "agent.datadoghq.com/appsec.injector.processor.port"
	// AnnotationInjectorProcessorServiceName is the processor service name (required in external mode)
	AnnotationInjectorProcessorServiceName = "agent.datadoghq.com/appsec.injector.processor.service.name"
	// AnnotationInjectorProcessorServiceNamespace is the processor service namespace (optional, cluster-agent will use its own namespace if not specified)
	AnnotationInjectorProcessorServiceNamespace = "agent.datadoghq.com/appsec.injector.processor.service.namespace"
	// AnnotationInjectorMode is the injector mode (sidecar or external)
	AnnotationInjectorMode = "agent.datadoghq.com/appsec.injector.mode"
	// AnnotationSidecarImage is the sidecar container image
	AnnotationSidecarImage = "agent.datadoghq.com/appsec.sidecar.image"
	// AnnotationSidecarImageTag is the sidecar container image tag
	AnnotationSidecarImageTag = "agent.datadoghq.com/appsec.sidecar.image_tag"
	// AnnotationSidecarPort is the sidecar container port
	AnnotationSidecarPort = "agent.datadoghq.com/appsec.sidecar.port"
	// AnnotationSidecarHealthPort is the sidecar container health port
	AnnotationSidecarHealthPort = "agent.datadoghq.com/appsec.sidecar.health_port"
	// AnnotationSidecarResourcesRequestsCPU is the sidecar container CPU request
	AnnotationSidecarResourcesRequestsCPU = "agent.datadoghq.com/appsec.sidecar.resources.requests.cpu"
	// AnnotationSidecarResourcesRequestsMemory is the sidecar container memory request
	AnnotationSidecarResourcesRequestsMemory = "agent.datadoghq.com/appsec.sidecar.resources.requests.memory"
	// AnnotationSidecarResourcesLimitsCPU is the sidecar container CPU limit
	AnnotationSidecarResourcesLimitsCPU = "agent.datadoghq.com/appsec.sidecar.resources.limits.cpu"
	// AnnotationSidecarResourcesLimitsMemory is the sidecar container memory limit
	AnnotationSidecarResourcesLimitsMemory = "agent.datadoghq.com/appsec.sidecar.resources.limits.memory"
	// AnnotationSidecarBodyParsingSizeLimit is the sidecar body parsing size limit
	AnnotationSidecarBodyParsingSizeLimit = "agent.datadoghq.com/appsec.sidecar.body_parsing_size_limit"
)

const (
	// DDAppsecProxyEnabled enables AppSec proxy integration
	DDAppsecProxyEnabled = "DD_APPSEC_PROXY_ENABLED"
	// DDClusterAgentAppsecInjectorEnabled enables the AppSec injector in the cluster agent
	DDClusterAgentAppsecInjectorEnabled = "DD_CLUSTER_AGENT_APPSEC_INJECTOR_ENABLED"
	// DDAppsecProxyAutoDetect enables auto-detection of supported proxies
	DDAppsecProxyAutoDetect = "DD_APPSEC_PROXY_AUTO_DETECT"
	// DDAppsecProxyProxies is the JSON array of proxy types to inject
	DDAppsecProxyProxies = "DD_APPSEC_PROXY_PROXIES"
	// DDAppsecProxyProcessorPort is the processor service port
	DDAppsecProxyProcessorPort = "DD_APPSEC_PROXY_PROCESSOR_PORT"
	// DDAppsecProxyProcessorAddress is the processor service address
	DDAppsecProxyProcessorAddress = "DD_APPSEC_PROXY_PROCESSOR_ADDRESS"
	// DDClusterAgentAppsecInjectorProcessorServiceName is the processor service name
	DDClusterAgentAppsecInjectorProcessorServiceName = "DD_CLUSTER_AGENT_APPSEC_INJECTOR_PROCESSOR_SERVICE_NAME"
	// DDClusterAgentAppsecInjectorProcessorServiceNamespace is the processor service namespace
	DDClusterAgentAppsecInjectorProcessorServiceNamespace = "DD_CLUSTER_AGENT_APPSEC_INJECTOR_PROCESSOR_SERVICE_NAMESPACE"
	// DDClusterAgentAppsecInjectorMode is the injector mode (sidecar or external)
	DDClusterAgentAppsecInjectorMode = "DD_CLUSTER_AGENT_APPSEC_INJECTOR_MODE"
	// DDAdmissionControllerAppsecSidecarImage is the sidecar container image
	DDAdmissionControllerAppsecSidecarImage = "DD_ADMISSION_CONTROLLER_APPSEC_SIDECAR_IMAGE"
	// DDAdmissionControllerAppsecSidecarImageTag is the sidecar container image tag
	DDAdmissionControllerAppsecSidecarImageTag = "DD_ADMISSION_CONTROLLER_APPSEC_SIDECAR_IMAGE_TAG"
	// DDAdmissionControllerAppsecSidecarPort is the sidecar container port
	DDAdmissionControllerAppsecSidecarPort = "DD_ADMISSION_CONTROLLER_APPSEC_SIDECAR_PORT"
	// DDAdmissionControllerAppsecSidecarHealthPort is the sidecar container health port
	DDAdmissionControllerAppsecSidecarHealthPort = "DD_ADMISSION_CONTROLLER_APPSEC_SIDECAR_HEALTH_PORT"
	// DDAdmissionControllerAppsecSidecarResourcesRequestsCPU is the sidecar container CPU request
	DDAdmissionControllerAppsecSidecarResourcesRequestsCPU = "DD_ADMISSION_CONTROLLER_APPSEC_SIDECAR_RESOURCES_REQUESTS_CPU"
	// DDAdmissionControllerAppsecSidecarResourcesRequestsMemory is the sidecar container memory request
	DDAdmissionControllerAppsecSidecarResourcesRequestsMemory = "DD_ADMISSION_CONTROLLER_APPSEC_SIDECAR_RESOURCES_REQUESTS_MEMORY"
	// DDAdmissionControllerAppsecSidecarResourcesLimitsCPU is the sidecar container CPU limit
	DDAdmissionControllerAppsecSidecarResourcesLimitsCPU = "DD_ADMISSION_CONTROLLER_APPSEC_SIDECAR_RESOURCES_LIMITS_CPU"
	// DDAdmissionControllerAppsecSidecarResourcesLimitsMemory is the sidecar container memory limit
	DDAdmissionControllerAppsecSidecarResourcesLimitsMemory = "DD_ADMISSION_CONTROLLER_APPSEC_SIDECAR_RESOURCES_LIMITS_MEMORY"
	// DDAdmissionControllerAppsecSidecarBodyParsingSizeLimit is the sidecar body parsing size limit
	DDAdmissionControllerAppsecSidecarBodyParsingSizeLimit = "DD_ADMISSION_CONTROLLER_APPSEC_SIDECAR_BODY_PARSING_SIZE_LIMIT"
)

var allowedProxyValues = []string{"envoy-gateway", "istio", "istio-gateway"}

// AllowedProxyValues returns the proxy types that the current RBAC supports.
// The returned slice must not be modified.
func AllowedProxyValues() []string {
	result := make([]string, len(allowedProxyValues))
	copy(result, allowedProxyValues)
	return result
}
