// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConfName get the name of the Configmap for a CustomConfigSpec
func GetConfName(owner metav1.Object, conf *CustomConfig, defaultName string) string {
	// `configData` and `configMap` can't be set together.
	// Return the default if the conf is not overridden or if it is just overridden with the ConfigData.
	if conf != nil && conf.ConfigMap != nil {
		return conf.ConfigMap.Name
	}
	return fmt.Sprintf("%s-%s", owner.GetName(), defaultName)
}

// GetClusterAgentServiceAccount return the cluster-agent serviceAccountName
func GetClusterAgentServiceAccount(dda *DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, common.DefaultClusterAgentResourceSuffix)
	if dda.Spec.Override[ClusterAgentComponentName] != nil && dda.Spec.Override[ClusterAgentComponentName].ServiceAccountName != nil {
		return *dda.Spec.Override[ClusterAgentComponentName].ServiceAccountName
	}
	return saDefault
}

// GetAgentServiceAccount returns the agent service account name
func GetAgentServiceAccount(dda *DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, common.DefaultAgentResourceSuffix)
	if dda.Spec.Override[NodeAgentComponentName] != nil && dda.Spec.Override[NodeAgentComponentName].ServiceAccountName != nil {
		return *dda.Spec.Override[NodeAgentComponentName].ServiceAccountName
	}
	return saDefault
}

// GetClusterChecksRunnerServiceAccount return the cluster-checks-runner service account name
func GetClusterChecksRunnerServiceAccount(dda *DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, common.DefaultClusterChecksRunnerResourceSuffix)
	if dda.Spec.Override[ClusterChecksRunnerComponentName] != nil && dda.Spec.Override[ClusterChecksRunnerComponentName].ServiceAccountName != nil {
		return *dda.Spec.Override[ClusterChecksRunnerComponentName].ServiceAccountName
	}
	return saDefault
}

// ConvertCustomConfig use to convert a CustomConfig to a common.CustomConfig.
func ConvertCustomConfig(config *CustomConfig) *commonv1.CustomConfig {
	if config == nil {
		return nil
	}
	var configMap *commonv1.ConfigMapConfig
	if config.ConfigMap != nil {
		configMap = &commonv1.ConfigMapConfig{
			Name:  config.ConfigMap.Name,
			Items: config.ConfigMap.Items,
		}
	}
	return &commonv1.CustomConfig{
		ConfigData: config.ConfigData,
		ConfigMap:  configMap,
	}
}

// IsHostNetworkEnabled returns whether the pod should use the host's network namespace
func IsHostNetworkEnabled(dda *DatadogAgent, component ComponentName) bool {
	if dda.Spec.Override != nil {
		if c, ok := dda.Spec.Override[component]; ok {
			return apiutils.BoolValue(c.HostNetwork)
		}
	}
	return false
}

// IsClusterChecksEnabled returns whether the DDA should use cluster checks
func IsClusterChecksEnabled(dda *DatadogAgent) bool {
	return dda.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(dda.Spec.Features.ClusterChecks.Enabled)
}

// IsCCREnabled returns whether the DDA should use Cluster Checks Runners
func IsCCREnabled(dda *DatadogAgent) bool {
	return dda.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(dda.Spec.Features.ClusterChecks.UseClusterChecksRunners)
}

// GetLocalAgentServiceName returns the name used for the local agent service
func GetLocalAgentServiceName(dda *DatadogAgent) string {
	if dda.Spec.Global.LocalService != nil && dda.Spec.Global.LocalService.NameOverride != nil {
		return *dda.Spec.Global.LocalService.NameOverride
	}
	return fmt.Sprintf("%s-%s", dda.Name, common.DefaultAgentResourceSuffix)
}

// IsNetworkPolicyEnabled returns whether a network policy should be created and which flavor to use
func IsNetworkPolicyEnabled(dda *DatadogAgent) (bool, NetworkPolicyFlavor) {
	if dda.Spec.Global != nil && dda.Spec.Global.NetworkPolicy != nil && apiutils.BoolValue(dda.Spec.Global.NetworkPolicy.Create) {
		if dda.Spec.Global.NetworkPolicy.Flavor != "" {
			return true, dda.Spec.Global.NetworkPolicy.Flavor
		}
		return true, NetworkPolicyFlavorKubernetes
	}
	return false, ""
}

// ShouldCreateSCC returns whether a scc should be created for a component
func ShouldCreateSCC(dda *DatadogAgent, componentName ComponentName) bool {
	if dda.Spec.Override[componentName] != nil && dda.Spec.Override[componentName].SecurityContextConstraints != nil {
		return apiutils.BoolValue(dda.Spec.Override[componentName].SecurityContextConstraints.Create)
	}
	return false
}
