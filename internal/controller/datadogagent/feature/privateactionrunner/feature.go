// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
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
	owner                metav1.Object
	logger               logr.Logger
	nodeSelfEnroll       *bool
	nodeActionsAllowlist []string
	nodeEnabled          bool
}

// ID returns the ID of the Feature
func (f *privateActionRunnerFeature) ID() feature.IDType {
	return feature.PrivateActionRunnerIDType
}

// Configure configures the feature from a v2alpha1.DatadogAgent instance.
func (f *privateActionRunnerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	// Check if feature is enabled
	if ddaSpec.Features == nil ||
		ddaSpec.Features.PrivateActionRunner == nil ||
		!apiutils.BoolValue(ddaSpec.Features.PrivateActionRunner.Enabled) {
		return feature.RequiredComponents{}
	}

	parConfig := ddaSpec.Features.PrivateActionRunner

	// Determine node deployment
	// Default: enabled if parent enabled and not explicitly disabled
	f.nodeEnabled = parConfig.NodeAgent == nil || parConfig.NodeAgent.Enabled == nil || apiutils.BoolValue(parConfig.NodeAgent.Enabled)

	if f.nodeEnabled && parConfig.NodeAgent != nil {
		f.nodeSelfEnroll = parConfig.NodeAgent.SelfEnroll
		f.nodeActionsAllowlist = parConfig.NodeAgent.ActionsAllowlist
	}

	if f.nodeEnabled {
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.CoreAgentContainerName,
				apicommon.PrivateActionRunnerContainerName,
			},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
func (f *privateActionRunnerFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	// No external dependencies needed for now
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	// Cluster Agent support not yet implemented
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	if !f.nodeEnabled {
		return nil
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.PrivateActionRunnerContainerName, &corev1.EnvVar{
		Name:  "DD_PRIVATEACTIONRUNNER_ENABLED",
		Value: "true",
	})

	if f.nodeSelfEnroll != nil {
		managers.EnvVar().AddEnvVarToContainer(apicommon.PrivateActionRunnerContainerName, &corev1.EnvVar{
			Name:  "DD_PRIVATEACTIONRUNNER_SELF_ENROLL",
			Value: apiutils.BoolToString(f.nodeSelfEnroll),
		})
	}

	if len(f.nodeActionsAllowlist) > 0 {
		managers.EnvVar().AddEnvVarToContainer(apicommon.PrivateActionRunnerContainerName, &corev1.EnvVar{
			Name:  "DD_PRIVATEACTIONRUNNER_ACTIONS_ALLOWLIST",
			Value: strings.Join(f.nodeActionsAllowlist, ","),
		})
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.PrivateActionRunnerContainerName, &corev1.EnvVar{
		Name: constants.DDHostName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: common.FieldPathSpecNodeName,
			},
		},
	})

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Private Action Runner requires separate container, not compatible with single-container mode
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	// Private Action Runner doesn't run in cluster checks runner
	return nil
}

// ManageOtelAgentGateway allows a feature to configure the OtelAgentGateway's corev1.PodTemplateSpec
func (f *privateActionRunnerFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	// Private Action Runner doesn't run in OTel Agent Gateway
	return nil
}
