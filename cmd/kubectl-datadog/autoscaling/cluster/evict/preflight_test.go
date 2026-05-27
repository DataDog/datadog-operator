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

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
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

func TestWarnKarpenterWeightConflicts(t *testing.T) {
	for _, tc := range []struct {
		name      string
		nodePools []ctrlclient.Object
		targets   []Target
		// wantContains lists substrings that MUST appear in the stderr output.
		wantContains []string
		// wantWarnEmpty asserts the stderr output is empty (no warning fired).
		wantWarnEmpty bool
	}{
		{
			// No Karpenter target ⇒ nothing to warn about.
			name: "no karpenter targets ⇒ no warning",
			targets: []Target{
				{Manager: clusterinfo.NodeManagerASG, Entity: "asg-1"},
				{Manager: clusterinfo.NodeManagerStandalone, Entity: ""},
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
			targets:       []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "user-np"}},
			wantWarnEmpty: true,
		},
		{
			// User NP weight > Datadog NP weight ⇒ warns (`>=` check).
			name: "user weight above Datadog weight warns",
			nodePools: []ctrlclient.Object{
				mkNodePool("dd-np", ptr.To(int32(10)), true),
				mkNodePool("user-np", ptr.To(int32(50)), false),
			},
			targets:      []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "user-np"}},
			wantContains: []string{"user-np", "weight=50"},
		},
		{
			// Both nil ⇒ both default to 0 ⇒ equal-weight conflict warns.
			name: "nil weights both default to 0 (equal-weight conflict)",
			nodePools: []ctrlclient.Object{
				mkNodePool("dd-np", nil, true),
				mkNodePool("user-np", nil, false),
			},
			targets:      []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "user-np"}},
			wantContains: []string{"user-np"},
		},
		{
			// Target name doesn't match any NodePool in the cluster ⇒
			// nothing to compare against, no panic.
			name: "target not in cluster is ignored",
			nodePools: []ctrlclient.Object{
				mkNodePool("dd-np", ptr.To(int32(100)), true),
			},
			targets:       []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "ghost-np"}},
			wantWarnEmpty: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := ctrlfake.NewClientBuilder().
				WithScheme(newKarpenterScheme(t)).
				WithObjects(tc.nodePools...).
				Build()
			streams, _, _, errBuf := genericclioptions.NewTestIOStreams()

			warnKarpenterWeightConflicts(t.Context(), streams, cli, tc.targets)

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

// TestRunPreflightWarnings_NoOp keeps the thin `runPreflightWarnings` wrapper
// exercised separately so coverage doesn't regress as new pre-flight checks
// are added.
func TestRunPreflightWarnings_NoOp(t *testing.T) {
	cli := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).Build()
	streams, _, _, errBuf := genericclioptions.NewTestIOStreams()
	runPreflightWarnings(t.Context(), streams, cli, nil)
	assert.Empty(t, errBuf.String())
}
