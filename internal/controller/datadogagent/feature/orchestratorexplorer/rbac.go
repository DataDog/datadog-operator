// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// getRBACRules generates the cluster role permissions required for the orchestrator explorer feature
func getRBACPolicyRules(logger logr.Logger, crs []string, collectKubernetesNetworkResources bool) []rbacv1.PolicyRule {
	rbacRules := []rbacv1.PolicyRule{
		// To get the kube-system namespace UID and generate a cluster ID
		{
			APIGroups:     []string{rbac.CoreAPIGroup},
			Resources:     []string{rbac.NamespaceResource},
			ResourceNames: []string{common.KubeSystemResourceName},
			Verbs:         []string{rbac.GetVerb},
		},
		// To create the cluster-id configmap
		{
			APIGroups:     []string{rbac.CoreAPIGroup},
			Resources:     []string{rbac.ConfigMapsResource},
			ResourceNames: []string{common.DatadogClusterIDResourceName},
			Verbs: []string{
				rbac.GetVerb,
				rbac.CreateVerb,
				rbac.UpdateVerb,
			},
		},
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.PodsResource,
				rbac.ServicesResource,
				rbac.NodesResource,
				rbac.PersistentVolumesResource,
				rbac.PersistentVolumeClaimsResource,
				rbac.ServiceAccountResource,
				rbac.LimitRangesResource,
			},
		},
		{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{
				rbac.DeploymentsResource,
				rbac.ReplicasetsResource,
				rbac.DaemonsetsResource,
				rbac.StatefulsetsResource,
			},
		},
		{
			APIGroups: []string{rbac.BatchAPIGroup},
			Resources: []string{
				rbac.JobsResource,
				rbac.CronjobsResource,
			},
		},
		{
			APIGroups: []string{rbac.RbacAPIGroup},
			Resources: []string{
				rbac.RoleResource,
				rbac.RoleBindingResource,
				rbac.ClusterRoleResource,
				rbac.ClusterRoleBindingResource,
			},
		},
		{
			APIGroups: []string{rbac.NetworkingAPIGroup},
			Resources: []string{rbac.IngressesResource},
		},
		{
			APIGroups: []string{rbac.AutoscalingK8sIoAPIGroup},
			Resources: []string{rbac.VPAResource},
		},
		{
			APIGroups: []string{apiextensionsv1.GroupName},
			Resources: []string{common.CustomResourceDefinitionsName},
		},
		{
			APIGroups: []string{rbac.AutoscalingAPIGroup},
			Resources: []string{rbac.HorizontalPodAutoscalersRecource},
		},
		{
			APIGroups: []string{rbac.StorageAPIGroup},
			Resources: []string{rbac.StorageClassesResource},
		},
		{
			APIGroups: []string{rbac.PolicyAPIGroup},
			Resources: []string{rbac.PodDisruptionBudgetsResource},
		},
		{
			APIGroups: []string{rbac.DiscoveryAPIGroup},
			Resources: []string{rbac.EndpointsSlicesResource},
		},
		{
			APIGroups: []string{rbac.DatadogAPIGroup},
			Resources: []string{rbac.Wildcard},
		},
		{
			APIGroups: []string{rbac.ArgoProjAPIGroup},
			Resources: []string{rbac.Rollout, rbac.Applications, rbac.Applicationsets},
		},
		{
			APIGroups: []string{rbac.FluxSourceToolkitAPIGroup},
			Resources: []string{
				rbac.Buckets,
				rbac.Helmcharts,
				rbac.Externalartifacts,
				rbac.Gitrepositories,
				rbac.Helmrepositories,
				rbac.Ocirepositories,
			},
		},
		{
			APIGroups: []string{rbac.FluxKustomizeToolkitAPIGroup},
			Resources: []string{rbac.Kustomizations},
		},
		{
			APIGroups: []string{rbac.KarpenterAPIGroup},
			Resources: []string{rbac.Wildcard},
		},
		{
			APIGroups: []string{rbac.KarpenterAWSAPIGroup},
			Resources: []string{rbac.Wildcard},
		},
		{
			APIGroups: []string{rbac.KarpenterAzureAPIGroup},
			Resources: []string{rbac.Wildcard},
		},
		{
			APIGroups: []string{rbac.EKSAPIGroup},
			Resources: []string{rbac.Wildcard},
		},
	}

	if collectKubernetesNetworkResources {
		rbacRules = append(rbacRules,
			// Gateway API — resource-specific
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.GatewayAPIGroup},
				Resources: []string{
					rbac.GatewaysResource,
					rbac.HTTPRoutesResource,
					rbac.GRPCRoutesResource,
					rbac.TLSRoutesResource,
					rbac.ListenerSetsResource,
				},
			},
			// Service Mesh — Istio: resource-specific
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.IstioNetworkingAPIGroup},
				Resources: []string{
					rbac.VirtualServicesResource,
					rbac.GatewaysResource,
					rbac.DestinationRulesResource,
					rbac.ServiceEntriesResource,
					rbac.SidecarsResource,
				},
			},
			// Service Mesh — Others: group-level
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.EnvoyGatewayAPIGroup},
				Resources: []string{rbac.Wildcard},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.TraefikLegacyAPIGroup},
				Resources: []string{rbac.Wildcard},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.LinkerdPolicyAPIGroup},
				Resources: []string{rbac.Wildcard},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.ConsulAPIGroup},
				Resources: []string{rbac.Wildcard},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.ConsulMeshAPIGroup},
				Resources: []string{rbac.Wildcard},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.KumaAPIGroup},
				Resources: []string{rbac.Wildcard},
			},
			// Ingress Controllers — NGINX: resource-specific
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.NginxAPIGroup},
				Resources: []string{
					rbac.VirtualServersResource,
					rbac.VirtualServerRoutesResource,
				},
			},
			// Ingress Controllers — Traefik: resource-specific
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.TraefikAPIGroup},
				Resources: []string{rbac.IngressRoutesResource},
			},
			// Ingress Controllers — Others: group-level
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.KongAPIGroup},
				Resources: []string{rbac.Wildcard},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.HAProxyCoreAPIGroup},
				Resources: []string{rbac.Wildcard},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{rbac.HAProxyIngressV1APIGroup},
				Resources: []string{rbac.Wildcard},
			},
		)
	}

	if len(crs) > 0 {
		for _, cr := range crs {
			crSplit := strings.Split(cr, "/")
			if len(crSplit) == 3 {
				// use ToLower as rbac resource names are lowercase but input may not be
				rbacRules = append(rbacRules, rbacv1.PolicyRule{
					APIGroups: []string{crSplit[0]},
					Resources: []string{strings.ToLower(crSplit[2])},
				})
			} else {
				logger.Error(fmt.Errorf("unable to create cluster role for %s, skipping", cr), "correct format should be group/version/resource")
			}
		}
	}

	defaultVerbs := []string{
		rbac.ListVerb,
		rbac.WatchVerb,
	}

	for i := range rbacRules {
		if rbacRules[i].Verbs == nil {
			// Add defaultVerbs only on Rules with no Verbs yet.
			rbacRules[i].Verbs = defaultVerbs
		}
	}

	return rbacRules
}
