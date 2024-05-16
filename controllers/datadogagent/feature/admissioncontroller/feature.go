// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissioncontroller

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	componentdca "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/defaulting"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func init() {
	err := feature.Register(feature.AdmissionControllerIDType, buildAdmissionControllerFeature)
	if err != nil {
		panic(err)
	}
}

type admissionControllerFeature struct {
	mutateUnlabelled       bool
	serviceName            string
	webhookName            string
	agentCommunicationMode string
	agentSidecarInjection  *agentSidecarInjectionConfig
	localServiceName       string
	failurePolicy          string

	serviceAccountName string
	owner              metav1.Object
}

type agentSidecarInjectionConfig struct {
	enabled                          bool
	clusterAgentCommunicationEnabled bool
	provider                         string
	registry                         string
	imageName                        string
	imageTag                         string
}

func buildAdmissionControllerFeature(options *feature.Options) feature.Feature {
	return &admissionControllerFeature{}
}

// ID returns the ID of the Feature
func (f *admissionControllerFeature) ID() feature.IDType {
	return feature.AdmissionControllerIDType
}
func shouldEnableSidecarInjection(sidecarInjectionConf *v2alpha1.AgentSidecarInjectionFeatureConfig) bool {
	if sidecarInjectionConf == nil {
		return false
	}

	if sidecarInjectionConf.Enabled != nil {
		return apiutils.BoolValue(sidecarInjectionConf.Enabled)
	}

	return false
}

func (f *admissionControllerFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	f.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)

	ac := dda.Spec.Features.AdmissionController

	if ac != nil && apiutils.BoolValue(ac.Enabled) {
		f.mutateUnlabelled = apiutils.BoolValue(ac.MutateUnlabelled)
		if ac.ServiceName != nil && *ac.ServiceName != "" {
			f.serviceName = *ac.ServiceName
		}
		// agent communication mode set by user
		if ac.AgentCommunicationMode != nil && *ac.AgentCommunicationMode != "" {
			f.agentCommunicationMode = *ac.AgentCommunicationMode
		} else {
			// agent communication mode set automatically
			// use `socket` mode if either apm or dsd uses uds
			apm := dda.Spec.Features.APM
			dsd := dda.Spec.Features.Dogstatsd
			if apm != nil && apiutils.BoolValue(apm.Enabled) && apiutils.BoolValue(apm.UnixDomainSocketConfig.Enabled) ||
				dsd.UnixDomainSocketConfig != nil && apiutils.BoolValue(dsd.UnixDomainSocketConfig.Enabled) {
				f.agentCommunicationMode = apicommon.AdmissionControllerSocketCommunicationMode
			}
			// otherwise don't set to fall back to default agent setting `hostip`
		}
		f.localServiceName = v2alpha1.GetLocalAgentServiceName(dda)
		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
		}
		if ac.FailurePolicy != nil && *ac.FailurePolicy != "" {
			f.failurePolicy = *ac.FailurePolicy
		}

		f.webhookName = apicommon.DefaultAdmissionControllerWebhookName
		if ac.WebhookName != nil {
			f.webhookName = *ac.WebhookName
		}

		if shouldEnableSidecarInjection(ac.AgentSidecarInjection) {
			f.agentSidecarInjection = &agentSidecarInjectionConfig{}
			f.agentSidecarInjection.enabled = *ac.AgentSidecarInjection.Enabled
			if ac.AgentSidecarInjection.Provider != nil && *ac.AgentSidecarInjection.Provider != "" {
				f.agentSidecarInjection.provider = *ac.AgentSidecarInjection.Provider
			}

			if ac.AgentSidecarInjection.ClusterAgentCommunicationEnabled != nil {
				f.agentSidecarInjection.clusterAgentCommunicationEnabled = *ac.AgentSidecarInjection.ClusterAgentCommunicationEnabled
			} else {
				f.agentSidecarInjection.clusterAgentCommunicationEnabled = apicommon.DefaultAdmissionControllerAgentSidecarClusterAgentEnabled
			}

			// set image registry from admissionController config or global config if defined
			if ac.AgentSidecarInjection.Registry != nil && *ac.AgentSidecarInjection.Registry != "" {
				f.agentSidecarInjection.registry = *ac.AgentSidecarInjection.Registry
			}
			// set agent image from admissionController config or else, It will follow agent image name.
			// default is "agent"
			if ac.AgentSidecarInjection.ImageName != nil && *ac.AgentSidecarInjection.ImageName != "" {
				f.agentSidecarInjection.imageName = *ac.AgentSidecarInjection.ImageName
			} else {
				f.agentSidecarInjection.imageName = apicommon.DefaultAgentImageName
			}
			// set agent image tag from admissionController config or else, It will follow default image tag.
			// defaults will depend on operation version.
			if ac.AgentSidecarInjection.ImageTag != nil && *ac.AgentSidecarInjection.ImageTag != "" {
				f.agentSidecarInjection.imageTag = *ac.AgentSidecarInjection.ImageTag
			} else {
				f.agentSidecarInjection.imageTag = defaulting.AgentLatestVersion
			}

		}

	}
	return reqComp
}

