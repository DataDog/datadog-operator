// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/merger"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	err := feature.Register(feature.ProfilingIDType, buildProfilingFeature)
	if err != nil {
		panic(err)
	}
}

func buildProfilingFeature(options *feature.Options) feature.Feature {
	profilingFeat := &profilingFeature{}

	return profilingFeat
}

type profilingFeature struct {
	owner   metav1.Object
	enabled string
}

// ID returns the ID of the Feature
func (f *profilingFeature) ID() feature.IDType {
	return feature.ProfilingIDType
}

func (f *profilingFeature) shouldEnableProfiling(dda *v2alpha1.DatadogAgent) bool {
	Profiling := dda.Spec.Features.Profiling
	if dda.Spec.Features.AdmissionController == nil || !apiutils.BoolValue(dda.Spec.Features.AdmissionController.Enabled) {
		return false
	}

	return apiutils.StringValue(Profiling.Enabled) != ""
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *profilingFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	f.owner = dda
	Profiling := dda.Spec.Features.Profiling
	if !f.shouldEnableProfiling(dda) {
		return feature.RequiredComponents{}
	}

	f.enabled = apiutils.StringValue(Profiling.Enabled)

	// The cluster agent and the admission controller are required for the Profiling feature.
	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{
				apicommonv1.ClusterAgentContainerName,
			},
		},
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *profilingFeature) ManageDependencies(_ feature.ResourceManagers, _ feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *profilingFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	if f.enabled != "" {
		if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerProfilingEnabled,
			Value: f.enabled,
		}, merger.IgnoreNewEnvVarMergeFunction); err != nil {
			return err
		}
	}

	return nil
}

func (f *profilingFeature) ManageSingleContainerNodeAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func (f *profilingFeature) ManageNodeAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

func (f *profilingFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers) error {
	return nil
}
