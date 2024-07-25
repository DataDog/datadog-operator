// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/go-logr/logr"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
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

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *sbomFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	// Merge configuration from Status.RemoteConfigConfiguration into the Spec
	mergeConfigs(&dda.Spec, &dda.Status)

	sbomConfig := dda.Spec.Features.SBOM

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
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

func mergeConfigs(ddaSpec *v2alpha1.DatadogAgentSpec, ddaStatus *v2alpha1.DatadogAgentStatus) {
	if ddaStatus.RemoteConfigConfiguration == nil || ddaStatus.RemoteConfigConfiguration.Features == nil || ddaStatus.RemoteConfigConfiguration.Features.SBOM == nil || ddaStatus.RemoteConfigConfiguration.Features.SBOM.Enabled == nil {
		return
	}

	if ddaSpec.Features == nil {
		ddaSpec.Features = &v2alpha1.DatadogFeatures{}
	}

	if ddaSpec.Features.SBOM == nil {
		ddaSpec.Features.SBOM = &v2alpha1.SBOMFeatureConfig{}
	}

	if ddaStatus.RemoteConfigConfiguration.Features.SBOM.Enabled != nil {
		ddaSpec.Features.SBOM.Enabled = ddaStatus.RemoteConfigConfiguration.Features.SBOM.Enabled
	}

	if ddaStatus.RemoteConfigConfiguration.Features.SBOM.Host != nil && ddaStatus.RemoteConfigConfiguration.Features.SBOM.Host.Enabled != nil {
		if ddaSpec.Features.SBOM.Host == nil {
			ddaSpec.Features.SBOM.Host = &v2alpha1.SBOMHostConfig{}
		}
		ddaSpec.Features.SBOM.Host.Enabled = ddaStatus.RemoteConfigConfiguration.Features.SBOM.Host.Enabled
	}

	if ddaStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage != nil && ddaStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage.Enabled != nil {
		if ddaSpec.Features.SBOM.ContainerImage == nil {
			ddaSpec.Features.SBOM.ContainerImage = &v2alpha1.SBOMContainerImageConfig{}
		}
		ddaSpec.Features.SBOM.ContainerImage.Enabled = ddaStatus.RemoteConfigConfiguration.Features.SBOM.ContainerImage.Enabled
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *sbomFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *sbomFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (p sbomFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// This feature doesn't set env vars on specific containers, so no specific logic for the single agent
	p.ManageNodeAgent(managers, provider)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *sbomFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDSBOMEnabled,
		Value: apiutils.BoolToString(&f.enabled),
	})

	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDSBOMContainerImageEnabled,
		Value: apiutils.BoolToString(&f.containerImageEnabled),
	})
	if len(f.containerImageAnalyzers) > 0 {
		managers.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDSBOMContainerImageAnalyzers,
			Value: strings.Join(f.containerImageAnalyzers, " "),
		})
	}
	if f.containerImageUncompressedLayersSupport {
		if f.containerImageOverlayFSDirectScan {
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDSBOMContainerOverlayFSDirectScan,
				Value: "true",
			})
		} else {
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDSBOMContainerUseMount,
				Value: "true",
			})

			managers.SecurityContext().AddCapabilitiesToContainer(
				[]corev1.Capability{"SYS_ADMIN"},
				apicommonv1.CoreAgentContainerName,
			)

			managers.Annotation().AddAnnotation(apicommon.AgentAppArmorAnnotationKey, apicommon.AgentAppArmorAnnotationValue)
		}

		volMgr := managers.Volume()
		volMountMgr := managers.VolumeMount()

		containerdLibVol, containerdLibVolMount := volume.GetVolumes(apicommon.ContainerdDirVolumeName, apicommon.ContainerdDirVolumePath, apicommon.ContainerdDirMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&containerdLibVolMount, apicommonv1.CoreAgentContainerName)
		volMgr.AddVolume(&containerdLibVol)
	}

	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDSBOMHostEnabled,
		Value: apiutils.BoolToString(&f.hostEnabled),
	})
	if len(f.hostAnalyzers) > 0 {
		managers.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDSBOMHostAnalyzers,
			Value: strings.Join(f.hostAnalyzers, " "),
		})
	}

	if f.hostEnabled {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDHostRootEnvVar,
			Value: "/host",
		})

		volMgr := managers.Volume()
		volMountMgr := managers.VolumeMount()

		osReleaseVol, osReleaseVolMount := volume.GetVolumes(apicommon.SystemProbeOSReleaseDirVolumeName, apicommon.SystemProbeOSReleaseDirVolumePath, apicommon.SystemProbeOSReleaseDirMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&osReleaseVolMount, apicommonv1.CoreAgentContainerName)
		volMgr.AddVolume(&osReleaseVol)

		hostApkVol, hostApkVolMount := volume.GetVolumes(apicommon.ApkDirVolumeName, apicommon.ApkDirVolumePath, apicommon.ApkDirMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&hostApkVolMount, apicommonv1.CoreAgentContainerName)
		volMgr.AddVolume(&hostApkVol)

		hostDpkgVol, hostDpkgVolMount := volume.GetVolumes(apicommon.DpkgDirVolumeName, apicommon.DpkgDirVolumePath, apicommon.DpkgDirMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&hostDpkgVolMount, apicommonv1.CoreAgentContainerName)
		volMgr.AddVolume(&hostDpkgVol)

		hostRpmVol, hostRpmVolMount := volume.GetVolumes(apicommon.RpmDirVolumeName, apicommon.RpmDirVolumePath, apicommon.RpmDirMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&hostRpmVolMount, apicommonv1.CoreAgentContainerName)
		volMgr.AddVolume(&hostRpmVol)

		hostRedhatReleaseVol, hostRedhatReleaseVolMount := volume.GetVolumes(apicommon.RedhatReleaseVolumeName, apicommon.RedhatReleaseVolumePath, apicommon.RedhatReleaseMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&hostRedhatReleaseVolMount, apicommonv1.CoreAgentContainerName)
		volMgr.AddVolume(&hostRedhatReleaseVol)

		hostFedoraReleaseVol, hostFedoraReleaseVolMount := volume.GetVolumes(apicommon.FedoraReleaseVolumeName, apicommon.FedoraReleaseVolumePath, apicommon.FedoraReleaseMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&hostFedoraReleaseVolMount, apicommonv1.CoreAgentContainerName)
		volMgr.AddVolume(&hostFedoraReleaseVol)

		hostLsbReleaseVol, hostLsbReleaseVolMount := volume.GetVolumes(apicommon.LsbReleaseVolumeName, apicommon.LsbReleaseVolumePath, apicommon.LsbReleaseMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&hostLsbReleaseVolMount, apicommonv1.CoreAgentContainerName)
		volMgr.AddVolume(&hostLsbReleaseVol)

		hostSystemReleaseVol, hostSystemReleaseVolMount := volume.GetVolumes(apicommon.SystemReleaseVolumeName, apicommon.SystemReleaseVolumePath, apicommon.SystemReleaseMountPath, true)
		volMountMgr.AddVolumeMountToContainer(&hostSystemReleaseVolMount, apicommonv1.CoreAgentContainerName)
		volMgr.AddVolume(&hostSystemReleaseVol)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *sbomFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
