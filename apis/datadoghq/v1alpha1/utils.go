// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConfName get the name of the Configmap for a CustomConfigSpec
func GetConfName(owner metav1.Object, conf *CustomConfigSpec, defaultName string) string {
	// `configData` and `configMap` can't be set together.
	// Return the default if the conf is not overridden or if it is just overridden with the ConfigData.
	if conf != nil && conf.ConfigMap != nil {
		return conf.ConfigMap.Name
	}
	return fmt.Sprintf("%s-%s", owner.GetName(), defaultName)
}

// GetAgentServiceAccount returns the agent serviceAccountName
func GetAgentServiceAccount(dda *DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, common.DefaultAgentResourceSuffix)

	if dda.Spec.Agent.Rbac != nil && dda.Spec.Agent.Rbac.ServiceAccountName != nil {
		return *dda.Spec.Agent.Rbac.ServiceAccountName
	}
	return saDefault
}

// GetClusterAgentServiceAccount return the cluster-agent serviceAccountName
func GetClusterAgentServiceAccount(dda *DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, common.DefaultClusterAgentResourceSuffix)
	if !isClusterAgentEnabled(dda.Spec.ClusterAgent) {
		return saDefault
	}
	if dda.Spec.ClusterAgent.Rbac != nil && dda.Spec.ClusterAgent.Rbac.ServiceAccountName != nil {
		return *dda.Spec.ClusterAgent.Rbac.ServiceAccountName
	}
	return saDefault
}

// GetClusterChecksRunnerServiceAccount return the cluster-checks-runner service account name
func GetClusterChecksRunnerServiceAccount(dda *DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, common.DefaultClusterChecksRunnerResourceSuffix)

	if !apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) {
		return saDefault
	}
	if dda.Spec.ClusterChecksRunner.Rbac.ServiceAccountName != nil {
		return *dda.Spec.ClusterChecksRunner.Rbac.ServiceAccountName
	}
	return saDefault
}

func isClusterAgentEnabled(spec DatadogAgentSpecClusterAgentSpec) bool {
	return apiutils.BoolValue(spec.Enabled)
}

// ConvertCustomConfig use to convert a CustomConfigSpec to a common.CustomConfig.
func ConvertCustomConfig(config *CustomConfigSpec) *commonv1.CustomConfig {
	if config == nil {
		return nil
	}

	var configMap *commonv1.ConfigMapConfig
	if config.ConfigMap != nil {
		configMap = &commonv1.ConfigMapConfig{
			Name: config.ConfigMap.Name,
			Items: []corev1.KeyToPath{
				{
					Key:  config.ConfigMap.FileKey,
					Path: config.ConfigMap.FileKey,
				},
			},
		}
	}
	return &commonv1.CustomConfig{
		ConfigData: config.ConfigData,
		ConfigMap:  configMap,
	}
}

// IsHostNetworkEnabled returns whether the pod should use the host's network namespace
func IsHostNetworkEnabled(dda *DatadogAgent) bool {
	return apiutils.BoolValue(&dda.Spec.Agent.HostNetwork)
}

// IsClusterChecksEnabled returns whether the DDA should use cluster checks
func IsClusterChecksEnabled(dda *DatadogAgent) bool {
	return dda.Spec.ClusterAgent.Config != nil && apiutils.BoolValue(dda.Spec.ClusterAgent.Config.ClusterChecksEnabled)
}

// IsCCREnabled returns whether the DDA should use Cluster Checks Runners
func IsCCREnabled(dda *DatadogAgent) bool {
	return apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled)
}
