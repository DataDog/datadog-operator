package evict

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

type stubEKS struct {
	gotInputs []*eks.UpdateNodegroupConfigInput
	err       error
}

func (s *stubEKS) UpdateNodegroupConfig(_ context.Context, in *eks.UpdateNodegroupConfigInput, _ ...func(*eks.Options)) (*eks.UpdateNodegroupConfigOutput, error) {
	s.gotInputs = append(s.gotInputs, in)
	return &eks.UpdateNodegroupConfigOutput{}, s.err
}

func mngDrainOpts() nodeDrainOptions {
	return nodeDrainOptions{
		NodeTimeout:  5 * time.Second,
		PollInterval: 10 * time.Millisecond,
	}
}

func TestEvictEKSManagedNodeGroup_DryRun(t *testing.T) {
	stub := &stubEKS{}
	client := fake.NewClientset()
	opts := mngDrainOpts()
	opts.DryRun = true
	require.NoError(t, evictEKSManagedNodeGroup(context.Background(), stub, client, "my-cluster", "my-mng", opts))
	assert.Empty(t, stub.gotInputs, "dry-run must not call UpdateNodegroupConfig")
}

func TestEvictEKSManagedNodeGroup_PropagatesError(t *testing.T) {
	stub := &stubEKS{err: errors.New("api error")}
	client := fake.NewClientset()
	err := evictEKSManagedNodeGroup(context.Background(), stub, client, "my-cluster", "my-mng", mngDrainOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UpdateNodegroupConfig")
}

func TestEvictEKSManagedNodeGroup_WaitsForNodesToDisappear(t *testing.T) {
	stub := &stubEKS{}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ip-1",
			Labels: map[string]string{"eks.amazonaws.com/nodegroup": "my-mng"},
		},
	}
	client := fake.NewClientset(node)
	opts := mngDrainOpts()
	opts.NodeTimeout = 200 * time.Millisecond
	opts.PollInterval = 10 * time.Millisecond

	// Simulate EKS finishing the drain between two polls: the first List
	// observes the node, subsequent Lists return an empty set. Counter-based
	// reactor avoids the time.Sleep + goroutine race the goroutine version
	// had under loaded CI.
	var listCalls int
	client.PrependReactor("list", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
		listCalls++
		if listCalls > 1 {
			return true, &corev1.NodeList{}, nil
		}
		return false, nil, nil
	})

	require.NoError(t, evictEKSManagedNodeGroup(context.Background(), stub, client, "my-cluster", "my-mng", opts))
	require.Len(t, stub.gotInputs, 1)
	in := stub.gotInputs[0]
	assert.Equal(t, "my-cluster", awssdk.ToString(in.ClusterName))
	assert.Equal(t, "my-mng", awssdk.ToString(in.NodegroupName))
	require.NotNil(t, in.ScalingConfig)
	assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.MinSize))
	assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.MaxSize))
	assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.DesiredSize))
}

func TestEvictEKSManagedNodeGroup_TimeoutWrapsSentinel(t *testing.T) {
	stub := &stubEKS{}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ip-1",
			Labels: map[string]string{"eks.amazonaws.com/nodegroup": "my-mng"},
		},
	}
	client := fake.NewClientset(node)
	opts := mngDrainOpts()
	opts.NodeTimeout = 100 * time.Millisecond
	opts.PollInterval = 30 * time.Millisecond

	err := evictEKSManagedNodeGroup(context.Background(), stub, client, "my-cluster", "my-mng", opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, errEKSDrainIncomplete, "wait timeout must wrap errEKSDrainIncomplete so the orchestrator can keep temp PDBs in place")
	assert.Contains(t, err.Error(), "my-mng")
}

// TestEvictEKSManagedNodeGroup_UpdateFailDoesNotWrapSentinel locks in the
// classification: an UpdateNodegroupConfig failure (EKS never started
// draining) must NOT look like a timeout, so the orchestrator can still run
// the temp-PDB cleanup safely.
func TestEvictEKSManagedNodeGroup_UpdateFailDoesNotWrapSentinel(t *testing.T) {
	stub := &stubEKS{err: errors.New("invalid scaling config")}
	client := fake.NewClientset()
	err := evictEKSManagedNodeGroup(context.Background(), stub, client, "my-cluster", "my-mng", mngDrainOpts())
	require.Error(t, err)
	assert.NotErrorIs(t, err, errEKSDrainIncomplete)
}

// TestEvictEKSManagedNodeGroup_ListErrorWrapsSentinel covers the case where
// the EKS scaling change succeeded but the subsequent K8s Node list call
// errors out (transient apiserver issue). Because EKS may still be draining,
// the orchestrator must keep temp PDBs in place — so the error must wrap the
// sentinel.
func TestEvictEKSManagedNodeGroup_ListErrorWrapsSentinel(t *testing.T) {
	stub := &stubEKS{}
	client := fake.NewClientset()
	client.PrependReactor("list", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver unreachable")
	})
	err := evictEKSManagedNodeGroup(context.Background(), stub, client, "my-cluster", "my-mng", mngDrainOpts())
	require.Error(t, err)
	assert.ErrorIs(t, err, errEKSDrainIncomplete, "post-UpdateNodegroupConfig list failure must wrap errEKSDrainIncomplete so cleanup stays paused")
}
