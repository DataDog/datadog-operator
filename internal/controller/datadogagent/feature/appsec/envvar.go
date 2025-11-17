// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

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
