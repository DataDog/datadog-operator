// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
)

func init() {
	if err := feature.Register(feature.AutodiscoveryIDType, buildAutodiscoveryFeature); err != nil {
		panic(err)
	}
}

func buildAutodiscoveryFeature(_ *feature.Options) feature.Feature {
	return &autodiscoveryFeature{}
}

type autodiscoveryFeature struct {
	extraIgnore []string
}

// ID returns the ID of the Feature
func (f *autodiscoveryFeature) ID() feature.IDType { return feature.AutodiscoveryIDType }

// Configure configures the feature from a DatadogAgent instance.
func (f *autodiscoveryFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	if ddaSpec.Features == nil || ddaSpec.Features.Autodiscovery == nil {
		return reqComp
	}
	if len(ddaSpec.Features.Autodiscovery.ExtraIgnoreAutoConfig) == 0 {
		return reqComp
	}

	f.extraIgnore = ddaSpec.Features.Autodiscovery.ExtraIgnoreAutoConfig

	// Mark Agent as required so single-container strategy switches to the unprivileged container.
	reqComp.Agent.IsRequired = apiutils.NewBoolPointer(true)
	reqComp.Agent.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
	reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
func (f *autodiscoveryFeature) ManageDependencies(_ feature.ResourceManagers, _ string) error {
	return nil
}

// ManageClusterAgent appends extra ignore entries to DD_IGNORE_AUTOCONF for the Cluster Agent.
func (f *autodiscoveryFeature) ManageClusterAgent(managers feature.PodTemplateManagers, _ string) error {
	if len(f.extraIgnore) == 0 {
		return nil
	}
	env := &corev1.EnvVar{Name: DDIgnoreAutoConf, Value: strings.Join(f.extraIgnore, " ")}
	return managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.ClusterAgentContainerName, env, merger.AppendToValueEnvVarMergeFunction)
}

// ManageSingleContainerNodeAgent appends extra ignore entries for single-container strategy.
func (f *autodiscoveryFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, _ string) error {
	if len(f.extraIgnore) == 0 {
		return nil
	}
	env := &corev1.EnvVar{Name: DDIgnoreAutoConf, Value: strings.Join(f.extraIgnore, " ")}
	return managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.UnprivilegedSingleAgentContainerName, env, merger.AppendToValueEnvVarMergeFunction)
}

// ManageNodeAgent appends extra ignore entries for the core agent container.
func (f *autodiscoveryFeature) ManageNodeAgent(managers feature.PodTemplateManagers, _ string) error {
	if len(f.extraIgnore) == 0 {
		return nil
	}
	env := &corev1.EnvVar{Name: DDIgnoreAutoConf, Value: strings.Join(f.extraIgnore, " ")}
	return managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.CoreAgentContainerName, env, merger.AppendToValueEnvVarMergeFunction)
}

// ManageClusterChecksRunner does nothing for this feature.
func (f *autodiscoveryFeature) ManageClusterChecksRunner(feature.PodTemplateManagers, string) error {
	return nil
}

// ManageOtelAgentGateway does nothing for this feature.
func (f *autodiscoveryFeature) ManageOtelAgentGateway(feature.PodTemplateManagers, string) error {
	return nil
}
