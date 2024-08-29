// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissioncontroller

import (
	"encoding/json"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
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
	agentSidecarConfig     *AgentSidecarInjectionConfig
	localServiceName       string
	failurePolicy          string
	registry               string
	serviceAccountName     string
	owner                  metav1.Object

	cwsInstrumentationEnabled bool
	cwsInstrumentationMode    string
}

type AgentSidecarInjectionConfig struct {
	enabled                          bool
	clusterAgentCommunicationEnabled bool
	provider                         string
	registry                         string
	imageName                        string
	imageTag                         string
	selectors                        []*v2alpha1.Selector
	profiles                         []*v2alpha1.Profile
}

func buildAdmissionControllerFeature(options *feature.Options) feature.Feature {
	return &admissionControllerFeature{}
}

// ID returns the ID of the Feature
func (f *admissionControllerFeature) ID() feature.IDType {
	return feature.AdmissionControllerIDType
}
func shouldEnablesidecarInjection(sidecarInjectionConf *v2alpha1.AgentSidecarInjectionConfig) bool {
	if sidecarInjectionConf != nil && sidecarInjectionConf.Enabled != nil && apiutils.BoolValue(sidecarInjectionConf.Enabled) {
		return true
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
		// set image registry from feature config or global config if defined
		if ac.Registry != nil && *ac.Registry != "" {
			f.registry = *ac.Registry
		} else if dda.Spec.Global.Registry != nil && *dda.Spec.Global.Registry != "" {
			f.registry = *dda.Spec.Global.Registry
		}
		// agent communication mode set by user
		if ac.AgentCommunicationMode != nil && *ac.AgentCommunicationMode != "" {
			f.agentCommunicationMode = *ac.AgentCommunicationMode
		} else {
			// agent communication mode set automatically
			// use `socket` mode if either apm or dsd uses uds
			apm := dda.Spec.Features.APM
			dsd := dda.Spec.Features.Dogstatsd
			if (apm != nil && apm.UnixDomainSocketConfig != nil && apiutils.BoolValue(apm.Enabled) && apiutils.BoolValue(apm.UnixDomainSocketConfig.Enabled)) ||
				(dsd != nil && dsd.UnixDomainSocketConfig != nil && apiutils.BoolValue(dsd.UnixDomainSocketConfig.Enabled)) {
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

		if ac.CWSInstrumentation != nil && apiutils.BoolValue(ac.CWSInstrumentation.Enabled) {
			f.cwsInstrumentationEnabled = true
			f.cwsInstrumentationMode = apiutils.StringValue(ac.CWSInstrumentation.Mode)
		}

		sidecarConfig := dda.Spec.Features.AdmissionController.AgentSidecarInjection
		if shouldEnablesidecarInjection(sidecarConfig) {
			f.agentSidecarConfig = &AgentSidecarInjectionConfig{}
			if sidecarConfig.Enabled != nil {
				f.agentSidecarConfig.enabled = *sidecarConfig.Enabled

			}
			if sidecarConfig.Provider != nil && *sidecarConfig.Provider != "" {
				f.agentSidecarConfig.provider = *sidecarConfig.Provider
			}

			if sidecarConfig.ClusterAgentCommunicationEnabled != nil {
				f.agentSidecarConfig.clusterAgentCommunicationEnabled = *sidecarConfig.ClusterAgentCommunicationEnabled
			}
			// set image registry from admissionController config or global config if defined
			if sidecarConfig.Registry != nil && *sidecarConfig.Registry != "" {
				f.agentSidecarConfig.registry = *sidecarConfig.Registry
			} else if dda.Spec.Global.Registry != nil && *dda.Spec.Global.Registry != "" {
				f.agentSidecarConfig.registry = *dda.Spec.Global.Registry
			}

			// set agent image from admissionController config or nodeAgent override image name. else, It will follow agent image name.
			// default is "agent"
			f.agentSidecarConfig.imageName = apicommon.DefaultAgentImageName
			f.agentSidecarConfig.imageTag = defaulting.AgentLatestVersion

			componentOverride, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]
			if sidecarConfig.Image != nil && sidecarConfig.Image.Name != "" {
				f.agentSidecarConfig.imageName = sidecarConfig.Image.Name
			} else if ok && componentOverride.Image != nil {
				f.agentSidecarConfig.imageName = componentOverride.Image.Name
			}
			// set agent image tag from admissionController config or nodeAgent override image tag. else, It will follow default image tag.
			// defaults will depend on operator version.
			if sidecarConfig.Image != nil && sidecarConfig.Image.Tag != "" {
				f.agentSidecarConfig.imageTag = sidecarConfig.Image.Tag
			} else if ok && componentOverride.Image != nil {
				f.agentSidecarConfig.imageTag = componentOverride.Image.Tag
			}

			// Assemble agent sidecar selectors.
			for _, selector := range sidecarConfig.Selectors {
				newSelector := &v2alpha1.Selector{}

				if selector.NamespaceSelector != nil {
					newSelector.NamespaceSelector = &metav1.LabelSelector{
						MatchLabels:      selector.NamespaceSelector.MatchLabels,
						MatchExpressions: selector.NamespaceSelector.MatchExpressions,
					}
				}

				if selector.ObjectSelector != nil {
					newSelector.ObjectSelector = &metav1.LabelSelector{
						MatchLabels:      selector.ObjectSelector.MatchLabels,
						MatchExpressions: selector.ObjectSelector.MatchExpressions,
					}
				}

				if newSelector.NamespaceSelector != nil || newSelector.ObjectSelector != nil {
					f.agentSidecarConfig.selectors = append(f.agentSidecarConfig.selectors, newSelector)
				}
			}

			// Assemble agent sidecar profiles.
			for _, profile := range sidecarConfig.Profiles {
				if len(profile.EnvVars) > 0 || profile.ResourceRequirements != nil {
					newProfile := &v2alpha1.Profile{
						EnvVars: profile.EnvVars,
					}
					if profile.ResourceRequirements != nil {
						newProfile.ResourceRequirements = profile.ResourceRequirements
					}
					f.agentSidecarConfig.profiles = append(f.agentSidecarConfig.profiles, newProfile)
				}
			}
		}

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
	if err := managers.RBACManager().AddClusterPolicyRules(ns, rbacName, f.serviceAccountName, getRBACClusterPolicyRules(f.webhookName, f.cwsInstrumentationEnabled, f.cwsInstrumentationMode)); err != nil {
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

	if f.registry != "" {
		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerRegistryName,
			Value: f.registry,
		})
	}

	if f.serviceName != "" {
		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerServiceName,
			Value: f.serviceName,
		})
	}

	if f.cwsInstrumentationEnabled {
		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerCWSInstrumentationEnabled,
			Value: apiutils.BoolToString(&f.cwsInstrumentationEnabled),
		})

		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerCWSInstrumentationMode,
			Value: f.cwsInstrumentationMode,
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

	if f.agentSidecarConfig != nil {
		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAgentSidecarEnabled,
			Value: apiutils.BoolToString(&f.agentSidecarConfig.enabled),
		})

		managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAgentSidecarClusterAgentEnabled,
			Value: apiutils.BoolToString(&f.agentSidecarConfig.clusterAgentCommunicationEnabled),
		})
		if f.agentSidecarConfig.provider != "" {
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarProvider,
				Value: f.agentSidecarConfig.provider,
			})
		}
		if f.agentSidecarConfig.registry != "" {
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarRegistry,
				Value: f.agentSidecarConfig.registry,
			})
		}

		if f.agentSidecarConfig.imageName != "" {
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarImageName,
				Value: f.agentSidecarConfig.imageName,
			})
		}
		if f.agentSidecarConfig.imageTag != "" {
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarImageTag,
				Value: f.agentSidecarConfig.imageTag,
			})
		}

		if len(f.agentSidecarConfig.selectors) > 0 {
			selectorsJSON, err := json.Marshal(f.agentSidecarConfig.selectors)
			if err != nil {
				return err
			}
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarSelectors,
				Value: string(selectorsJSON),
			})
		}

		if len(f.agentSidecarConfig.profiles) > 0 {
			profilesJSON, err := json.Marshal(f.agentSidecarConfig.profiles)
			if err != nil {
				return err
			}
			managers.EnvVar().AddEnvVarToContainer(common.ClusterAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDAdmissionControllerAgentSidecarProfiles,
				Value: string(profilesJSON),
			})
		}

	}

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
