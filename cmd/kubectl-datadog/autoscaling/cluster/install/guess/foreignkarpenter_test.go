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

// TestKarpenterClusterRoleSelectorContract pins the exact label selector we
// match against. Subsequent tests build fake objects via the same constants,
// so a typo in the constants would silently make those tests pass while real
// Karpenter installs stop matching — this assertion locks down the contract
// against the chart's documented label.
func TestKarpenterClusterRoleSelectorContract(t *testing.T) {
	assert.Equal(t, "app.kubernetes.io/name=karpenter", karpenterClusterRoleSelector)
}

func TestIsForeignKarpenterInstalled(t *testing.T) {
	for _, tc := range []struct {
		name     string
		objects  []runtime.Object
		expected bool
	}{
		{
			name:     "no Karpenter ClusterRoles on the cluster",
			objects:  nil,
			expected: false,
		},
		{
			name: "only kubectl-datadog ClusterRoles",
			objects: []runtime.Object{
				clusterRole("karpenter", map[string]string{
					karpenterChartNameLabel:      karpenterChartNameValue,
					"app.kubernetes.io/instance": "karpenter",
					InstalledByLabel:             InstalledByValue,
				}),
				clusterRole("karpenter-core", map[string]string{
					karpenterChartNameLabel:      karpenterChartNameValue,
					"app.kubernetes.io/instance": "karpenter",
					InstalledByLabel:             InstalledByValue,
				}),
			},
			expected: false,
		},
		{
			name: "foreign ClusterRole without our sentinel",
			objects: []runtime.Object{
				clusterRole("karpenter", map[string]string{
					karpenterChartNameLabel:        karpenterChartNameValue,
					"app.kubernetes.io/instance":   "karpenter",
					"app.kubernetes.io/managed-by": "Helm",
				}),
			},
			expected: true,
		},
		{
			name: "mix of ours and foreign returns true",
			objects: []runtime.Object{
				clusterRole("karpenter-core", map[string]string{
					karpenterChartNameLabel:      karpenterChartNameValue,
					"app.kubernetes.io/instance": "karpenter",
					InstalledByLabel:             InstalledByValue,
				}),
				clusterRole("karpenter", map[string]string{
					karpenterChartNameLabel:      karpenterChartNameValue,
					"app.kubernetes.io/instance": "their-release",
				}),
			},
			expected: true,
		},
		{
			name: "ClusterRole without the karpenter selector label is ignored",
			objects: []runtime.Object{
				clusterRole("unrelated", map[string]string{
					karpenterChartNameLabel: "something-else",
				}),
			},
			expected: false,
		},
		{
			name: "foreign sentinel value is treated as foreign",
			objects: []runtime.Object{
				clusterRole("karpenter", map[string]string{
					karpenterChartNameLabel: karpenterChartNameValue,
					InstalledByLabel:        "someone-else",
				}),
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
		assert.Contains(t, err.Error(), "failed to list Karpenter ClusterRoles")
	})
}

func clusterRole(name string, labels map[string]string) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}