func (f *admissionControllerFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	f.serviceAccountName = v1alpha1.GetClusterAgentServiceAccount(dda)

	if dda.Spec.ClusterAgent.Config != nil && dda.Spec.ClusterAgent.Config.AdmissionController != nil && apiutils.BoolValue(dda.Spec.ClusterAgent.Config.AdmissionController.Enabled) {
		ac := dda.Spec.ClusterAgent.Config.AdmissionController
		f.mutateUnlabelled = apiutils.BoolValue(ac.MutateUnlabelled)
		f.serviceName = *ac.ServiceName
		if ac.AgentCommunicationMode != nil && *ac.AgentCommunicationMode != "" {
			f.agentCommunicationMode = *ac.AgentCommunicationMode
		}
		f.localServiceName = v1alpha1.GetLocalAgentServiceName(dda)
		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
		}
		f.webhookName = apicommon.DefaultAdmissionControllerWebhookName
	}
	return reqComp
}

func (f *admissionControllerFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	ns := f.owner.GetNamespace()
	rbacName := componentdca.GetClusterAgentRbacResourcesName(f.owner)

	// service
	selector := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:      f.owner.GetName(),
		apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
	}
	port := []corev1.ServicePort{
		{
			Name:       apicommon.AdmissionControllerPortName,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(apicommon.DefaultAdmissionControllerTargetPort),
			Port:       apicommon.DefaultAdmissionControllerServicePort,
		},
	}
	if err := managers.ServiceManager().AddService(f.serviceName, ns, selector, port, nil); err != nil {
		return err
	}

	// rbac
	if err := managers.RBACManager().AddClusterPolicyRules(ns, rbacName, f.serviceAccountName, getRBACClusterPolicyRules(f.webhookName)); err != nil {
		return err
	}
	return managers.RBACManager().AddPolicyRules(ns, rbacName, f.serviceAccountName, getRBACPolicyRules())
}

func (f *admissionControllerFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDAdmissionControllerEnabled,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDAdmissionControllerMutateUnlabelled,
		Value: apiutils.BoolToString(&f.mutateUnlabelled),
	})

	if f.agentSidecarInjection != nil {
		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAgentSidecarEnabled,
			Value: apiutils.BoolToString(&f.agentSidecarInjection.enabled),
		})

		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAgentSidecarClusterAgentEnabled,
			Value: apiutils.BoolToString(&f.agentSidecarInjection.clusterAgentCommunicationEnabled),
		})
		if f.agentSidecarInjection.provider != "" {
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarProvider,
				Value: f.agentSidecarInjection.provider,
			})
		}
		if f.agentSidecarInjection.registry != "" {
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarRegistry,
				Value: f.agentSidecarInjection.registry,
			})
		}

		if f.agentSidecarInjection.imageName != "" {
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarImageName,
				Value: f.agentSidecarInjection.imageName,
			})
		}
		if f.agentSidecarInjection.imageTag != "" {
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarImageTag,
				Value: f.agentSidecarInjection.imageTag,
			})
		}

	}

	if f.serviceName != "" {
		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerServiceName,
			Value: f.serviceName,
		})
	}

	if f.agentCommunicationMode != "" {
		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerInjectConfigMode,
			Value: f.agentCommunicationMode,
		})
	}

	managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDAdmissionControllerLocalServiceName,
		Value: f.localServiceName,
	})

	if f.failurePolicy != "" {
		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerFailurePolicy,
			Value: f.failurePolicy,
		})
	}

	managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDAdmissionControllerWebhookName,
		Value: f.webhookName,
	})

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set..
// It should do nothing if the feature doesn't need to configure it.
func (f *admissionControllerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (f *admissionControllerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (f *admissionControllerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
