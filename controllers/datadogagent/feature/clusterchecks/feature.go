// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func init() {
	err := feature.Register(feature.ClusterChecksIDType, buildClusterChecksFeature)
	if err != nil {
		panic(err)
	}
}

type clusterChecksFeature struct {
	useClusterCheckRunners bool
	owner                  metav1.Object

	createKubernetesNetworkPolicy bool
	createCiliumNetworkPolicy     bool

	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	logger logr.Logger
}

func buildClusterChecksFeature(options *feature.Options) feature.Feature {
	feature := &clusterChecksFeature{}
	if options != nil {
		feature.logger = options.Logger
	}
	return feature
}

// ID returns the ID of the Feature
func (f *clusterChecksFeature) ID() feature.IDType {
	return feature.ClusterChecksIDType
}

func (f *clusterChecksFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if apiutils.BoolValue(dda.Spec.Features.ClusterChecks.Enabled) {
		f.updateConfigHash(dda)
		f.owner = dda

		if enabled, flavor := v2alpha1.IsNetworkPolicyEnabled(dda); enabled {
			if flavor == v2alpha1.NetworkPolicyFlavorCilium {
				f.createCiliumNetworkPolicy = true
			} else {
				f.createKubernetesNetworkPolicy = true
			}
		}

		f.useClusterCheckRunners = apiutils.BoolValue(dda.Spec.Features.ClusterChecks.UseClusterChecksRunners)
		reqComp = feature.RequiredComponents{
			ClusterAgent:        feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			ClusterChecksRunner: feature.RequiredComponent{IsRequired: &f.useClusterCheckRunners},
		}
	}

	return reqComp
}

func (f *clusterChecksFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) feature.RequiredComponents {
	clusterChecksEnabled := false

	if dda != nil && dda.Spec.ClusterAgent.Config != nil {
		clusterChecksEnabled = apiutils.BoolValue(dda.Spec.ClusterAgent.Config.ClusterChecksEnabled)
		f.useClusterCheckRunners = clusterChecksEnabled && apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled)
	}

	if clusterChecksEnabled {
		f.owner = dda

		if enabled, flavor := v1alpha1.IsAgentNetworkPolicyEnabled(dda); enabled {
			if flavor == v1alpha1.NetworkPolicyFlavorCilium {
				f.createCiliumNetworkPolicy = true
			} else {
				f.createKubernetesNetworkPolicy = true
			}
		}

		return feature.RequiredComponents{
			ClusterAgent:        feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			ClusterChecksRunner: feature.RequiredComponent{IsRequired: &f.useClusterCheckRunners},
		}
	}

	return feature.RequiredComponents{
		ClusterChecksRunner: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(false)},
	}
}

func (f *clusterChecksFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	policyName, podSelector := component.GetNetworkPolicyMetadata(f.owner, v2alpha1.ClusterAgentComponentName)
	_, ccrPodSelector := component.GetNetworkPolicyMetadata(f.owner, v2alpha1.ClusterChecksRunnerComponentName)
	if f.createKubernetesNetworkPolicy {
		ingressRules := []netv1.NetworkPolicyIngressRule{
			{
				Ports: []netv1.NetworkPolicyPort{
					{
						Port: &intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: apicommon.DefaultClusterAgentServicePort,
						},
					},
				},
				From: []netv1.NetworkPolicyPeer{
					{
						PodSelector: &ccrPodSelector,
					},
				},
			},
		}
		return managers.NetworkPolicyManager().AddKubernetesNetworkPolicy(
			policyName,
			f.owner.GetNamespace(),
			podSelector,
			nil,
			ingressRules,
			nil,
		)
	} else if f.createCiliumNetworkPolicy {
		policySpecs := []cilium.NetworkPolicySpec{
			{
				Description:      "Ingress from cluster workers",
				EndpointSelector: podSelector,
				Ingress: []cilium.IngressRule{
					{
						FromEndpoints: []metav1.LabelSelector{ccrPodSelector},
						ToPorts: []cilium.PortRule{
							{
								Ports: []cilium.PortProtocol{
									{
										Port:     "5005",
										Protocol: cilium.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
		}
		return managers.CiliumPolicyManager().AddCiliumPolicy(policyName, f.owner.GetNamespace(), policySpecs)
	}

	return nil
}

func (f *clusterChecksFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(
		common.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  apicommon.DDClusterChecksEnabled,
			Value: "true",
		},
	)

	managers.EnvVar().AddEnvVarToContainer(
		common.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  apicommon.DDExtraConfigProviders,
			Value: apicommon.KubeServicesAndEndpointsConfigProviders,
		},
	)

	managers.EnvVar().AddEnvVarToContainer(
		common.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  apicommon.DDExtraListeners,
			Value: apicommon.KubeServicesAndEndpointsListeners,
		},
	)

	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}

	return nil
}

func (f *clusterChecksFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	if f.useClusterCheckRunners {
		managers.EnvVar().AddEnvVarToContainer(
			common.CoreAgentContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDExtraConfigProviders,
				Value: apicommon.EndpointsChecksConfigProvider,
			},
		)
	} else {
		managers.EnvVar().AddEnvVarToContainer(
			common.CoreAgentContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDExtraConfigProviders,
				Value: apicommon.ClusterAndEndpointsConfigProviders,
			},
		)
	}

	return nil
}

func (f *clusterChecksFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	if f.useClusterCheckRunners {
		managers.EnvVar().AddEnvVarToContainer(
			common.ClusterChecksRunnersContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDClusterChecksEnabled,
				Value: "true",
			},
		)

		managers.EnvVar().AddEnvVarToContainer(
			common.ClusterChecksRunnersContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDExtraConfigProviders,
				Value: apicommon.ClusterChecksConfigProvider,
			},
		)
	}

	return nil
}

func (f *clusterChecksFeature) updateConfigHash(dda *v2alpha1.DatadogAgent) {
	hash, err := comparison.GenerateMD5ForSpec(dda.Spec.Features.ClusterChecks)
	if err != nil {
		f.logger.Error(err, "couldn't generate hash for cluster checks config")
	} else {
		f.logger.V(2).Info("created cluster checks", "hash", hash)
	}
	f.customConfigAnnotationValue = hash
	f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.ClusterChecksIDType)
}
