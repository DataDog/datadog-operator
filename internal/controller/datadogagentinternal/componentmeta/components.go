// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package componentmeta

import "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"

// ClusterAgent returns ClusterAgent metadata
func ClusterAgent() ComponentMeta {
	return &ClusterAgentMeta{}
}

// ClusterChecksRunner returns ClusterChecksRunner metadata
func ClusterChecksRunner() ComponentMeta {
	return &ClusterChecksRunnerMeta{}
}

// Get returns ComponentMeta by name (for dynamic lookup if needed)
func Get(name v2alpha1.ComponentName) ComponentMeta {
	switch name {
	case v2alpha1.ClusterAgentComponentName:
		return &ClusterAgentMeta{}
	case v2alpha1.ClusterChecksRunnerComponentName:
		return &ClusterChecksRunnerMeta{}
	default:
		return nil
	}
}

