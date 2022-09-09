// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	corev1 "k8s.io/api/core/v1"
)

func init() {
	err := feature.Register(feature.ClusterChecksIDType, buildClusterChecksFeature)
	if err != nil {
		panic(err)
	}
}

type clusterChecksFeature struct {
	useClusterCheckRunners bool
}

func buildClusterChecksFeature(options *feature.Options) feature.Feature {
	return &clusterChecksFeature{}
}

// ID returns the ID of the Feature
func (f *clusterChecksFeature) ID() feature.IDType {
	return feature.ClusterChecksIDType
}

func (f *clusterChecksFeature) Configure(dda *v2alpha1.DatadogAgent, newStatus *v2alpha1.DatadogAgentStatus) feature.RequiredComponents {
	clusterChecksEnabled := apiutils.BoolValue(dda.Spec.Features.ClusterChecks.Enabled)
	f.useClusterCheckRunners = clusterChecksEnabled && apiutils.BoolValue(dda.Spec.Features.ClusterChecks.UseClusterChecksRunners)

	if clusterChecksEnabled {
		return feature.RequiredComponents{
			ClusterAgent:        feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			ClusterChecksRunner: feature.RequiredComponent{IsRequired: &f.useClusterCheckRunners},
		}
	}

	// Don't set ClusterAgent here because we can have a DCA deployed (as
	// defined in the "default" feature) with cluster checks disabled.
	return feature.RequiredComponents{
		ClusterChecksRunner: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(false)},
	}
}

func (f *clusterChecksFeature) ConfigureV1(dda *datadoghqv1alpha1.DatadogAgent) feature.RequiredComponents {
	clusterChecksEnabled := false

	if dda != nil && dda.Spec.ClusterAgent.Config != nil {
		clusterChecksEnabled = apiutils.BoolValue(dda.Spec.ClusterAgent.Config.ClusterChecksEnabled)
		f.useClusterCheckRunners = clusterChecksEnabled && apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled)
	}

	if clusterChecksEnabled {
		return feature.RequiredComponents{
			ClusterAgent:        feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			ClusterChecksRunner: feature.RequiredComponent{IsRequired: &f.useClusterCheckRunners},
		}
	}

	return feature.RequiredComponents{
		ClusterChecksRunner: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(false)},
	}
}

func (f *clusterChecksFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

func (f *clusterChecksFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(
		common.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  apicommon.DDClusterChecksEnabled,
			Value: "true",
		},
	)

	managers.EnvVar().AddEnvVarToContainer(
		common.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  apicommon.DDExtraConfigProviders,
			Value: apicommon.KubeServicesAndEndpointsConfigProviders,
		},
	)

	managers.EnvVar().AddEnvVarToContainer(
		common.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  apicommon.DDExtraListeners,
			Value: apicommon.KubeServicesAndEndpointsListeners,
		},
	)

	return nil
}

func (f *clusterChecksFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	if f.useClusterCheckRunners {
		managers.EnvVar().AddEnvVarToContainer(
			common.CoreAgentContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDExtraConfigProviders,
				Value: apicommon.EndpointsChecksConfigProvider,
			},
		)
	} else {
		managers.EnvVar().AddEnvVarToContainer(
			common.CoreAgentContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDExtraConfigProviders,
				Value: apicommon.ClusterAndEndpointsConfigProviders,
			},
		)
	}

	return nil
}

func (f *clusterChecksFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	if f.useClusterCheckRunners {
		managers.EnvVar().AddEnvVarToContainer(
			common.ClusterChecksRunnersContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDClusterChecksEnabled,
				Value: "true",
			},
		)

		managers.EnvVar().AddEnvVarToContainer(
			common.ClusterChecksRunnersContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDExtraConfigProviders,
				Value: apicommon.ClusterChecksConfigProvider,
			},
		)
	}

	return nil
}
