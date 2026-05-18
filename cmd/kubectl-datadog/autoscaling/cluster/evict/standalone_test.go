package evict

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type stubEC2 struct {
	terminated []string
	err        error
}

func (s *stubEC2) TerminateInstances(_ context.Context, in *ec2.TerminateInstancesInput, _ ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	s.terminated = append(s.terminated, in.InstanceIds...)
	return &ec2.TerminateInstancesOutput{}, s.err
}

func TestEvictStandalone_HappyPath(t *testing.T) {
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-aaa"},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-2"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3b/i-bbb"},
	}
	client := fake.NewClientset(node1, node2)
	stub := &stubEC2{}

	require.NoError(t, evictStandalone(context.Background(), client, stub, []string{"ip-1", "ip-2"}, newDrainOpts(false)))

	assert.ElementsMatch(t, []string{"i-aaa", "i-bbb"}, stub.terminated)

	for _, name := range []string{"ip-1", "ip-2"} {
		got, err := client.CoreV1().Nodes().Get(context.Background(), name, metav1.GetOptions{})
		require.NoError(t, err)
		assert.True(t, got.Spec.Unschedulable, "node %s should be cordoned", name)
	}
}

func TestEvictStandalone_DryRun(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-aaa"},
	}
	client := fake.NewClientset(node)
	stub := &stubEC2{}

	require.NoError(t, evictStandalone(context.Background(), client, stub, []string{"ip-1"}, newDrainOpts(true)))

	assert.Empty(t, stub.terminated, "dry-run must not call TerminateInstances")
	got, err := client.CoreV1().Nodes().Get(context.Background(), "ip-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.False(t, got.Spec.Unschedulable, "dry-run must not cordon")
}

func TestEvictStandalone_NodeGone_SkipsTerminate(t *testing.T) {
	client := fake.NewClientset() // no nodes
	stub := &stubEC2{}

	require.NoError(t, evictStandalone(context.Background(), client, stub, []string{"ip-1"}, newDrainOpts(false)))

	assert.Empty(t, stub.terminated, "no instance id known → no terminate call")
}

func TestEvictStandalone_NonEC2ProviderID(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
		Spec:       corev1.NodeSpec{ProviderID: "gce:///zone/instance"},
	}
	client := fake.NewClientset(node)
	stub := &stubEC2{}

	require.NoError(t, evictStandalone(context.Background(), client, stub, []string{"ip-1"}, newDrainOpts(false)))

	assert.Empty(t, stub.terminated, "non-EC2 providerID skips terminate")
	got, err := client.CoreV1().Nodes().Get(context.Background(), "ip-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, got.Spec.Unschedulable, "non-EC2 node should still be cordoned")
}

// TestEvictStandalone_DrainFailure_NoTerminate is the safety regression for
// standalone: a node whose drain fails must NOT have its instance terminated.
// Single-node setup avoids the fake clientset's FieldSelector limitation.
func TestEvictStandalone_DrainFailure_NoTerminate(t *testing.T) {
	stuckNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-stuck"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-aaaaaaaaaaaaaaaaa"},
	}
	stuckPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "blocker", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "ip-stuck"},
	}
	client := fake.NewClientset(stuckNode, stuckPod)
	installPodEvictionReactor(client)

	stub := &stubEC2{}
	opts := nodeDrainOptions{
		EvictionTimeout: 50 * time.Millisecond,
		NodeTimeout:     100 * time.Millisecond,
		PollInterval:    10 * time.Millisecond,
	}
	err := evictStandalone(context.Background(), client, stub, []string{"ip-stuck"}, opts)
	require.Error(t, err)
	assert.Empty(t, stub.terminated, "drain failure must prevent TerminateInstances on the affected node")
}

