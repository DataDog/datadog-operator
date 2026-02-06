// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	err := feature.Register(feature.PrivateActionRunnerIDType, buildPrivateActionRunnerFeature)
	if err != nil {
		panic(err)
	}
}

func buildPrivateActionRunnerFeature(options *feature.Options) feature.Feature {
	parFeat := &privateActionRunnerFeature{}
	if options != nil {
		parFeat.logger = options.Logger
	}
	return parFeat
}

type privateActionRunnerFeature struct {
	owner       metav1.Object
	logger      logr.Logger
	nodeEnabled bool
	configData  string
}

// ID returns the ID of the Feature
func (f *privateActionRunnerFeature) ID() feature.IDType {
	return feature.PrivateActionRunnerIDType
}

const defaultConfigData = "privateactionrunner:\n    enabled: true\n"

// Configure configures the feature from annotations on the DatadogAgent object.
func (f *privateActionRunnerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	// Check if feature is enabled via annotation
	if !featureutils.HasPrivateActionRunnerAnnotation(dda) {
		return feature.RequiredComponents{}
	}

	f.nodeEnabled = true

	// Use config data from annotation directly, or fall back to default
	if configData, ok := featureutils.HasPrivateActionRunnerConfigAnnotation(dda, featureutils.PrivateActionRunnerConfigDataAnnotation); ok {
		f.configData = configData
	} else {
		f.configData = defaultConfigData
	}

	reqComp.Agent = feature.RequiredComponent{
		IsRequired: apiutils.NewBoolPointer(true),
		Containers: []apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
			apicommon.PrivateActionRunnerContainerName,
		},
	}

	return reqComp
}

const (
	PrivateActionRunnerConfigPath = "/etc/datadog-agent/privateactionrunner.yaml"
	privateActionRunnerVolumeName = "privateactionrunner-config"
)

// ManageDependencies allows a feature to manage its dependencies.
func (f *privateActionRunnerFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	if !f.nodeEnabled {
		return nil
	}

	// Create ConfigMap with the config content (either from annotation or default)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.getConfigMapName(),
			Namespace: f.owner.GetNamespace(),
		},
		Data: map[string]string{
			"privateactionrunner.yaml": f.configData,
		},
	}

	if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
		return err
	}

	return nil
}

func (f *privateActionRunnerFeature) getConfigMapName() string {
	return fmt.Sprintf("%s-privateactionrunner", f.owner.GetName())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	// Cluster Agent support not yet implemented
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	if !f.nodeEnabled {
		return nil
	}

	configMapName := f.getConfigMapName()

	cmConfig := &v2alpha1.ConfigMapConfig{
		Name: configMapName,
	}
	vol := volume.GetVolumeFromConfigMap(cmConfig, configMapName, privateActionRunnerVolumeName)
	managers.Volume().AddVolume(&vol)

	volMount := corev1.VolumeMount{
		Name:      privateActionRunnerVolumeName,
		MountPath: PrivateActionRunnerConfigPath,
		SubPath:   "privateactionrunner.yaml",
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.PrivateActionRunnerContainerName)

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Private Action Runner requires separate container, not compatible with single-container mode
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	// Private Action Runner doesn't run in cluster checks runner
	return nil
}

// ManageOtelAgentGateway allows a feature to configure the OtelAgentGateway's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	// Private Action Runner doesn't run in OTel Agent Gateway
	return nil
}
