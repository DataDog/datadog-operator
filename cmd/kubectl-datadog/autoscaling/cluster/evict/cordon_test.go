package evict

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestCordonNode(t *testing.T) {
	for _, tc := range []struct {
		name string
		// initialUnschedulable is the node's starting `Spec.Unschedulable`.
		initialUnschedulable bool
		// conflictFirstUpdate forces a Conflict on the first Update so
		// RetryOnConflict has to refetch and re-apply.
		conflictFirstUpdate bool
		// updateNotFound makes the Update return NotFound, simulating the node
		// being deleted between the Get and the Update.
		updateNotFound bool
		dryRun         bool
		// wantUpdateCalls is the minimum number of Update invocations
		// expected on the Nodes endpoint.
		wantMinUpdateCalls int
		wantUnschedulable  bool
		// wantNilNode expects cordonNode to return a nil Node (the node is gone).
		wantNilNode bool
	}{
		{
			name:               "schedulable node becomes cordoned",
			wantMinUpdateCalls: 1,
			wantUnschedulable:  true,
		},
		{
			// Already cordoned ⇒ no Update issued (idempotent).
			name:                 "already cordoned is a no-op",
			initialUnschedulable: true,
			wantMinUpdateCalls:   0,
			wantUnschedulable:    true,
		},
		{
			name:                "retries on Conflict",
			conflictFirstUpdate: true,
			wantMinUpdateCalls:  2,
			wantUnschedulable:   true,
		},
		{
			name:               "dry-run touches nothing",
			dryRun:             true,
			wantMinUpdateCalls: 0,
			wantUnschedulable:  false,
		},
		{
			// Node deleted between the Get and the Update ⇒ NotFound on Update
			// is a silent success (nothing to schedule onto a deleted node).
			name:               "deleted between get and update is a no-op",
			updateNotFound:     true,
			wantMinUpdateCalls: 1,
			wantUnschedulable:  false,
			wantNilNode:        true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n1"},
				Spec:       corev1.NodeSpec{Unschedulable: tc.initialUnschedulable},
			}
			client := fake.NewClientset(node)

			var updateCalls int
			client.PrependReactor("update", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
				updateCalls++
				if tc.conflictFirstUpdate && updateCalls == 1 {
					return true, nil, apierrors.NewConflict(
						schema.GroupResource{Resource: "nodes"}, "n1", errors.New("forced conflict"),
					)
				}
				if tc.updateNotFound {
					return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, "n1")
				}
				return false, nil, nil
			})

			gotNode, cordonErr := cordonNode(t.Context(), client, "n1", tc.dryRun)
			require.NoError(t, cordonErr)
			assert.Equal(t, tc.wantNilNode, gotNode == nil, "cordonNode nil-node contract")

			assert.GreaterOrEqual(t, updateCalls, tc.wantMinUpdateCalls, "minimum Update calls")
			got, err := client.CoreV1().Nodes().Get(t.Context(), "n1", metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, tc.wantUnschedulable, got.Spec.Unschedulable)
		})
	}
}

// TestCordonNodes covers the up-front cordon pass: a live node is cordoned and
// returned (carrying its state so the caller needs no second Get), an
// already-gone node is a silent skip (no error, absent from the result, so a
// re-run after a partial execution is not derailed), and a node that fails to
// cordon is recorded as an error and excluded so the caller never drains a node
// that can still receive pods.
func TestCordonNodes(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "ip-live"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "ip-bad"}},
	)
	// Updates to ip-bad fail with a non-conflict error so cordonNode gives up
	// immediately (RetryOnConflict only retries Conflict).
	client.PrependReactor("update", "nodes", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ua, ok := action.(clienttesting.UpdateAction)
		if !ok || ua.GetObject().(*corev1.Node).Name != "ip-bad" {
			return false, nil, nil
		}
		return true, nil, apierrors.NewInternalError(errors.New("update boom"))
	})

	// ip-gone is absent from the cluster: cordonNode must treat NotFound as a
	// silent skip.
	cordoned, errs := cordonNodes(t.Context(), client, []string{"ip-live", "ip-gone", "ip-bad"}, false)

	require.Len(t, cordoned, 1, "only the live, successfully-cordoned node is returned")
	assert.Equal(t, "ip-live", cordoned[0].Name)
	assert.True(t, cordoned[0].Spec.Unschedulable, "the returned node carries the cordoned state")
	require.Len(t, errs, 1)
	assert.ErrorContains(t, errs[0], "ip-bad")
}
