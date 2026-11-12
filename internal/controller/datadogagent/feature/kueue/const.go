// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kueue

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

const (
	// DDClusterAgentKueueEnabled enables Kueue metadata collection in the Cluster Agent
	DDClusterAgentKueueEnabled = "DD_CLUSTER_AGENT_KUEUE_ENABLED"

	kueueConfFileName     = "kueue.yaml"
	kueueCheckFolderName  = "kueue.d"
	kueueRBACPrefix       = "kueue"
	kueueConfigVolumeName = "kueue-config"

	// defaultKueueConf default Kueue check ConfigMap name suffix
	defaultKueueConf = "kueue-config"

	// Add "-0" so that prerelease versions are considered sufficient.
	kueueMinVersion = "7.82.0-0"

	// Autodiscovery template variables, resolved per Kueue pod by the Cluster Agent.
	kueueOpenMetricsEndpoint = "https://%%host%%:%%port%%/metrics"

	defaultMetricsServiceName      = "kueue-controller-manager-metrics-service"
	defaultMetricsServiceNamespace = "kueue-system"
)

// Cluster Agent: Kueue workloadmeta collector (list/watch on the Kueue CRs).
var kueueClusterAgentRBACPolicyRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbac.KueueAPIGroup},
		Resources: []string{
			rbac.KueueClusterQueuesResource,
			rbac.KueueLocalQueuesResource,
			rbac.KueueResourceFlavorsResource,
			rbac.KueueWorkloadsResource,
		},
		Verbs: []string{
			rbac.ListVerb,
			rbac.WatchVerb,
		},
	},
}

// Node Agent: the Kueue check reads Workloads to emit lifecycle events.
var kueueNodeAgentRBACPolicyRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbac.KueueAPIGroup},
		Resources: []string{rbac.KueueWorkloadsResource},
		Verbs: []string{
			rbac.GetVerb,
			rbac.ListVerb,
		},
	},
}

func getKueueRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), kueueRBACPrefix, suffix)
}
