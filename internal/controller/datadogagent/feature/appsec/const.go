// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

const ClusterAgentMinVersion = "7.73.0"

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
	// AnnotationInjectorProcessorServiceName is the processor service name (required)
	AnnotationInjectorProcessorServiceName = "agent.datadoghq.com/appsec.injector.processor.service.name"
	// AnnotationInjectorProcessorServiceNamespace is the processor service namespace (optional, cluster-agent will use its own namespace if not specified)
	AnnotationInjectorProcessorServiceNamespace = "agent.datadoghq.com/appsec.injector.processor.service.namespace"
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
)

var allowedProxyValues = []string{"envoy-gateway", "istio"}

// AllowedProxyValues returns the proxy types that the current RBAC supports.
// The returned slice must not be modified.
func AllowedProxyValues() []string {
	result := make([]string, len(allowedProxyValues))
	copy(result, allowedProxyValues)
	return result
}
