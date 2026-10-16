// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelcollector

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// getStandaloneRBACResourceName returns the RBAC resource name of otel-agent running without the core Agent.
func getStandaloneRBACResourceName(owner metav1.Object) string {
	return fmt.Sprintf("%s-%s-%s", owner.GetNamespace(), owner.GetName(), "otel-agent-standalone")
}

// standaloneRBACPolicyRules are needed by the k8s_attributes processor of the default config.
var standaloneRBACPolicyRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{rbac.PodsResource, rbac.NamespaceResource},
		Verbs:     []string{rbac.GetVerb, rbac.ListVerb, rbac.WatchVerb},
	},
	{
		APIGroups: []string{rbac.AppsAPIGroup},
		Resources: []string{rbac.ReplicasetsResource},
		Verbs:     []string{rbac.GetVerb, rbac.ListVerb, rbac.WatchVerb},
	},
}
