package guess

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// karpenterCoreRules is the minimal set of rules that uniquely identifies a
// Karpenter ClusterRole: the chart's clusterrole-core.yaml hard-codes the
// karpenter.sh API group with nodepools/nodeclaims, so any real Karpenter
// install produces a ClusterRole containing them.
var karpenterCoreRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{"karpenter.sh"},
		Resources: []string{"nodepools", "nodeclaims"},
		Verbs:     []string{"get", "list", "watch"},
	},
}

// TestKarpenterAPIGroupContract pins the API group fingerprint we match
// against. Subsequent tests build fake objects via the same constant, so a
// typo here would silently make them pass while real Karpenter installs stop
// matching — this assertion locks down the contract against the chart's
// hard-coded apiGroup.
func TestKarpenterAPIGroupContract(t *testing.T) {
	assert.Equal(t, "karpenter.sh", karpenterAPIGroup)
}

func TestIsForeignKarpenterInstalled(t *testing.T) {
	for _, tc := range []struct {
		name     string
		objects  []runtime.Object
		expected bool
	}{
		{
			name:     "no ClusterRoles on the cluster",
			objects:  nil,
			expected: false,
		},
		{
			name: "no Karpenter ClusterRoles among unrelated ones",
			objects: []runtime.Object{
				clusterRole("system:auth-delegator", nil, []rbacv1.PolicyRule{
					{APIGroups: []string{"authentication.k8s.io"}, Resources: []string{"tokenreviews"}, Verbs: []string{"create"}},
				}),
			},
			expected: false,
		},
		{
			name: "only kubectl-datadog ClusterRoles",
			objects: []runtime.Object{
				clusterRole("karpenter", map[string]string{
					"app.kubernetes.io/name":     "karpenter",
					"app.kubernetes.io/instance": "karpenter",
					InstalledByLabel:             InstalledByValue,
				}, karpenterCoreRules),
				clusterRole("karpenter-core", map[string]string{
					"app.kubernetes.io/name":     "karpenter",
					"app.kubernetes.io/instance": "karpenter",
					InstalledByLabel:             InstalledByValue,
				}, karpenterCoreRules),
			},
			expected: false,
		},
		{
			name: "foreign ClusterRole without our sentinel",
			objects: []runtime.Object{
				clusterRole("karpenter", map[string]string{
					"app.kubernetes.io/name":       "karpenter",
					"app.kubernetes.io/instance":   "karpenter",
					"app.kubernetes.io/managed-by": "Helm",
				}, karpenterCoreRules),
			},
			expected: true,
		},
		{
			name: "foreign Karpenter installed with custom nameOverride",
			// Helm chart with `nameOverride: my-karpenter` renames both the
			// ClusterRole and its app.kubernetes.io/name label. The rules,
			// however, still reference karpenter.sh — so we must detect it.
			objects: []runtime.Object{
				clusterRole("my-karpenter-core", map[string]string{
					"app.kubernetes.io/name":       "my-karpenter",
					"app.kubernetes.io/instance":   "my-karpenter",
					"app.kubernetes.io/managed-by": "Helm",
				}, karpenterCoreRules),
			},
			expected: true,
		},
		{
			name: "mix of ours and foreign returns true",
			objects: []runtime.Object{
				clusterRole("karpenter-core", map[string]string{
					"app.kubernetes.io/name":     "karpenter",
					"app.kubernetes.io/instance": "karpenter",
					InstalledByLabel:             InstalledByValue,
				}, karpenterCoreRules),
				clusterRole("karpenter", map[string]string{
					"app.kubernetes.io/name":     "karpenter",
					"app.kubernetes.io/instance": "their-release",
				}, karpenterCoreRules),
			},
			expected: true,
		},
		{
			name: "ClusterRole with karpenter-looking labels but no karpenter.sh rules is ignored",
			// Defensive: a user-authored ClusterRole that happens to carry
			// `app.kubernetes.io/name=karpenter` but no actual Karpenter
			// rules must not trigger the guard.
			objects: []runtime.Object{
				clusterRole("fake-karpenter", map[string]string{
					"app.kubernetes.io/name": "karpenter",
				}, []rbacv1.PolicyRule{
					{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
				}),
			},
			expected: false,
		},
		{
			name: "foreign sentinel value is treated as foreign",
			objects: []runtime.Object{
				clusterRole("karpenter", map[string]string{
					"app.kubernetes.io/name": "karpenter",
					InstalledByLabel:         "someone-else",
				}, karpenterCoreRules),
			},
			expected: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.objects...)

			result, err := IsForeignKarpenterInstalled(t.Context(), clientset)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}

	t.Run("API list error propagates", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("list", "clusterroles", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewServiceUnavailable("test failure")
		})

		_, err := IsForeignKarpenterInstalled(t.Context(), clientset)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list ClusterRoles")
	})

	t.Run("pagination follows Continue token across pages and short-circuits on first foreign match", func(t *testing.T) {
		// Three pages: an empty page with a non-empty Continue token
		// (the API server may legitimately return one), a page with only
		// our own ClusterRole, and a page where the foreign install lives.
		// We expect the function to walk pages 1 and 2, find the foreign on
		// page 3, and stop. Page 4 must never be requested.
		pages := []*rbacv1.ClusterRoleList{
			{
				ListMeta: metav1.ListMeta{Continue: "page2"},
				Items:    nil,
			},
			{
				ListMeta: metav1.ListMeta{Continue: "page3"},
				Items: []rbacv1.ClusterRole{
					*clusterRole("karpenter", map[string]string{
						InstalledByLabel: InstalledByValue,
					}, karpenterCoreRules),
				},
			},
			{
				ListMeta: metav1.ListMeta{Continue: "page4"},
				Items: []rbacv1.ClusterRole{
					*clusterRole("their-karpenter", map[string]string{
						"app.kubernetes.io/instance": "their-release",
					}, karpenterCoreRules),
				},
			},
			{
				Items: []rbacv1.ClusterRole{
					*clusterRole("never-fetched", nil, karpenterCoreRules),
				},
			},
		}

		clientset := fake.NewSimpleClientset()
		var calls []string
		clientset.PrependReactor("list", "clusterroles", func(action k8stesting.Action) (bool, runtime.Object, error) {
			opts := action.(k8stesting.ListActionImpl).GetListOptions()
			calls = append(calls, opts.Continue)
			assert.EqualValues(t, clusterRoleListChunkSize, opts.Limit, "Limit must be set so the API server can chunk")
			require.Less(t, len(calls)-1, len(pages),
				"reactor would over-fetch beyond the synthetic pages — early-exit broken")
			return true, pages[len(calls)-1], nil
		})

		result, err := IsForeignKarpenterInstalled(t.Context(), clientset)

		require.NoError(t, err)
		assert.True(t, result)
		assert.Equal(t, []string{"", "page2", "page3"}, calls,
			"each call must forward the previous page's Continue token, and page 4 must never be requested")
	})
}

func clusterRole(name string, labels map[string]string, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Rules: rules,
	}
}
