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
	localServiceName       string
	failurePolicy          string

	serviceAccountName string
	owner              metav1.Object
}

func buildAdmissionControllerFeature(options *feature.Options) feature.Feature {
	return &admissionControllerFeature{}
}

// ID returns the ID of the Feature
func (f *admissionControllerFeature) ID() feature.IDType {
	return feature.AdmissionControllerIDType
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

func (f *admissionControllerFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (f *admissionControllerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
