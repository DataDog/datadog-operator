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

// ClusterAgentMeta provides metadata for the Cluster Agent component
type ClusterAgentMeta struct{}

func (c *ClusterAgentMeta) ComponentName() v2alpha1.ComponentName {
	return v2alpha1.ClusterAgentComponentName
}

func (c *ClusterAgentMeta) ComponentSuffix() string {
	return constants.DefaultClusterAgentResourceSuffix
}

func (c *ClusterAgentMeta) GetDefaultName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), c.ComponentSuffix())
}

func (c *ClusterAgentMeta) GetWorkloadNameWithOverride(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) string {
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

func (c *ClusterAgentMeta) GetServiceAccountNameWithOverride(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) string {
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

func (c *ClusterAgentMeta) GetRBACResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), c.ComponentSuffix())
}

func (c *ClusterAgentMeta) GetServiceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", constants.GetDDAName(dda), c.ComponentSuffix())
}

func (c *ClusterAgentMeta) GetDefaultServicePort() int32 {
	return 5005
}

func (c *ClusterAgentMeta) GetPDBName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s-pdb", dda.GetName(), c.ComponentSuffix())
}
