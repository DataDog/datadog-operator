// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsbackend

import (
	"testing"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
)

func Test_getSecretsRolesPermissions(t *testing.T) {
	for _, tt := range []struct {
		name               string
		role               secretsBackendRole
		expectedPolicyRule []rbacv1.PolicyRule
	}{
		{
			// Classic use case : access within a namespace of specific secrets
			name: "role with namespace and 3 secrets",
			role: secretsBackendRole{
				namespace:   "foo",
				secretsList: []string{"bar", "baz", "qux"},
			},
			expectedPolicyRule: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{rbac.CoreAPIGroup},
					Resources:     []string{rbac.SecretsResource},
					ResourceNames: []string{"bar", "baz", "qux"},
					Verbs:         []string{rbac.GetVerb},
				},
			},
		},
	} {
		assert.Equal(t, tt.expectedPolicyRule, getSecretsRolesPermissions(tt.role))
	}

}
