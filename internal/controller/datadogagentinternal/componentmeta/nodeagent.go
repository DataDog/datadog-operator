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

// NodeAgentMeta provides metadata for the Node Agent component
type NodeAgentMeta struct{}

func (n *NodeAgentMeta) ComponentName() v2alpha1.ComponentName {
	return v2alpha1.NodeAgentComponentName
}

func (n *NodeAgentMeta) ComponentSuffix() string {
	return constants.DefaultAgentResourceSuffix
}

func (n *NodeAgentMeta) GetDefaultName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), n.ComponentSuffix())
}

func (n *NodeAgentMeta) GetNameWithOverride(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	name := n.GetDefaultName(dda)
	if ddaSpec != nil && ddaSpec.Override != nil {
		if componentOverride, ok := ddaSpec.Override[n.ComponentName()]; ok {
			if componentOverride.Name != nil && *componentOverride.Name != "" {
				name = *componentOverride.Name
			}
		}
	}
	return name
}

func (n *NodeAgentMeta) GetServiceAccountNameWithOverride(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) string {
	saDefault := fmt.Sprintf("%s-%s", dda.GetName(), n.ComponentSuffix())
	if ddaSpec != nil && ddaSpec.Override != nil {
		if componentOverride, ok := ddaSpec.Override[n.ComponentName()]; ok {
			if componentOverride.ServiceAccountName != nil && *componentOverride.ServiceAccountName != "" {
				return *componentOverride.ServiceAccountName
			}
		}
	}
	return saDefault
}

func (n *NodeAgentMeta) GetRBACResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), n.ComponentSuffix())
}

func (n *NodeAgentMeta) GetServiceName(dda metav1.Object) string {
	// Node agent doesn't have a service
	return ""
}

func (n *NodeAgentMeta) GetDefaultServicePort() int32 {
	// Node agent doesn't have a service
	return 0
}

func (n *NodeAgentMeta) GetPDBName(dda metav1.Object) string {
	// Node agent doesn't support PDB
	return ""
}
