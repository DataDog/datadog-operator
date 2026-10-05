// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.DefaultIDType, buildDefaultFeature)
	if err != nil {
		panic(err)
	}
}

func buildDefaultFeature(options *feature.Options) feature.Feature {
	dF := &defaultFeature{}

	if options != nil {
		dF.logger = options.Logger
	}

	return dF
}

type defaultFeature struct {
	owner                metav1.Object
	clusterAgentDisabled bool
	logger               logr.Logger
}

// ID returns the ID of the Feature
func (f *defaultFeature) ID() feature.IDType {
	return feature.DefaultIDType
}

func (f *defaultFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	trueValue := true
	f.owner = dda

	if clusterAgent, ok := ddaSpec.Override[v2alpha1.ClusterAgentComponentName]; ok {
		f.clusterAgentDisabled = apiutils.BoolValue(clusterAgent.Disabled)
	}

	// IsRequired: false (rather than nil) tells the merge that the Cluster Agent is
	// explicitly disabled via override, so checkComponentEnabledWithOverride resolves
	// this as a clean disable instead of an override/feature conflict, as long as no
	// other feature still requires it.
	clusterAgentRequired := feature.RequiredComponent{
		IsRequired: &trueValue,
		Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
	}
	if f.clusterAgentDisabled {
		clusterAgentRequired = feature.RequiredComponent{IsRequired: new(false)}
	}

	return feature.RequiredComponents{
		ClusterAgent: clusterAgentRequired,
		Agent: feature.RequiredComponent{
			IsRequired: &trueValue,
			Containers: []apicommon.AgentContainerName{},
		},
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *defaultFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	return f.overrideClusterAgentEnabledEnvVar(managers)
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	return f.overrideClusterAgentEnabledEnvVar(managers)
}

// overrideClusterAgentEnabledEnvVar corrects DD_CLUSTER_AGENT_ENABLED, which the default pod
// template scaffolding (component/agent/default.go) hardcodes to "true" on every agent
// container regardless of whether a Cluster Agent is actually being deployed.
func (f *defaultFeature) overrideClusterAgentEnabledEnvVar(managers feature.PodTemplateManagers) error {
	if !f.clusterAgentDisabled {
		return nil
	}
	envVar := &corev1.EnvVar{
		Name:  common.DDClusterAgentEnabled,
		Value: "false",
	}
	managers.EnvVar().AddEnvVar(envVar)
	// AddEnvVar only reaches regular containers; the init-config init container builds its
	// env vars from the same hardcoded-true default and needs the override too.
	managers.EnvVar().AddEnvVarToInitContainer(apicommon.InitConfigContainerName, envVar)
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}

func (f *defaultFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers) error {
	return nil
}
