// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package instrumentationcrd

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

// minVersion is the minimum agent version that supports the instrumentation CRD controller.
const minVersion = "7.82.0-0"

func init() {
	err := feature.Register(feature.InstrumentationCRDIDType, buildInstrumentationCRDFeature)
	if err != nil {
		panic(err)
	}
}

func buildInstrumentationCRDFeature(options *feature.Options) feature.Feature {
	f := &instrumentationCRDFeature{
		rbacSuffix: common.ClusterAgentSuffix,
	}
	if options != nil {
		f.logger = options.Logger
	}
	return f
}

type instrumentationCRDFeature struct {
	owner              metav1.Object
	serviceAccountName string
	rbacSuffix         string
	logger             logr.Logger
}

// ID returns the ID of the Feature
func (f *instrumentationCRDFeature) ID() feature.IDType {
	return feature.InstrumentationCRDIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *instrumentationCRDFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	f.owner = dda

	// If the cluster agent version is explicitly set and below the minimum, skip enabling.
	if clusterAgent, ok := ddaSpec.Override[v2alpha1.ClusterAgentComponentName]; ok && clusterAgent.Image != nil {
		version := common.GetAgentVersionFromImage(*clusterAgent.Image)
		if !utils.IsAboveMinVersion(version, minVersion, nil) {
			return feature.RequiredComponents{}
		}
	} else if !utils.IsAboveMinVersion(images.ClusterAgentLatestVersion, minVersion, nil) {
		return feature.RequiredComponents{}
	}

	// If the node agent version is explicitly set and below the minimum, skip enabling.
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok && nodeAgent.Image != nil {
		version := common.GetAgentVersionFromImage(*nodeAgent.Image)
		if !utils.IsAboveMinVersion(version, minVersion, nil) {
			return feature.RequiredComponents{}
		}
	} else if !utils.IsAboveMinVersion(images.AgentLatestVersion, minVersion, nil) {
		return feature.RequiredComponents{}
	}

	if !featureutils.HasFeatureEnableAnnotation(dda, featureutils.EnableInstrumentationCRDAnnotation) {
		return feature.RequiredComponents{}
	}

	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: new(true),
			Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
		},
		Agent: feature.RequiredComponent{
			IsRequired: new(true),
			Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
		},
	}
}

// ManageDependencies allows a feature to manage its dependencies.
func (f *instrumentationCRDFeature) ManageDependencies(managers feature.ResourceManagers) error {
	rbacName := GetInstrumentationCRDRBACResourceName(f.owner, f.rbacSuffix)

	return managers.RBACManager().AddClusterPolicyRulesByComponent(
		f.owner.GetNamespace(),
		rbacName,
		f.serviceAccountName,
		instrumentationCRDRBACPolicyRules,
		string(v2alpha1.ClusterAgentComponentName),
	)
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *instrumentationCRDFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(
		apicommon.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  DDInstrumentationCRDControllerEnabled,
			Value: "true",
		},
	)
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
func (f *instrumentationCRDFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(
		apicommon.UnprivilegedSingleAgentContainerName,
		&corev1.EnvVar{
			Name:  DDInstrumentationCRDControllerEnabled,
			Value: "true",
		},
	)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
func (f *instrumentationCRDFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(
		apicommon.CoreAgentContainerName,
		&corev1.EnvVar{
			Name:  DDInstrumentationCRDControllerEnabled,
			Value: "true",
		},
	)
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
func (f *instrumentationCRDFeature) ManageClusterChecksRunner(feature.PodTemplateManagers) error {
	return nil
}

// ManageOtelAgentGateway allows a feature to configure the OtelAgentGateway's corev1.PodTemplateSpec
func (f *instrumentationCRDFeature) ManageOtelAgentGateway(feature.PodTemplateManagers) error {
	return nil
}
