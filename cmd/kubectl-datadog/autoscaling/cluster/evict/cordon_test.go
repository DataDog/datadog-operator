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
		dryRun              bool
		// wantUpdateCalls is the minimum number of Update invocations
		// expected on the Nodes endpoint.
		wantMinUpdateCalls int
		wantUnschedulable  bool
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
				return false, nil, nil
			})

			require.NoError(t, cordonNode(t.Context(), client, "n1", tc.dryRun))

			assert.GreaterOrEqual(t, updateCalls, tc.wantMinUpdateCalls, "minimum Update calls")
			got, err := client.CoreV1().Nodes().Get(t.Context(), "n1", metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, tc.wantUnschedulable, got.Spec.Unschedulable)
		})
	}
}
