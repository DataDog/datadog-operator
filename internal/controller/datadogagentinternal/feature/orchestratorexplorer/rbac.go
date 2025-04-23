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

	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// getRBACRules generates the cluster role permissions required for the orchestrator explorer feature
func getRBACPolicyRules(logger logr.Logger, crs []string) []rbacv1.PolicyRule {
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
	}

	groupResources := mapAPIGroupsResources(logger, crs)
	for group, resources := range groupResources {
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{group},
			Resources: append([]string{}, resources...),
		})
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

func mapAPIGroupsResources(logger logr.Logger, customResources []string) map[string][]string {
	groupResources := make(map[string][]string, len(customResources))
	for _, cr := range customResources {
		crSplit := strings.Split(cr, "/")
		if len(crSplit) == 3 {
			if _, ok := groupResources[crSplit[0]]; !ok {
				groupResources[crSplit[0]] = make([]string, 0, len(customResources))
			}
			groupResources[crSplit[0]] = append(groupResources[crSplit[0]], crSplit[2])
		} else {
			logger.Error(fmt.Errorf("unable to create cluster role for %s, skipping", cr), "correct format should be group/version/kind")
		}
	}
	return groupResources
}
