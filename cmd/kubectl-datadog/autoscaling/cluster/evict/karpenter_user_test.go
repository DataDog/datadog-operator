package evict

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEvictKarpenterUserNodePool(t *testing.T) {
	for _, tc := range []struct {
		name              string
		dryRun            bool
		wantUnschedulable bool
	}{
		{name: "cordons and drains", dryRun: false, wantUnschedulable: true},
		{name: "dry-run touches nothing", dryRun: true, wantUnschedulable: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
			client := fake.NewClientset(node)

			require.NoError(t, evictKarpenterUserNodePool(t.Context(), client, "user-np", []string{"n1"}, newDrainOpts(tc.dryRun)))

			got, err := client.CoreV1().Nodes().Get(t.Context(), "n1", metav1.GetOptions{})
			require.NoError(t, err)
			assert.Equal(t, tc.wantUnschedulable, got.Spec.Unschedulable)
		})
	}
}
