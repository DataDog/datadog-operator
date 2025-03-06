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

	agentCustomConfigVolumeName        = "custom-datadog-yaml"
	clusterAgentCustomConfigVolumeName = "custom-cluster-agent-yaml"
)
