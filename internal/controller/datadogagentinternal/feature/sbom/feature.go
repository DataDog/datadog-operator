// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/object/volume"
)

func init() {
	err := feature.Register(feature.SBOMIDType, buildSBOMFeature)
	if err != nil {
		panic(err)
	}
}

func buildSBOMFeature(options *feature.Options) feature.Feature {
	sbomFeature := &sbomFeature{}

	if options != nil {
		sbomFeature.logger = options.Logger
	}

	return sbomFeature
}

type sbomFeature struct {
	owner  metav1.Object
	logger logr.Logger

	enabled                                 bool
	containerImageEnabled                   bool
	containerImageAnalyzers                 []string
	containerImageUncompressedLayersSupport bool
	containerImageOverlayFSDirectScan       bool
	hostEnabled                             bool
	hostAnalyzers                           []string
}

// ID returns the ID of the Feature
func (f *sbomFeature) ID() feature.IDType {
	return feature.SBOMIDType
}

// Configure is used to configure the feature from a v1alpha1.DatadogAgentInternal instance.
func (f *sbomFeature) Configure(ddai *v1alpha1.DatadogAgentInternal) (reqComp feature.RequiredComponents) {
	f.owner = ddai

	// Merge configuration from Status.RemoteConfigConfiguration into the Spec
	mergeConfigs(&ddai.Spec, &ddai.Status)

	sbomConfig := ddai.Spec.Features.SBOM

	if sbomConfig != nil && apiutils.BoolValue(sbomConfig.Enabled) {
		f.enabled = true
		if sbomConfig.ContainerImage != nil && apiutils.BoolValue(sbomConfig.ContainerImage.Enabled) {
			f.containerImageEnabled = true
			f.containerImageAnalyzers = sbomConfig.ContainerImage.Analyzers
			f.containerImageUncompressedLayersSupport = sbomConfig.ContainerImage.UncompressedLayersSupport
			f.containerImageOverlayFSDirectScan = sbomConfig.ContainerImage.OverlayFSDirectScan
		}
		if sbomConfig.Host != nil && apiutils.BoolValue(sbomConfig.Host.Enabled) {
			f.hostEnabled = true
			f.hostAnalyzers = sbomConfig.Host.Analyzers
		}
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

func mergeConfigs(ddaSpec *v2alpha1.DatadogAgentSpec, ddaiStatus *v1alpha1.DatadogAgentInternalStatus) {
	if ddaiStatus.RemoteConfigConfiguration == nil || ddaiStatus.RemoteConfigConfiguration.Features == nil || ddaiStatus.RemoteConfigConfiguration.Features.SBOM == nil || ddaiStatus.RemoteConfigConfiguration.Features.SBOM.Enabled == nil {
		return
	}

	if ddaSpec.Features == nil {
		ddaSpec.Features = &v2alpha1.DatadogFeatures{}
	}

	if ddaSpec.Features.SBOM == nil {
		ddaSpec.Features.SBOM = &v2alpha1.SBOMFeatureConfig{}
	}

	if ddaiStatus.RemoteConfigConfiguration.Features.SBOM.Enabled != nil {
		ddaSpec.Features.SBOM.Enabled = ddaiStatus.RemoteConfigConfiguration.Features.SBOM.Enabled
	}

	if ddaiStatus.RemoteConfigConfiguration.Features.SBOM.Host != nil && ddaiStatus.RemoteConfigConfiguration.Features.SBOM.Host.Enabled != nil {
		if ddaSpec.Features.SBOM.Host == nil {
			ddaSpec.Features.SBOM.Host = &v2alpha1.SBOMHostConfig{}
		}
		ddaSpec.Features.SBOM.Host.Enabled = ddaiStatus.RemoteConfigConfiguration.Features.SBOM.Host.Enabled
	}

	if ddaiStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage != nil && ddaiStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage.Enabled != nil {
		if ddaSpec.Features.SBOM.ContainerImage == nil {
			ddaSpec.Features.SBOM.ContainerImage = &v2alpha1.SBOMContainerImageConfig{}
		}
		ddaSpec.Features.SBOM.ContainerImage.Enabled = ddaiStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage.Enabled
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *sbomFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *sbomFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (p sbomFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	// This feature doesn't set env vars on specific containers, so no specific logic for the single agent
	p.ManageNodeAgent(managers)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *sbomFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
		Name:  DDSBOMEnabled,
		Value: apiutils.BoolToString(&f.enabled),
	})

	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
		Name:  DDSBOMContainerImageEnabled,
		Value: apiutils.BoolToString(&f.containerImageEnabled),
	})
	if len(f.containerImageAnalyzers) > 0 {
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  DDSBOMContainerImageAnalyzers,
			Value: strings.Join(f.containerImageAnalyzers, " "),
		})
	}
	if f.containerImageUncompressedLayersSupport {
		if f.containerImageOverlayFSDirectScan {
			managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
				Name:  DDSBOMContainerOverlayFSDirectScan,
				Value: "true",
			})
		} else {
			managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
				Name:  DDSBOMContainerUseMount,
				Value: "true",
			})

			managers.SecurityContext().AddCapabilitiesToContainer(
				[]corev1.Capability{"SYS_ADMIN"},
				apicommon.CoreAgentContainerName,
			)

			managers.Annotation().AddAnnotation(agentAppArmorAnnotationKey, agentAppArmorAnnotationValue)
		}

		volMgr := managers.Volume()
		volMountMgr := managers.VolumeMount()

		containerdLibVol, containerdLibVolMount := volume.GetVolumes(containerdDirVolumeName, containerdDirVolumePath, containerdDirMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&containerdLibVolMount, apicommon.CoreAgentContainerName)
		volMgr.AddVolume(&containerdLibVol)

		criLibVol, criLibVolMount := volume.GetVolumes(criDirVolumeName, criDirVolumePath, criDirMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&criLibVolMount, apicommon.CoreAgentContainerName)
		volMgr.AddVolume(&criLibVol)
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
		Name:  DDSBOMHostEnabled,
		Value: apiutils.BoolToString(&f.hostEnabled),
	})
	if len(f.hostAnalyzers) > 0 {
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  DDSBOMHostAnalyzers,
			Value: strings.Join(f.hostAnalyzers, " "),
		})
	}

	if f.hostEnabled {
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  common.DDHostRootEnvVar,
			Value: common.HostRootMountPath,
		})

		volMgr := managers.Volume()
		volMountMgr := managers.VolumeMount()

		hostRootVol, hostRootVolMount := volume.GetVolumes(common.HostRootVolumeName, common.HostRootHostPath, common.HostRootMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&hostRootVolMount, apicommon.CoreAgentContainerName)
		volMgr.AddVolume(&hostRootVol)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *sbomFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
