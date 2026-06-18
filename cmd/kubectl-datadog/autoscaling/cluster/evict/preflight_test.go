package evict

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func newKarpenterScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	sch := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(sch))
	gv := schema.GroupVersion{Group: "karpenter.sh", Version: "v1"}
	sch.AddKnownTypes(gv, &karpv1.NodePool{}, &karpv1.NodePoolList{})
	metav1.AddToGroupVersion(sch, gv)
	return sch
}

func mkNodePool(name string, weight *int32, datadogManaged bool) *karpv1.NodePool {
	labels := map[string]string{}
	if datadogManaged {
		labels[ddNodePoolCreatedLabel] = "true"
	}
	return &karpv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       karpv1.NodePoolSpec{Weight: weight},
	}
}

func mkStaticNodePool(name string, weight *int32, datadogManaged bool) *karpv1.NodePool {
	np := mkNodePool(name, weight, datadogManaged)
	np.Spec.Replicas = ptr.To(int64(1))
	return np
}

func TestWarnKarpenterWeightConflicts(t *testing.T) {
	// The check is cluster-wide: it never looks at the eviction targets, only at
	// the NodePools present in the cluster, because Karpenter arbitrates
	// provisioning across every NodePool by weight.
	for _, tc := range []struct {
		name      string
		nodePools []ctrlclient.Object
		// wantContains lists substrings that MUST appear in the stderr output.
		wantContains []string
		// wantWarnEmpty asserts the stderr output is empty (no warning fired).
		wantWarnEmpty bool
	}{
		{
			// No Datadog NodePool ⇒ nothing to migrate onto ⇒ no warning, even
			// though a user NodePool exists.
			name: "no Datadog NodePool ⇒ no warning",
			nodePools: []ctrlclient.Object{
				mkNodePool("user-np", ptr.To(int32(50)), false),
			},
			wantWarnEmpty: true,
		},
		{
			// User NP weight < Datadog NP weight ⇒ no conflict, no warning.
			name: "user weight below Datadog weight",
			nodePools: []ctrlclient.Object{
				mkNodePool("dd-np", ptr.To(int32(100)), true),
				mkNodePool("user-np", ptr.To(int32(50)), false),
			},
			wantWarnEmpty: true,
		},
		{
			// User NP weight > Datadog NP weight ⇒ warns (`>=` check). The user
			// NodePool is not an eviction target here, demonstrating the
			// cluster-wide scope.
			name: "user weight above Datadog weight warns",
			nodePools: []ctrlclient.Object{
				mkNodePool("dd-np", ptr.To(int32(10)), true),
				mkNodePool("user-np", ptr.To(int32(50)), false),
			},
			wantContains: []string{"user-np", "weight=50"},
		},
		{
			// Tie at the max weight 100: mirrors the cluster-agent edge case
			// where a target NodePool already at weight 100 yields an equal
			// (not higher) Datadog replica weight. The `>=` check catches it.
			name: "equal weight at max 100 warns",
			nodePools: []ctrlclient.Object{
				mkNodePool("dd-np", ptr.To(int32(100)), true),
				mkNodePool("user-np", ptr.To(int32(100)), false),
			},
			wantContains: []string{"user-np", "weight=100"},
		},
		{
			// Both nil ⇒ both default to 0 ⇒ equal-weight conflict warns.
			name: "nil weights both default to 0 (equal-weight conflict)",
			nodePools: []ctrlclient.Object{
				mkNodePool("dd-np", nil, true),
				mkNodePool("user-np", nil, false),
			},
			wantContains: []string{"user-np"},
		},
		{
			// Static NodePools (spec.replicas set) don't provision on weight-based
			// pod demand, so they're skipped even when their weight would
			// otherwise trip the check.
			name: "static user NodePool is skipped",
			nodePools: []ctrlclient.Object{
				mkNodePool("dd-np", ptr.To(int32(10)), true),
				mkStaticNodePool("user-static", ptr.To(int32(50)), false),
			},
			wantWarnEmpty: true,
		},
		{
			// A static Datadog NodePool carries no weight, so it neither raises
			// nor lowers the max — that still comes from the dynamic Datadog
			// NodePool, and the in-between user NodePool warns against it.
			name: "static Datadog NodePool does not affect dynamic max",
			nodePools: []ctrlclient.Object{
				mkStaticNodePool("dd-static", nil, true),
				mkNodePool("dd-dynamic", ptr.To(int32(10)), true),
				mkNodePool("user-np", ptr.To(int32(50)), false),
			},
			wantContains: []string{"user-np", "weight=50", "max weight=10"},
		},
		{
			// Regression: when the only Datadog NodePool is static (and thus
			// carries no weight), a dynamic user NodePool still out-prioritizes
			// it for demand-driven provisioning, so the warning must still fire
			// — the static Datadog NodePool must count as "present" (max 0), not
			// collapse the sentinel into an early return.
			name: "all-static Datadog still warns dynamic user NodePool",
			nodePools: []ctrlclient.Object{
				mkStaticNodePool("dd-static", nil, true),
				mkNodePool("user-np", ptr.To(int32(50)), false),
			},
			wantContains: []string{"user-np", "weight=50", "max weight=0"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, tc.wantWarnEmpty || len(tc.wantContains) > 0,
				"test case must assert on the output (set wantWarnEmpty or wantContains)")

			cli := ctrlfake.NewClientBuilder().
				WithScheme(newKarpenterScheme(t)).
				WithObjects(tc.nodePools...).
				Build()
			streams, _, _, errBuf := genericclioptions.NewTestIOStreams()

			warnKarpenterWeightConflicts(t.Context(), streams, cli)

			out := errBuf.String()
			if tc.wantWarnEmpty {
				assert.Empty(t, out)
			}
			for _, s := range tc.wantContains {
				assert.Contains(t, out, s)
			}
		})
	}
}
