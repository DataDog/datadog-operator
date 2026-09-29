// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

import (
	"sort"
	"strings"

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
	// kubeChecksRunnerDefaultEnabled mirrors the experimental
	// kube-checks-runner-default annotation (mixed mode: runner groups exist
	// without the default CCR Deployment).
	kubeChecksRunnerDefaultEnabled bool
	owner                          metav1.Object

	createKubernetesNetworkPolicy bool
	createCiliumNetworkPolicy     bool

	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	// defaultRunnerChecksExclude is the union of every runner group's
	// ChecksInclude: checks claimed by a group stay off the default runners.
	defaultRunnerChecksExclude []string

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
		f.kubeChecksRunnerDefaultEnabled = v2alpha1.IsExperimentalKubeChecksRunnerDefaultEnabled(dda)
		runnerGroups, err := v2alpha1.GetEffectiveClusterChecksRunnerGroups(dda)
		if err != nil {
			f.logger.Error(err, "ignoring malformed experimental cluster checks runner groups annotation")
		}
		if !f.useClusterCheckRunners && !f.kubeChecksRunnerDefaultEnabled && len(runnerGroups) > 0 {
			f.logger.Info("ignoring experimental cluster checks runner groups: they require useClusterChecksRunners or the experimental-kube-checks-runner-default annotation")
		}
		// The exclude union only covers MATERIALIZED groups: with both
		// useClusterCheckRunners and the knob off, groups are ignored and must
		// not restrict node agents.
		if f.useClusterCheckRunners || f.kubeChecksRunnerDefaultEnabled {
			f.defaultRunnerChecksExclude = defaultRunnerChecksExclude(runnerGroups)
		}
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

	// Strict isolation: node agents refuse the checks claimed by runner
	// groups (same exclude union as the default CCR), so group-claimed checks
	// only ever run on their group — no fallback if the group is down.
	if len(f.defaultRunnerChecksExclude) > 0 {
		managers.EnvVar().AddEnvVarToContainer(
			agentContainerName,
			&corev1.EnvVar{
				Name:  DDCLCRunnerChecksExclude,
				Value: strings.Join(f.defaultRunnerChecksExclude, " "),
			},
		)
	}

	return nil
}

func (f *clusterChecksFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	// Base runner envs: apply to the default CCR and dedicated groups alike.
	// Group Deployments later overwrite the exclude env with their own
	// include/exclude lists (applyClusterChecksRunnerGroupCompatibility).
	if f.useClusterCheckRunners || f.kubeChecksRunnerDefaultEnabled {
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

		if len(f.defaultRunnerChecksExclude) > 0 {
			managers.EnvVar().AddEnvVarToContainer(
				apicommon.ClusterChecksRunnersContainerName,
				&corev1.EnvVar{
					Name:  DDCLCRunnerChecksExclude,
					Value: strings.Join(f.defaultRunnerChecksExclude, " "),
				},
			)
		}
	}

	return nil
}

// defaultRunnerChecksExclude returns the sorted, de-duplicated union of
// every runner group's ChecksInclude: what the default CCR must refuse to run.
func defaultRunnerChecksExclude(runners []v2alpha1.ClusterChecksRunnerGroup) []string {
	seen := make(map[string]struct{})
	for _, runner := range runners {
		for _, check := range runner.ChecksInclude {
			seen[check] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil
	}

	excludes := make([]string, 0, len(seen))
	for check := range seen {
		excludes = append(excludes, check)
	}
	sort.Strings(excludes)

	return excludes
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
