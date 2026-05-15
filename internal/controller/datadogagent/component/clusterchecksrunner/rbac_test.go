// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecksrunner

import (
	"testing"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func TestGetDefaultClusterChecksRunnerClusterRolePolicyRules(t *testing.T) {
	dda := &metav1.ObjectMeta{Name: "datadog"}

	withNonResourceRules := GetDefaultClusterChecksRunnerClusterRolePolicyRules(dda, false)
	withoutNonResourceRules := GetDefaultClusterChecksRunnerClusterRolePolicyRules(dda, true)

	require.Len(t, withNonResourceRules, len(withoutNonResourceRules)+1)
	require.Contains(t, withNonResourceRules, rbacv1.PolicyRule{
		NonResourceURLs: []string{rbac.MetricsURL, rbac.MetricsSLIsURL},
		Verbs:           []string{rbac.GetVerb},
	})
	require.NotContains(t, withoutNonResourceRules, rbacv1.PolicyRule{
		NonResourceURLs: []string{rbac.MetricsURL, rbac.MetricsSLIsURL},
		Verbs:           []string{rbac.GetVerb},
	})
}
