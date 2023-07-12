// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/go-logr/logr"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
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

	enabled                 bool
	containerImageEnabled   bool
	containerImageAnalyzers []string
	hostEnabled             bool
	hostAnalyzers           []string
}

// ID returns the ID of the Feature
func (f *sbomFeature) ID() feature.IDType {
	return feature.SBOMIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *sbomFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if dda.Spec.Features != nil && dda.Spec.Features.SBOM != nil && apiutils.BoolValue(dda.Spec.Features.SBOM.Enabled) {
		f.enabled = true
		if dda.Spec.Features.SBOM.ContainerImage != nil && apiutils.BoolValue(dda.Spec.Features.SBOM.ContainerImage.Enabled) {
			f.containerImageEnabled = true
			f.containerImageAnalyzers = dda.Spec.Features.SBOM.ContainerImage.Analyzers
		}
		if dda.Spec.Features.SBOM.Host != nil && apiutils.BoolValue(dda.Spec.Features.SBOM.Host.Enabled) {
			f.hostEnabled = true
			f.hostAnalyzers = dda.Spec.Features.SBOM.Host.Analyzers
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

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *sbomFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	return
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

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *sbomFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDSBOMEnabled,
		Value: apiutils.BoolToString(&f.enabled),
	})
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDSBOMContainerImageEnabled,
		Value: apiutils.BoolToString(&f.containerImageEnabled),
	})
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDSBOMContainerImageAnalyzers,
		Value: strings.Join(f.containerImageAnalyzers, " "),
	})
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDSBOMHostEnabled,
		Value: apiutils.BoolToString(&f.hostEnabled),
	})
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDSBOMHostAnalyzers,
		Value: strings.Join(f.hostAnalyzers, " "),
	})

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *sbomFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
