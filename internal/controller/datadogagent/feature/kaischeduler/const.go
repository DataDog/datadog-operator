// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kaischeduler

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

const (
	// DDClusterAgentKAISchedulerEnabled enables KAI PodGroup collection in the Cluster Agent
	DDClusterAgentKAISchedulerEnabled = "DD_CLUSTER_AGENT_KAI_SCHEDULER_ENABLED"
	// DDGPUKAISchedulerEnabled enables the mapping of KAI fractional GPUs in the node Agent
	DDGPUKAISchedulerEnabled = "DD_GPU_KAI_SCHEDULER_ENABLED"

	kaiSchedulerRBACPrefix = "kai-scheduler"

	// Add "-0" so that prerelease versions are considered sufficient.
	kaiSchedulerMinVersion = "7.86.0-0"
)

// Cluster Agent: KAI PodGroup workloadmeta collector.
var kaiSchedulerClusterAgentRBACPolicyRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbac.KAIAPIGroup},
		Resources: []string{rbac.KAIPodGroupsResource},
		Verbs: []string{
			rbac.ListVerb,
			rbac.WatchVerb,
		},
	},
}

func getKAISchedulerRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), kaiSchedulerRBACPrefix, suffix)
}
