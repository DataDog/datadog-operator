// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesactions

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

func init() {
	err := feature.Register(feature.KubernetesActionsIDType, buildKubernetesActionsFeature)
	if err != nil {
		panic(err)
	}
}

func buildKubernetesActionsFeature(options *feature.Options) feature.Feature {
	f := &kubernetesActionsFeature{
		rbacSuffix: common.ClusterAgentSuffix,
	}
	if options != nil {
		f.logger = options.Logger
	}
	return f
}

type kubernetesActionsFeature struct {
	owner              metav1.Object
	serviceAccountName string
	rbacSuffix         string
	rcEnabled          bool
	parEnabled         bool
	// channelToggles is true when the Cluster Agent understands the per-channel
	// settings (DD_KUBEACTIONS_REMOTE_CONFIG_ENABLED / DD_KUBEACTIONS_PRIVATE_ACTION_RUNNER_ENABLED).
	channelToggles bool
	logger         logr.Logger
}

func (f *kubernetesActionsFeature) ID() feature.IDType {
	return feature.KubernetesActionsIDType
}

func (f *kubernetesActionsFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if ddaSpec.Features == nil || ddaSpec.Features.KubernetesActions == nil || !apiutils.BoolValue(ddaSpec.Features.KubernetesActions.Enabled) {
		return reqComp
	}

	f.rcEnabled = featureutils.IsKubernetesActionsRCEnabled(ddaSpec)
	parRequested := featureutils.IsKubernetesActionsPARRequested(ddaSpec)
	if !f.rcEnabled && !parRequested {
		f.logger.Error(nil, "Kubernetes Actions is enabled but both communication.rc and communication.par are false; the feature will not be configured")
		return reqComp
	}

	// The private action runner feature enables the runner under the same condition.
	f.parEnabled = featureutils.IsKubernetesActionsPAREnabled(ddaSpec)
	if parRequested && !f.parEnabled {
		f.logger.Error(nil, "Kubernetes Actions communication.par=true requires a newer Cluster Agent; the Private Action Runner is not enabled", "min", featureutils.KubernetesActionsPARMinVersion)
	}

	dcaVersion := featureutils.ClusterAgentVersion(ddaSpec)
	if !utils.IsAboveMinVersion(dcaVersion, ClusterAgentMinVersion, nil) {
		f.logger.V(1).Info("cluster agent version is too low for Kubernetes Actions", "min", ClusterAgentMinVersion)
		return reqComp
	}

	f.channelToggles = utils.IsAboveMinVersion(dcaVersion, ClusterAgentChannelTogglesMinVersion, nil)
	if !f.rcEnabled && !f.channelToggles {
		// Older Cluster Agents always use Remote Configuration; ignore the setting rather than
		// passing an environment variable they do not understand.
		f.logger.Error(nil, "Kubernetes Actions communication.rc=false requires a newer Cluster Agent; Remote Configuration stays enabled", "min", ClusterAgentChannelTogglesMinVersion)
		f.rcEnabled = true
	}

	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

	reqComp = feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: new(true),
			Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
		},
	}
	return reqComp
}

func (f *kubernetesActionsFeature) ManageDependencies(managers feature.ResourceManagers) error {
	rbacName := getRBACResourceName(f.owner, f.rbacSuffix)
	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, kubernetesActionsRBACPolicyRules)
}

func (f *kubernetesActionsFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDKubeActionsEnabled,
		Value: "true",
	})
	// Only pass a channel setting when turning that channel off, and only to a Cluster Agent
	// that understands it: both settings default to true in the agent.
	if f.channelToggles && !f.rcEnabled {
		managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDKubeActionsRemoteConfigEnabled,
			Value: "false",
		})
	}
	if f.channelToggles && !f.parEnabled {
		managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDKubeActionsPAREnabled,
			Value: "false",
		})
	}
	return nil
}

func (f *kubernetesActionsFeature) ManageSingleContainerNodeAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *kubernetesActionsFeature) ManageNodeAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *kubernetesActionsFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *kubernetesActionsFeature) ManageOtelAgentGateway(_ feature.PodTemplateManagers) error {
	return nil
}
