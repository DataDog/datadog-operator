// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

// This file tracks constants used in overrides

const (
	// extraConfdConfigMapName is the name of the ConfigMap storing Custom Confd data
	extraConfdConfigMapName = "%s-extra-confd"
	// extraChecksdConfigMapName is the name of the ConfigMap storing Custom Checksd data
	extraChecksdConfigMapName = "%s-extra-checksd"

	kubeletCAVolumeName                = "kubelet-ca"
	agentCustomConfigVolumeName        = "custom-datadog-yaml"
	clusterAgentCustomConfigVolumeName = "custom-cluster-agent-yaml"

	FIPSProxyCustomConfigVolumeName = "fips-proxy-cfg"
	FIPSProxyCustomConfigFileName   = "datadog-fips-proxy.cfg"
	FIPSProxyCustomConfigMapName    = "%s-fips-config"
	FIPSProxyCustomConfigMountPath  = "/etc/datadog-fips-proxy/datadog-fips-proxy.cfg"
)
