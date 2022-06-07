// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
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

	// todo implement the support of override

	return saDefault
}

// GetAgentServiceAccount returns the agent service account name
func GetAgentServiceAccount(dda *DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, common.DefaultAgentResourceSuffix)

	// Todo: implement the support of override
	return saDefault
}

// GetClusterChecksRunnerServiceAccount return the cluster-checks-runner service account name
func GetClusterChecksRunnerServiceAccount(dda *DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, common.DefaultClusterChecksRunnerResourceSuffix)

	// Todo: implement the support of override
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
