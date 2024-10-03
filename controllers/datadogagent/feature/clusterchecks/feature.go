// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/objects"
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
			Agent:               feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			ClusterAgent:        feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			ClusterChecksRunner: feature.RequiredComponent{IsRequired: &f.useClusterCheckRunners},
		}
	}

	return reqComp
}

func (f *clusterChecksFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	policyName, podSelector := objects.GetNetworkPolicyMetadata(f.owner, v2alpha1.ClusterAgentComponentName)
	_, ccrPodSelector := objects.GetNetworkPolicyMetadata(f.owner, v2alpha1.ClusterChecksRunnerComponentName)
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
		commonv1.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  apicommon.DDClusterChecksEnabled,
			Value: "true",
		},
	)

	managers.EnvVar().AddEnvVarToContainer(
		commonv1.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  apicommon.DDExtraConfigProviders,
			Value: apicommon.KubeServicesAndEndpointsConfigProviders,
		},
	)

	managers.EnvVar().AddEnvVarToContainer(
		commonv1.ClusterAgentContainerName,
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

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *clusterChecksFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.manageNodeAgent(commonv1.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

func (f *clusterChecksFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.manageNodeAgent(commonv1.CoreAgentContainerName, managers, provider)
	return nil
}

func (f *clusterChecksFeature) manageNodeAgent(agentContainerName commonv1.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {
	if f.useClusterCheckRunners {
		managers.EnvVar().AddEnvVarToContainer(
			agentContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDExtraConfigProviders,
				Value: apicommon.EndpointsChecksConfigProvider,
			},
		)
	} else {
		managers.EnvVar().AddEnvVarToContainer(
			agentContainerName,
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
			commonv1.ClusterChecksRunnersContainerName,
			&corev1.EnvVar{
				Name:  apicommon.DDClusterChecksEnabled,
				Value: "true",
			},
		)

		managers.EnvVar().AddEnvVarToContainer(
			commonv1.ClusterChecksRunnersContainerName,
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
