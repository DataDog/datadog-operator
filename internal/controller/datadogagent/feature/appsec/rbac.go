// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

const (
	appsecRBACPrefix = "appsec"
)

// getAppsecRBACResourceName returns the RBAC resources name for AppSec feature
func getAppsecRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), appsecRBACPrefix, suffix)
}

// getRBACPolicyRules generates the cluster role permissions required for the AppSec proxy injector feature
func getRBACPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{rbac.CoreAPIGroup},
			Resources: []string{rbac.EventsResource},
			Verbs: []string{
				rbac.GetVerb,
			},
		},
		// CRD detection
		{
			APIGroups: []string{rbac.APIExtensionsAPIGroup},
			Resources: []string{rbac.CustomResourceDefinitionsResource},
			Verbs: []string{
				rbac.GetVerb,
			},
		},
		// Envoy Gateway resources
		{
			APIGroups: []string{rbac.GatewayAPIGroup},
			Resources: []string{
				"gateways",
				"gatewayclasses",
			},
			Verbs: []string{
				rbac.GetVerb,
				rbac.ListVerb,
				rbac.WatchVerb,
				rbac.PatchVerb,
			},
		},
		{
			APIGroups: []string{rbac.GatewayAPIGroup},
			Resources: []string{"referencegrants"},
			Verbs: []string{
				rbac.GetVerb,
				rbac.DeleteVerb,
				rbac.CreateVerb,
				rbac.PatchVerb,
			},
		},
		{
			APIGroups: []string{"gateway.envoyproxy.io"},
			Resources: []string{"envoyextensionpolicies"},
			Verbs: []string{
				rbac.GetVerb,
				rbac.DeleteVerb,
				rbac.CreateVerb,
			},
		},
		// Istio resources
		{
			APIGroups: []string{"networking.istio.io"},
			Resources: []string{"envoyfilters"},
			Verbs: []string{
				rbac.GetVerb,
				rbac.CreateVerb,
				rbac.DeleteVerb,
			},
		},
	}
}
