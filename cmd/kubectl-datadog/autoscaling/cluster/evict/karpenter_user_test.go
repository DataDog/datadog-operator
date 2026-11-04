package evict

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestEvictKarpenterUserNodePool(t *testing.T) {
	const nodePool, providerID = "user-np", "aws:///eu-west-3a/i-abc"
	newNode := func() *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Spec:       corev1.NodeSpec{ProviderID: providerID},
		}
	}
	newNodeClaim := func() *karpv1.NodeClaim {
		return &karpv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "nc1", Labels: map[string]string{karpv1.NodePoolLabelKey: nodePool}},
			Status:     karpv1.NodeClaimStatus{ProviderID: providerID, NodeName: "n1"},
		}
	}

	t.Run("cordons, drains, then deletes the NodeClaim", func(t *testing.T) {
		cs := fake.NewClientset(newNode())
		ctrlClient := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).WithObjects(newNodeClaim()).Build()

		require.NoError(t, evictKarpenterUserNodePool(t.Context(), cs, ctrlClient, nodePool, []string{"n1"}, newDrainOpts(false)))

		got, err := cs.CoreV1().Nodes().Get(t.Context(), "n1", metav1.GetOptions{})
		require.NoError(t, err)
		assert.True(t, got.Spec.Unschedulable, "the node must be cordoned")

		nc := &karpv1.NodeClaim{}
		err = ctrlClient.Get(t.Context(), client.ObjectKey{Name: "nc1"}, nc)
		assert.True(t, apierrors.IsNotFound(err), "the NodeClaim must be deleted, got err=%v", err)
	})

	t.Run("dry-run touches nothing", func(t *testing.T) {
		cs := fake.NewClientset(newNode())
		ctrlClient := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).WithObjects(newNodeClaim()).Build()

		require.NoError(t, evictKarpenterUserNodePool(t.Context(), cs, ctrlClient, nodePool, []string{"n1"}, newDrainOpts(true)))

		got, err := cs.CoreV1().Nodes().Get(t.Context(), "n1", metav1.GetOptions{})
		require.NoError(t, err)
		assert.False(t, got.Spec.Unschedulable, "dry-run must not cordon")

		nc := &karpv1.NodeClaim{}
		require.NoError(t, ctrlClient.Get(t.Context(), client.ObjectKey{Name: "nc1"}, nc), "dry-run must not delete the NodeClaim")
	})

	t.Run("no matching NodeClaim: drains and warns, no error", func(t *testing.T) {
		cs := fake.NewClientset(newNode())
		// A NodeClaim for a different providerID: nothing matches node n1.
		other := newNodeClaim()
		other.Name = "nc-other"
		other.Status.ProviderID = "aws:///eu-west-3a/i-other"
		ctrlClient := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).WithObjects(other).Build()

		require.NoError(t, evictKarpenterUserNodePool(t.Context(), cs, ctrlClient, nodePool, []string{"n1"}, newDrainOpts(false)))

		nc := &karpv1.NodeClaim{}
		require.NoError(t, ctrlClient.Get(t.Context(), client.ObjectKey{Name: "nc-other"}, nc), "an unrelated NodeClaim must be left alone")
	})

	t.Run("NodeClaim list failure is surfaced; the node is still drained", func(t *testing.T) {
		cs := fake.NewClientset(newNode())
		ctrlClient := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return errors.New("apiserver unreachable")
				},
			}).Build()

		err := evictKarpenterUserNodePool(t.Context(), cs, ctrlClient, nodePool, []string{"n1"}, newDrainOpts(false))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list NodeClaims")

		got, getErr := cs.CoreV1().Nodes().Get(t.Context(), "n1", metav1.GetOptions{})
		require.NoError(t, getErr)
		assert.True(t, got.Spec.Unschedulable, "the node must still be cordoned and drained")
	})

	t.Run("NodeClaim delete failure is surfaced", func(t *testing.T) {
		cs := fake.NewClientset(newNode())
		ctrlClient := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).WithObjects(newNodeClaim()).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
					return apierrors.NewInternalError(errors.New("boom"))
				},
			}).Build()

		err := evictKarpenterUserNodePool(t.Context(), cs, ctrlClient, nodePool, []string{"n1"}, newDrainOpts(false))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete NodeClaim")
	})

	t.Run("drain failure leaves the NodeClaim intact", func(t *testing.T) {
		// A bare pod trips the --force gate, so the drain fails and the node's
		// NodeClaim must NOT be deleted (its workloads are still on it).
		bare := barePod("orphan")
		bare.Spec.NodeName = "n1"
		cs := fake.NewClientset(newNode(), &bare)
		ctrlClient := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).WithObjects(newNodeClaim()).Build()

		err := evictKarpenterUserNodePool(t.Context(), cs, ctrlClient, nodePool, []string{"n1"}, newDrainOpts(false))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "drain node n1")

		nc := &karpv1.NodeClaim{}
		require.NoError(t, ctrlClient.Get(t.Context(), client.ObjectKey{Name: "nc1"}, nc), "the NodeClaim must be left intact when the drain fails")
	})

	t.Run("missing NodeClaim CRD falls back to consolidation", func(t *testing.T) {
		cs := fake.NewClientset(newNode())
		ctrlClient := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "karpenter.sh", Kind: "NodeClaim"}}
				},
			}).Build()

		err := evictKarpenterUserNodePool(t.Context(), cs, ctrlClient, nodePool, []string{"n1"}, newDrainOpts(false))
		require.NoError(t, err, "an absent NodeClaim CRD must not fail the eviction")

		got, getErr := cs.CoreV1().Nodes().Get(t.Context(), "n1", metav1.GetOptions{})
		require.NoError(t, getErr)
		assert.True(t, got.Spec.Unschedulable, "the node is still cordoned and drained")
	})
}
