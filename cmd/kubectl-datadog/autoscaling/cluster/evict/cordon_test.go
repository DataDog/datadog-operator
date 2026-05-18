package evict

import (
	"context"
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

func TestCordonNode_Schedulable(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	client := fake.NewClientset(node)

	require.NoError(t, cordonNode(context.Background(), client, "n1", false))

	got, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, got.Spec.Unschedulable)
}

func TestCordonNode_AlreadyCordoned_NoUpdate(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Spec:       corev1.NodeSpec{Unschedulable: true},
	}
	client := fake.NewClientset(node)
	var updates int
	client.PrependReactor("update", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
		updates++
		return false, nil, nil
	})

	require.NoError(t, cordonNode(context.Background(), client, "n1", false))
	assert.Zero(t, updates, "an already-cordoned node should not trigger an Update")
}

func TestCordonNode_RetriesOnConflict(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	client := fake.NewClientset(node)
	var calls int
	client.PrependReactor("update", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
		calls++
		if calls == 1 {
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Resource: "nodes"}, "n1", errors.New("forced conflict"),
			)
		}
		return false, nil, nil
	})

	require.NoError(t, cordonNode(context.Background(), client, "n1", false))
	assert.GreaterOrEqual(t, calls, 2, "Update should have been retried after Conflict")

	got, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, got.Spec.Unschedulable)
}

func TestCordonNode_DryRun(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	client := fake.NewClientset(node)
	require.NoError(t, cordonNode(context.Background(), client, "n1", true))
	got, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.False(t, got.Spec.Unschedulable, "dry-run must not mutate the node")
}

