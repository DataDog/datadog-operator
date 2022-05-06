// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/apis/utils"
	v1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts a v1alpha1 to v2alpha1 (Hub)
func (src *DatadogAgent) ConvertTo(dst conversion.Hub) error {
	ddaV2 := dst.(*v2alpha1.DatadogAgent)

	if err := convertTo(src, ddaV2); err != nil {
		return fmt.Errorf("unable to convert DatadogAgent %s/%s to version: %v, err: %w", src.Namespace, src.Name, dst.GetObjectKind().GroupVersionKind().Version, err)
	}

	return nil
}

// ConvertFrom converts a v2alpha1 (Hub) to v1alpha1 (local)
// Not implemented
func (dst *DatadogAgent) ConvertFrom(src conversion.Hub) error { //nolint
	return fmt.Errorf("convert from v2alpha1 to %s is not implemented", src.GetObjectKind().GroupVersionKind().Version)
}

func convertTo(src *DatadogAgent, dst *v2alpha1.DatadogAgent) error {
	// Copying ObjectMeta as a whole
	dst.ObjectMeta = src.ObjectMeta

	// Convert spec
	if err := convertSpec(&src.Spec, dst); err != nil {
		return err
	}

	// Not converting status, will let the operator generate a new one

	return nil
}

// Convert the top level structs
func convertSpec(src *DatadogAgentSpec, dst *v2alpha1.DatadogAgent) error {
	if src == nil {
		return nil
	}

	// Convert credentials
	if src.Credentials != nil {
		if dstCred := convertCredentials(&src.Credentials.DatadogCredentials); dstCred != nil {
			getV2GlobalConfig(dst).Credentials = dstCred
		}
	}

	// Convert features first, it's up to other functions to handle duplicate fields
	convertFeatures(src.Features, dst)

	// Convert Specs
	convertDatadogAgentSpec(&src.Agent, dst)
	convertClusterAgentSpec(&src.ClusterAgent, dst)
	convertCCRSpec(&src.ClusterChecksRunner, dst)

	// Convert some settings
	if src.ClusterName != "" {
		getV2GlobalConfig(dst).ClusterName = &src.ClusterName
	}

	if src.Site != "" {
		getV2GlobalConfig(dst).Site = &src.Site
	}

	if src.Registry != nil {
		getV2GlobalConfig(dst).Registry = src.Registry
	}

	return nil
}

// Ad-hoc conversion for major structs
func convertFeatures(src DatadogFeatures, dst *v2alpha1.DatadogAgent) {
	dstFeatures := getV2Features(dst)

	if src.OrchestratorExplorer != nil {
		dstFeatures.OrchestratorExplorer = &v2alpha1.OrchestratorExplorerFeatureConfig{
			Enabled:   src.OrchestratorExplorer.Enabled,
			Conf:      convertConfigMapConfig(src.OrchestratorExplorer.Conf),
			ExtraTags: src.OrchestratorExplorer.ExtraTags,
		}

		if src.OrchestratorExplorer.Scrubbing != nil {
			dstFeatures.OrchestratorExplorer.ScrubContainers = src.OrchestratorExplorer.Scrubbing.Containers
		}

		if src.OrchestratorExplorer.DDUrl != nil {
			dstFeatures.OrchestratorExplorer.Endpoint = &v2alpha1.Endpoint{
				URL: src.OrchestratorExplorer.DDUrl,
			}
		}
		// TODO: Handle src ClusterChecks + AdditionalEndpoints, seems to be missing from V2
	}

	if src.KubeStateMetricsCore != nil {
		dstFeatures.KubeStateMetricsCore = &v2alpha1.KubeStateMetricsCoreFeatureConfig{
			Enabled: src.KubeStateMetricsCore.Enabled,
			Conf:    convertConfigMapConfig(src.KubeStateMetricsCore.Conf),
		}
	}

	if src.PrometheusScrape != nil {
		dstFeatures.PrometheusScrape = &v2alpha1.PrometheusScrapeFeatureConfig{
			Enabled:                src.PrometheusScrape.Enabled,
			EnableServiceEndpoints: src.PrometheusScrape.ServiceEndpoints,
			AdditionalConfigs:      src.PrometheusScrape.AdditionalConfigs,
		}
	}

	if src.NetworkMonitoring != nil {
		dstFeatures.NPM = &v2alpha1.NPMFeatureConfig{
			Enabled: src.NetworkMonitoring.Enabled,
		}
	}

	if src.LogCollection != nil {
		dstFeatures.LogCollection = &v2alpha1.LogCollectionFeatureConfig{
			Enabled:                    src.LogCollection.Enabled,
			ContainerCollectAll:        src.LogCollection.LogsConfigContainerCollectAll,
			ContainerCollectUsingFiles: src.LogCollection.ContainerCollectUsingFiles,
			ContainerLogsPath:          src.LogCollection.ContainerLogsPath,
			PodLogsPath:                src.LogCollection.PodLogsPath,
			ContainerSymlinksPath:      src.LogCollection.ContainerSymlinksPath,
			TempStoragePath:            src.LogCollection.TempStoragePath,
			OpenFilesLimit:             src.LogCollection.OpenFilesLimit,
		}
	}
}

