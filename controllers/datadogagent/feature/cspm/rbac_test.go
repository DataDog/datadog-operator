package cspm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func Test_getRBACPolicyRules(t *testing.T) {
	t.Parallel()

	type args struct {
		supportsPSP bool
	}
	tests := []struct {
		name string
		args args
		want []rbacv1.PolicyRule
	}{
		{
			name: "when supportsPSP is true",
			args: args{
				supportsPSP: true,
			},
			want: []rbacv1.PolicyRule{
				{
					APIGroups: []string{rbac.CoreAPIGroup},
					Resources: []string{
						rbac.NamespaceResource,
						rbac.ServiceAccountResource,
					},
					Verbs: []string{rbac.ListVerb},
				},
				{
					APIGroups: []string{rbac.PolicyAPIGroup},
					Resources: []string{
						rbac.PodSecurityPolicyResource,
					},
					Verbs: []string{rbac.GetVerb, rbac.ListVerb, rbac.WatchVerb},
				},
				{
					APIGroups: []string{rbac.RbacAPIGroup},
					Resources: []string{
						rbac.ClusterRoleBindingResource,
						rbac.RoleBindingResource,
					},
					Verbs: []string{rbac.ListVerb},
				},
				{
					APIGroups: []string{rbac.NetworkingAPIGroup},
					Resources: []string{
						rbac.NetworkPolicyResource,
					},
					Verbs: []string{rbac.ListVerb},
				},
			},
		},
		{
			name: "when supportsPSP is false",
			args: args{
				supportsPSP: false,
			},
			want: []rbacv1.PolicyRule{
				{
					APIGroups: []string{rbac.CoreAPIGroup},
					Resources: []string{
						rbac.NamespaceResource,
						rbac.ServiceAccountResource,
					},
					Verbs: []string{rbac.ListVerb},
				},
				{
					APIGroups: []string{rbac.RbacAPIGroup},
					Resources: []string{
						rbac.ClusterRoleBindingResource,
						rbac.RoleBindingResource,
					},
					Verbs: []string{rbac.ListVerb},
				},
				{
					APIGroups: []string{rbac.NetworkingAPIGroup},
					Resources: []string{
						rbac.NetworkPolicyResource,
					},
					Verbs: []string{rbac.ListVerb},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, getRBACPolicyRules(tt.args.supportsPSP))
		})
	}
}
