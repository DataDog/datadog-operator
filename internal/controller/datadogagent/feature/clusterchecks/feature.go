// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

import (
	"encoding/json"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/objects"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

func init() {
	err := feature.Register(feature.ClusterChecksIDType, buildClusterChecksFeature)
	if err != nil {
		panic(err)
	}
}

type clusterChecksFeature struct {
	useClusterCheckRunners bool
	// ccrFamilyEnabled is true when any runner Deployment exists: the default
	// CCR and/or experimental runner groups (which don't need the default CCR).
	ccrFamilyEnabled bool
	owner            metav1.Object

	createKubernetesNetworkPolicy bool
	createCiliumNetworkPolicy     bool

	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	// runnerGroups is the Cluster Agent's cluster_checks.runner_groups value:
	// the JSON map of each runner group to the checks it claims.
	runnerGroups string

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

func (f *clusterChecksFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	if apiutils.BoolValue(ddaSpec.Features.ClusterChecks.Enabled) {
		f.updateConfigHash(dda, ddaSpec)
		f.owner = dda

		if enabled, flavor := constants.IsNetworkPolicyEnabled(ddaSpec); enabled {
			if flavor == v2alpha1.NetworkPolicyFlavorCilium {
				f.createCiliumNetworkPolicy = true
			} else {
				f.createKubernetesNetworkPolicy = true
			}
		}

		f.useClusterCheckRunners = apiutils.BoolValue(ddaSpec.Features.ClusterChecks.UseClusterChecksRunners)
		runnerGroups, err := v2alpha1.GetEffectiveClusterChecksRunnerGroups(dda)
		if err != nil {
			f.logger.Error(err, "ignoring experimental cluster checks runner groups")
		}
		f.ccrFamilyEnabled = f.useClusterCheckRunners || len(runnerGroups) > 0
		f.runnerGroups = runnerGroupsJSON(runnerGroups)
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: new(true),
				Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
			},
			ClusterAgent: feature.RequiredComponent{
				IsRequired: new(true),
				Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
			},
			ClusterChecksRunner: feature.RequiredComponent{
				// Runners-only: in mixed mode the default CCR Deployment is NOT
				// created; the DDAI reconciler materializes the group Deployments
				// and the CCR SA/RBAC dependencies instead.
				IsRequired: &f.useClusterCheckRunners,
				Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
			},
		}
	}

	return reqComp
}

func (f *clusterChecksFeature) ManageDependencies(managers feature.ResourceManagers) error {
	policyName, podSelector := objects.GetNetworkPolicyMetadata(f.owner, v2alpha1.ClusterAgentComponentName)
	_, ccrPodSelector := objects.GetNetworkPolicyMetadata(f.owner, v2alpha1.ClusterChecksRunnerComponentName)
	if f.createKubernetesNetworkPolicy {
		ingressRules := []netv1.NetworkPolicyIngressRule{
			{
				Ports: []netv1.NetworkPolicyPort{
					{
						Port: &intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: common.DefaultClusterAgentServicePort,
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
		apicommon.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  DDClusterChecksEnabled,
			Value: "true",
		},
	)

	managers.EnvVar().AddEnvVarToContainer(
		apicommon.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  DDExtraConfigProviders,
			Value: kubeServicesAndEndpointsConfigProviders,
		},
	)

	managers.EnvVar().AddEnvVarToContainer(
		apicommon.ClusterAgentContainerName,
		&corev1.EnvVar{
			Name:  DDExtraListeners,
			Value: kubeServicesAndEndpointsListeners,
		},
	)

	// The Cluster Agent dispatches each claimed check only to its group, and
	// every other check only to general workers (node agents, default CCR).
	// Changing the groups rolls the Cluster Agent, not the node agents.
	if f.runnerGroups != "" {
		managers.EnvVar().AddEnvVarToContainer(
			apicommon.ClusterAgentContainerName,
			&corev1.EnvVar{
				Name:  DDClusterChecksRunnerGroups,
				Value: f.runnerGroups,
			},
		)
	}

	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *clusterChecksFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	f.manageNodeAgent(apicommon.UnprivilegedSingleAgentContainerName, managers)
	return nil
}

func (f *clusterChecksFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	f.manageNodeAgent(apicommon.CoreAgentContainerName, managers)
	return nil
}

func (f *clusterChecksFeature) manageNodeAgent(agentContainerName apicommon.AgentContainerName, managers feature.PodTemplateManagers) error {
	if f.useClusterCheckRunners {
		managers.EnvVar().AddEnvVarToContainer(
			agentContainerName,
			&corev1.EnvVar{
				Name:  DDExtraConfigProviders,
				Value: endpointsChecksConfigProvider,
			},
		)
	} else {
		managers.EnvVar().AddEnvVarToContainer(
			agentContainerName,
			&corev1.EnvVar{
				Name:  DDExtraConfigProviders,
				Value: clusterAndEndpointsConfigProviders,
			},
		)
	}

	return nil
}

func (f *clusterChecksFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	// Applies to the default CCR and runner groups alike.
	if f.ccrFamilyEnabled {
		managers.EnvVar().AddEnvVarToContainer(
			apicommon.ClusterChecksRunnersContainerName,
			&corev1.EnvVar{
				Name:  DDClusterChecksEnabled,
				Value: "true",
			},
		)

		managers.EnvVar().AddEnvVarToContainer(
			apicommon.ClusterChecksRunnersContainerName,
			&corev1.EnvVar{
				Name:  DDExtraConfigProviders,
				Value: clusterChecksConfigProvider,
			},
		)
	}

	return nil
}

// runnerGroupsJSON returns the groups as the Cluster Agent's
// cluster_checks.runner_groups value: a JSON object mapping each group to
// the checks it claims, or "" without groups. Map keys are marshalled sorted,
// so the value is stable across reconciles.
func runnerGroupsJSON(groups []v2alpha1.ClusterChecksRunnerGroup) string {
	if len(groups) == 0 {
		return ""
	}
	claims := make(map[string][]string, len(groups))
	for _, group := range groups {
		claims[group.Name] = group.ChecksInclude
	}
	raw, _ := json.Marshal(claims) // a map of string slices always marshals
	return string(raw)
}

func (f *clusterChecksFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers) error {
	return nil
}

func (f *clusterChecksFeature) updateConfigHash(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) {
	hash, err := comparison.GenerateMD5ForSpec(ddaSpec.Features.ClusterChecks)
	if err != nil {
		f.logger.Error(err, "couldn't generate hash for cluster checks config")
	} else {
		f.logger.V(2).Info("created cluster checks", "hash", hash)
	}
	f.customConfigAnnotationValue = hash
	f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.ClusterChecksIDType)
}
