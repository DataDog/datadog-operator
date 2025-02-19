// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package asm

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
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
	if dda.Spec.Features.AdmissionController == nil || !apiutils.NewDeref(dda.Spec.Features.AdmissionController.Enabled, false) {
		return false
	}

	return apiutils.NewDeref(asm.SCA.Enabled, false) || apiutils.NewDeref(asm.Threats.Enabled, false) || apiutils.NewDeref(asm.IAST.Enabled, false)
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *asmFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	f.owner = dda
	asm := dda.Spec.Features.ASM
	if !f.shouldEnableASM(dda) {
		return feature.RequiredComponents{}
	}

	f.threatsEnabled = apiutils.NewDeref(asm.Threats.Enabled, false)
	f.iastEnabled = apiutils.NewDeref(asm.IAST.Enabled, false)
	f.scaEnabled = apiutils.NewDeref(asm.SCA.Enabled, false)

	// The cluster agent and the admission controller are required for the ASM feature.
	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: apiutils.NewPointer(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.ClusterAgentContainerName,
			},
		},
	}
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
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDAdmissionControllerAppsecEnabled,
			Value: "true",
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	if f.iastEnabled {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDAdmissionControllerIASTEnabled,
			Value: "true",
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	if f.scaEnabled {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDAdmissionControllerAppsecSCAEnabled,
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
