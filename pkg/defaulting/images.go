// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2012 Datadog, Inc.

package defaulting

import "fmt"

const (
	// AgentLatestVersion correspond to the latest stable agent release
	AgentLatestVersion = "7.30.0"
	// ClusterAgentLatestVersion correspond to the latest stable cluster-agent release
	ClusterAgentLatestVersion = "1.14.0"

	// GCRContainerRegistry correspond to the datadoghq GCR registry
	GCRContainerRegistry = "gcr.io/datadoghq"
	// DefaultImageRegistry correspond to the datadoghq containers registry
	DefaultImageRegistry = GCRContainerRegistry
	// JMXTagSuffix prefix tag for agent JMX images
	JMXTagSuffix = "-jmx"
)

// GetLatestAgentImage return the latest stable agent release version
func GetLatestAgentImage() string {
	return fmt.Sprintf("%s/agent:%s", DefaultImageRegistry, AgentLatestVersion)
}

// GetLatestAgentImageJMX return the latest JMX stable agent release version
func GetLatestAgentImageJMX() string {
	return fmt.Sprintf("%s%s", GetLatestAgentImage(), JMXTagSuffix)
}

// GetLatestClusterAgentImage return the latest stable agent release version
func GetLatestClusterAgentImage() string {
	return fmt.Sprintf("%s/cluster-agent:%s", DefaultImageRegistry, ClusterAgentLatestVersion)
}
