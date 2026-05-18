package evict

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEvictKarpenterUserNodePool_CordonsAndDrains(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	client := fake.NewClientset(node)

	require.NoError(t, evictKarpenterUserNodePool(context.Background(), client, "user-np", []string{"n1"}, newDrainOpts(false)))

	got, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, got.Spec.Unschedulable, "node should be cordoned")
}

func TestEvictKarpenterUserNodePool_DryRun(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	client := fake.NewClientset(node)

	require.NoError(t, evictKarpenterUserNodePool(context.Background(), client, "user-np", []string{"n1"}, newDrainOpts(true)))

	got, err := client.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.False(t, got.Spec.Unschedulable, "dry-run must not cordon")
}
