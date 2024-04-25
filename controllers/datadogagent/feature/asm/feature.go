// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package asm

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/merger"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	err := feature.Register(feature.ASMIDType, buildASMFeature)
	if err != nil {
		panic(err)
	}
}

func buildASMFeature(options *feature.Options) feature.Feature {
	asmFeat := &asmFeature{}

	return asmFeat
}

type asmFeature struct {
	owner                    metav1.Object
	clusterAgentEnvAdditions []*corev1.EnvVar
}

// ID returns the ID of the Feature
func (f *asmFeature) ID() feature.IDType {
	return feature.ASMIDType
}

func (f *asmFeature) shouldEnableASM(asm *v2alpha1.ASMFeatureConfig) bool {
	return apiutils.BoolValue(asm.SCA.Enabled) || apiutils.BoolValue(asm.Threats.Enabled) || apiutils.BoolValue(asm.IAST.Enabled)
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *asmFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	f.owner = dda
	asm := dda.Spec.Features.ASM
	if !f.shouldEnableASM(asm) {
		return feature.RequiredComponents{}
	}

	if apiutils.BoolValue(asm.Threats.Enabled) {
		f.clusterAgentEnvAdditions = append(f.clusterAgentEnvAdditions, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAppsecEnabled,
			Value: "true",
		})
	}

	if apiutils.BoolValue(asm.IAST.Enabled) {
		f.clusterAgentEnvAdditions = append(f.clusterAgentEnvAdditions, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerIASTEnabled,
			Value: "true",
		})
	}

	if apiutils.BoolValue(asm.SCA.Enabled) {
		f.clusterAgentEnvAdditions = append(f.clusterAgentEnvAdditions, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAppsecSCAEnabled,
			Value: "true",
		})
	}

	// The cluster agent and the admission controller are required for the ASM feature.
	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{
				apicommonv1.ClusterAgentContainerName,
			},
		},
	}
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
// ASM is noy supported by v1
func (f *asmFeature) ConfigureV1(_ *v1alpha1.DatadogAgent) feature.RequiredComponents {
	return feature.RequiredComponents{}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *asmFeature) ManageDependencies(_ feature.ResourceManagers, _ feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *asmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	for _, env := range f.clusterAgentEnvAdditions {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommonv1.ClusterAgentContainerName, env, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}
	return nil
}

func (f *asmFeature) ManageSingleContainerNodeAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func (f *asmFeature) ManageNodeAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func (f *asmFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers) error {
	return nil
}
