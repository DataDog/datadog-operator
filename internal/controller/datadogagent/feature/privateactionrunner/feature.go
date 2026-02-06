// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package privateactionrunner

import (
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.PrivateActionRunnerIDType, buildPrivateActionRunnerFeature)
	if err != nil {
		panic(err)
	}
}

func buildPrivateActionRunnerFeature(options *feature.Options) feature.Feature {
	parFeat := &privateActionRunnerFeature{}

	if options != nil {
		parFeat.logger = options.Logger
	}

	return parFeat
}

type privateActionRunnerFeature struct {
	enabled          bool
	selfEnroll       bool
	actionsAllowlist []string

	owner  metav1.Object
	logger logr.Logger
}

// ID returns the ID of the Feature
func (f *privateActionRunnerFeature) ID() feature.IDType {
	return feature.PrivateActionRunnerIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *privateActionRunnerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if ddaSpec.Features == nil || ddaSpec.Features.PrivateActionRunner == nil {
		return feature.RequiredComponents{}
	}

	par := ddaSpec.Features.PrivateActionRunner
	if !apiutils.BoolValue(par.Enabled) {
		return feature.RequiredComponents{}
	}

	f.enabled = true
	f.selfEnroll = apiutils.BoolValue(par.SelfEnroll)
	if par.ActionsAllowlist != nil {
		f.actionsAllowlist = par.ActionsAllowlist
	}

	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
		},
	}
}

// ManageDependencies allows a feature to manage its dependencies.
func (f *privateActionRunnerFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	// Enable Private Action Runner
	managers.EnvVar().AddEnvVarToContainer(
		apicommon.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  "DD_PRIVATE_ACTION_RUNNER_ENABLED",
			Value: "true",
		},
	)

	// Configure self-enrollment
	managers.EnvVar().AddEnvVarToContainer(
		apicommon.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  "DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL",
			Value: apiutils.BoolToString(&f.selfEnroll),
		},
	)

	// Configure actions allowlist
	if len(f.actionsAllowlist) > 0 {
		allowlist := strings.Join(f.actionsAllowlist, ",")
		managers.EnvVar().AddEnvVarToContainer(
			apicommon.ClusterAgentContainerName,
			&corev1.EnvVar{
				Name:  "DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST",
				Value: allowlist,
			},
		)
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageOtelAgentGateway allows a feature to configure the OTel Agent Gateway's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
