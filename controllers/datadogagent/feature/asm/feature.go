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
	owner          metav1.Object
	threatsEnabled bool
	iastEnabled    bool
	scaEnabled     bool
}

// ID returns the ID of the Feature
func (f *asmFeature) ID() feature.IDType {
	return feature.ASMIDType
}

func (f *asmFeature) shouldEnableASM(dda *v2alpha1.DatadogAgent) bool {
	asm := dda.Spec.Features.ASM
	if dda.Spec.Features.AdmissionController == nil || !apiutils.BoolValue(dda.Spec.Features.AdmissionController.Enabled) {
		return false
	}

	return apiutils.BoolValue(asm.SCA.Enabled) || apiutils.BoolValue(asm.Threats.Enabled) || apiutils.BoolValue(asm.IAST.Enabled)
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *asmFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	f.owner = dda
	asm := dda.Spec.Features.ASM
	if !f.shouldEnableASM(dda) {
		return feature.RequiredComponents{}
	}

	f.threatsEnabled = apiutils.BoolValue(asm.Threats.Enabled)
	f.iastEnabled = apiutils.BoolValue(asm.IAST.Enabled)
	f.scaEnabled = apiutils.BoolValue(asm.SCA.Enabled)

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
// ASM is not supported by v1
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
	if f.threatsEnabled {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAppsecEnabled,
			Value: "true",
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	if f.iastEnabled {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerIASTEnabled,
			Value: "true",
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	if f.scaEnabled {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAppsecSCAEnabled,
			Value: "true",
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
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
