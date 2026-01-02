// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"strings"

	"github.com/gobuffalo/flect"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// getRBACPolicyRules generates the cluster role required for the KSM informers to query
// what is exposed as of the v2.0 https://github.com/kubernetes/kube-state-metrics/blob/release-2.0/examples/standard/cluster-role.yaml
func getRBACPolicyRules(collectorOpts collectorOptions) []rbacv1.PolicyRule {
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{
				rbac.ConfigMapsResource,
				rbac.EndpointsResource,
				rbac.EventsResource,
				rbac.LimitRangesResource,
				rbac.NamespaceResource,
				rbac.NodesResource,
				rbac.PersistentVolumeClaimsResource,
				rbac.PersistentVolumesResource,
				rbac.PodsResource,
				rbac.ReplicationControllersResource,
				rbac.ResourceQuotasResource,
				rbac.SecretsResource,
				rbac.ServicesResource,
			},
		},
		{
			APIGroups: []string{rbac.AppsAPIGroup},
			Resources: []string{
				rbac.DaemonsetsResource,
				rbac.DeploymentsResource,
				rbac.ReplicasetsResource,
				rbac.StatefulsetsResource,
				rbac.ControllerRevisionsResource,
			},
		},
		{
			APIGroups: []string{rbac.BatchAPIGroup},
			Resources: []string{
				rbac.CronjobsResource,
				rbac.JobsResource,
			},
		},
		{
			APIGroups: []string{rbac.AutoscalingAPIGroup},
			Resources: []string{
				rbac.HorizontalPodAutoscalersRecource,
			},
		},
		{
			APIGroups: []string{rbac.PolicyAPIGroup},
			Resources: []string{
				rbac.PodDisruptionBudgetsResource,
			},
		},
		{
			APIGroups: []string{rbac.CertificatesAPIGroup},
			Resources: []string{
				rbac.CertificatesSigningRequestsResource,
			},
		},
		{
			APIGroups: []string{rbac.StorageAPIGroup},
			Resources: []string{
				rbac.StorageClassesResource,
				rbac.VolumeAttachments,
			},
		},
		{
			APIGroups: []string{rbac.AdmissionAPIGroup},
			Resources: []string{
				rbac.MutatingConfigResource,
				rbac.ValidatingConfigResource,
			},
		},
		{
			APIGroups: []string{rbac.NetworkingAPIGroup},
			Resources: []string{
				rbac.IngressesResource,
				rbac.NetworkPolicyResource,
			},
		},
		{
			APIGroups: []string{rbac.CoordinationAPIGroup},
			Resources: []string{
				rbac.LeasesResource,
			},
		},
		{
			APIGroups: []string{rbac.AutoscalingK8sIoAPIGroup},
			Resources: []string{
				rbac.VPAResource,
			},
		},
	}

	if collectorOpts.enableAPIService {
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{rbac.RegistrationAPIGroup},
			Resources: []string{
				rbac.APIServicesResource,
			},
		})
	}

	if collectorOpts.enableCRD {
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{rbac.APIExtensionsAPIGroup},
			Resources: []string{
				rbac.CustomResourceDefinitionsResource,
			},
		})
	}

	commonVerbs := []string{
		rbac.ListVerb,
		rbac.WatchVerb,
	}

	for i := range rbacRules {
		rbacRules[i].Verbs = commonVerbs
	}

	// Add permissions for custom resources
	if len(collectorOpts.customResources) > 0 {
		rbacBuilder := utils.NewRBACBuilder(commonVerbs...)
		for _, cr := range collectorOpts.customResources {
			// Don't pluralize if the kind is a wildcard
			if cr.GroupVersionKind.Kind == "*" {
				rbacBuilder.AddGroupKind(cr.GroupVersionKind.Group, "*")
				continue
			}

			// Use the resource plural if specified, otherwise derive it from the Kind
			resourceName := cr.ResourcePlural
			if resourceName == "" {
				resourceName = strings.ToLower(flect.Pluralize(cr.GroupVersionKind.Kind))
			}
			rbacBuilder.AddGroupKind(cr.GroupVersionKind.Group, resourceName)
		}
		rbacRules = append(rbacRules, rbacBuilder.Build()...)
	}

	return rbacRules
}
