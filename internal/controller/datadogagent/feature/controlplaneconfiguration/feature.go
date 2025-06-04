// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplaneconfiguration

import (
	"fmt"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	fmt.Println("Registering control plane configuration feature")
	if err := feature.Register(feature.ControlPlaneConfigurationIDType, buildControlPlaneConfigurationFeature); err != nil {
		fmt.Printf("Failed to register control plane configuration feature: %v\n", err)
		panic(err)
	}
	fmt.Println("Successfully registered control plane configuration feature")
}

func buildControlPlaneConfigurationFeature(options *feature.Options) feature.Feature {
	controlplaneFeat := &controlPlaneConfigurationFeature{
		logger: options.Logger,
	}
	return controlplaneFeat
	// add rbac suffix???
}

type controlPlaneConfigurationFeature struct {
	enabled  bool
	owner    metav1.Object
	logger   logr.Logger
	provider string
}

// ID returns the ID of the Feature
func (f *controlPlaneConfigurationFeature) ID() feature.IDType {
	return feature.ControlPlaneConfigurationIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *controlPlaneConfigurationFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	f.logger.V(1).Info("Configuring control plane configuration feature", "dda", dda.Name, "namespace", dda.Namespace)

	controlPlaneConfiguration := dda.Spec.Features.ControlPlaneConfiguration
	f.logger.V(1).Info("Control plane configuration feature state",
		"feature", feature.ControlPlaneConfigurationIDType,
		"enabled", controlPlaneConfiguration != nil && apiutils.BoolValue(controlPlaneConfiguration.Enabled),
		"config", controlPlaneConfiguration)

	if controlPlaneConfiguration != nil && apiutils.BoolValue(controlPlaneConfiguration.Enabled) {
		f.enabled = true
		f.logger.V(1).Info("Control plane configuration feature enabled",
			"feature", feature.ControlPlaneConfigurationIDType,
			"requiredComponents", reqComp)
		reqComp.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)
		reqComp.ClusterAgent.Containers = []common.AgentContainerName{common.ClusterAgentContainerName}
		f.logger.V(1).Info("Control plane configuration feature requirements set",
			"feature", feature.ControlPlaneConfigurationIDType,
			"requiredComponents", reqComp)
	}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *controlPlaneConfigurationFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneConfigurationFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	// print feature name
	fmt.Println("controlplaneconfiguration feature")
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneConfigurationFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneConfigurationFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.provider = provider
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneConfigurationFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
