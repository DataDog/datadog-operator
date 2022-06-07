// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"fmt"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetClusterAgentServiceName return the Cluster-Agent service name based on the DatadogAgent name
func GetClusterAgentServiceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentName return the Cluster-Agent name based on the DatadogAgent name
func GetClusterAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentVersion return the Cluster-Agent version based on the DatadogAgent info
func GetClusterAgentVersion(dda metav1.Object) string {
	// Todo implement this function
	return ""
}

// GetClusterAgentRbacResourcesName return the Cluster-Agent RBAC resource name
func GetClusterAgentRbacResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}
