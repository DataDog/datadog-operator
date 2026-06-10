// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package katacontainers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

const (
	kataVcSbsVolumeName = "kata-vc-sbs"
	kataVcSbsHostPath   = "/run/vc/sbs"
	kataVcSbsMountPath  = "/host/run/vc/sbs"
	kataRunVolumeName   = "kata-run"
	kataRunHostPath     = "/run/kata"
	kataRunMountPath    = "/host/run/kata"
)

func init() {
	if err := feature.Register(feature.KataContainersIDType, buildFeature); err != nil {
		panic(err)
	}
}

func buildFeature(*feature.Options) feature.Feature {
	return &kataContainersFeature{}
}

type kataContainersFeature struct{}

func (f *kataContainersFeature) ID() feature.IDType {
	return feature.KataContainersIDType
}

func (f *kataContainersFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	if ddaSpec.Features == nil || ddaSpec.Features.KataContainers == nil || !apiutils.BoolValue(ddaSpec.Features.KataContainers.Enabled) {
		return reqComp
	}

	reqComp.Agent = feature.RequiredComponent{
		IsRequired: ptr.To(true),
		Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
	}
	return reqComp
}

func (f *kataContainersFeature) ManageDependencies(feature.ResourceManagers, string) error {
	return nil
}

func (f *kataContainersFeature) ManageClusterAgent(feature.PodTemplateManagers, string) error {
	return nil
}

func (f *kataContainersFeature) ManageNodeAgent(managers feature.PodTemplateManagers, _ string) error {
	vcSbsVol, vcSbsMount := volume.GetVolumes(kataVcSbsVolumeName, kataVcSbsHostPath, kataVcSbsMountPath, true)
	managers.Volume().AddVolume(&vcSbsVol)
	managers.VolumeMount().AddVolumeMountToContainer(&vcSbsMount, apicommon.CoreAgentContainerName)

	kataRunVol, kataRunMount := volume.GetVolumes(kataRunVolumeName, kataRunHostPath, kataRunMountPath, true)
	managers.Volume().AddVolume(&kataRunVol)
	managers.VolumeMount().AddVolumeMountToContainer(&kataRunMount, apicommon.CoreAgentContainerName)

	return nil
}

func (f *kataContainersFeature) ManageSingleContainerNodeAgent(feature.PodTemplateManagers, string) error {
	return nil
}

func (f *kataContainersFeature) ManageClusterChecksRunner(feature.PodTemplateManagers, string) error {
	return nil
}

func (f *kataContainersFeature) ManageOtelAgentGateway(feature.PodTemplateManagers, string) error {
	return nil
}
