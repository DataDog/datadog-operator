// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package componentmeta

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

// ClusterChecksRunnerMeta provides metadata for the Cluster Checks Runner component
type ClusterChecksRunnerMeta struct{}

func (c *ClusterChecksRunnerMeta) ComponentName() v2alpha1.ComponentName {
	return v2alpha1.ClusterChecksRunnerComponentName
}

func (c *ClusterChecksRunnerMeta) ComponentSuffix() string {
	return constants.DefaultClusterChecksRunnerResourceSuffix
}

func (c *ClusterChecksRunnerMeta) GetDefaultName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), c.ComponentSuffix())
}

func (c *ClusterChecksRunnerMeta) GetWorkloadNameWithOverride(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	name := c.GetDefaultName(dda)
	if ddaSpec != nil && ddaSpec.Override != nil {
		if componentOverride, ok := ddaSpec.Override[c.ComponentName()]; ok {
			if componentOverride.Name != nil && *componentOverride.Name != "" {
				name = *componentOverride.Name
			}
		}
	}
	return name
}

func (c *ClusterChecksRunnerMeta) GetServiceAccountNameWithOverride(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	saDefault := fmt.Sprintf("%s-%s", dda.GetName(), c.ComponentSuffix())
	if ddaSpec != nil && ddaSpec.Override != nil {
		if componentOverride, ok := ddaSpec.Override[c.ComponentName()]; ok {
			if componentOverride.ServiceAccountName != nil && *componentOverride.ServiceAccountName != "" {
				return *componentOverride.ServiceAccountName
			}
		}
	}
	return saDefault
}

func (c *ClusterChecksRunnerMeta) GetRBACResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), c.ComponentSuffix())
}

func (c *ClusterChecksRunnerMeta) GetServiceName(dda metav1.Object) string {
	// Cluster checks runner doesn't have a service
	return ""
}

func (c *ClusterChecksRunnerMeta) GetDefaultServicePort() int32 {
	// Cluster checks runner doesn't have a service
	return 0
}

func (c *ClusterChecksRunnerMeta) GetPDBName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s-pdb", dda.GetName(), c.ComponentSuffix())
}