// Converting internal structs
func convertCredentials(src *DatadogCredentials) *v2alpha1.DatadogCredentials {
	if src == nil {
		return nil
	}

	creds := &v2alpha1.DatadogCredentials{
		APISecret: src.APISecret,
		AppSecret: src.APPSecret,
	}

	if src.APIKey != "" {
		creds.APIKey = &src.APIKey
	}
	if src.AppKey != "" {
		creds.AppKey = &src.AppKey
	}

	return creds
}

func convertConfigMapConfig(src *CustomConfigSpec) *v2alpha1.CustomConfig {
	if src == nil {
		return nil
	}

	dstConfig := &v2alpha1.CustomConfig{
		ConfigData: src.ConfigData,
	}

	if src.ConfigMap != nil {
		dstConfig.ConfigMap = &commonv1.ConfigMapConfig{
			Name: src.ConfigMap.Name,
		}

		// TODO: Check if that was the intended usage of `FileKey`
		if src.ConfigMap.FileKey != "" {
			dstConfig.ConfigMap.Items = append(dstConfig.ConfigMap.Items, v1.KeyToPath{
				Key: src.ConfigMap.FileKey,
			})
		}
	}

	return dstConfig
}

func convertConfigDirSpec(src *ConfigDirSpec) *v2alpha1.CustomConfig {
	if src == nil {
		return nil
	}

	return &v2alpha1.CustomConfig{
		ConfigMap: &commonv1.ConfigMapConfig{
			Name:  src.ConfigMapName,
			Items: src.Items,
		},
	}
}

// Accessors
func getV2GlobalConfig(dst *v2alpha1.DatadogAgent) *v2alpha1.GlobalConfig {
	if dst.Spec.Global == nil {
		dst.Spec.Global = &v2alpha1.GlobalConfig{}
	}

	return dst.Spec.Global
}

func getV2Features(dst *v2alpha1.DatadogAgent) *v2alpha1.DatadogFeatures {
	if dst.Spec.Features != nil {
		return dst.Spec.Features
	}

	dst.Spec.Features = &v2alpha1.DatadogFeatures{}
	return dst.Spec.Features
}

func getV2TemplateOverride(dst *v2alpha1.DatadogAgentSpec, component v2alpha1.ComponentName) *v2alpha1.DatadogAgentComponentOverride {
	if override := dst.Override[component]; override != nil {
		return override
	}

	if dst.Override == nil {
		dst.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)
	}

	override := &v2alpha1.DatadogAgentComponentOverride{}
	dst.Override[component] = override
	return override
}

func getV2Container(comp *v2alpha1.DatadogAgentComponentOverride, containerName commonv1.AgentContainerName) *v2alpha1.DatadogAgentGenericContainer {
	if cont := comp.Containers[containerName]; cont != nil {
		return cont
	}

	if comp.Containers == nil {
		comp.Containers = make(map[commonv1.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer)
	}

	cont := &v2alpha1.DatadogAgentGenericContainer{}
	comp.Containers[containerName] = cont
	return cont
}

// Utils
func setBooleanPtrOR(src *bool, dst **bool) {
	if src == nil {
		return
	}

	if *dst == nil {
		*dst = src
	}

	*dst = utils.NewBoolPointer(**dst || *src)
}
