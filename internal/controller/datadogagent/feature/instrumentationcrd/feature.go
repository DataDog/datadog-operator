// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package instrumentationcrd

import (
	"errors"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

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
	owner                      metav1.Object
	serviceAccountName         string
	rbacSuffix                 string
	logger                     logr.Logger
	admissionControllerEnabled bool
}

// ID returns the ID of the Feature
func (f *instrumentationCRDFeature) ID() feature.IDType {
	return feature.InstrumentationCRDIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *instrumentationCRDFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	f.owner = dda

	if ddaSpec.Features.InstrumentationCRD != nil && apiutils.BoolValue(ddaSpec.Features.InstrumentationCRD.Enabled) {
		f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

		// Ensure admission controller is enabled since the instrumentation CRD controller
		// requires it for the validation webhook.
		if ddaSpec.Features.AdmissionController != nil {
			f.admissionControllerEnabled = apiutils.BoolValue(ddaSpec.Features.AdmissionController.Enabled)
		}

		return feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{
				IsRequired: ptr.To(true),
				Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
			},
		}
	}

	return feature.RequiredComponents{}
}

// ManageDependencies allows a feature to manage its dependencies.
func (f *instrumentationCRDFeature) ManageDependencies(managers feature.ResourceManagers, _ string) error {
	if !f.admissionControllerEnabled {
		return errors.New("admission controller feature must be enabled to use the instrumentation CRD feature")
	}

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
func (f *instrumentationCRDFeature) ManageClusterAgent(managers feature.PodTemplateManagers, _ string) error {
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
func (f *instrumentationCRDFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, _ string) error {
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
func (f *instrumentationCRDFeature) ManageNodeAgent(managers feature.PodTemplateManagers, _ string) error {
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
func (f *instrumentationCRDFeature) ManageClusterChecksRunner(feature.PodTemplateManagers, string) error {
	return nil
}

// ManageOtelAgentGateway allows a feature to configure the OtelAgentGateway's corev1.PodTemplateSpec
func (f *instrumentationCRDFeature) ManageOtelAgentGateway(feature.PodTemplateManagers, string) error {
	return nil
}
