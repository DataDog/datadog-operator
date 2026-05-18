package evict

import (
	"context"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// stubAutoscaling implements AutoscalingAPI by recording each call. Returning
// a non-nil output keeps the production code happy without exercising any AWS
// SDK middleware.
type stubAutoscaling struct {
	scaledTo   map[string]int32 // asg name → desired capacity captured
	scaledASGs []string
	updateErr  error
}

func (s *stubAutoscaling) UpdateAutoScalingGroup(_ context.Context, in *autoscaling.UpdateAutoScalingGroupInput, _ ...func(*autoscaling.Options)) (*autoscaling.UpdateAutoScalingGroupOutput, error) {
	if s.scaledTo == nil {
		s.scaledTo = make(map[string]int32)
	}
	name := awssdk.ToString(in.AutoScalingGroupName)
	s.scaledASGs = append(s.scaledASGs, name)
	if in.DesiredCapacity != nil {
		s.scaledTo[name] = *in.DesiredCapacity
	}
	return &autoscaling.UpdateAutoScalingGroupOutput{}, s.updateErr
}

func newDrainOpts(dryRun bool) nodeDrainOptions {
	return nodeDrainOptions{
		DryRun:          dryRun,
		EvictionTimeout: time.Second,
		NodeTimeout:     time.Second,
		PollInterval:    10 * time.Millisecond,
	}
}

func TestEvictASG_HappyPath(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-0123456789abcdef0"},
	}
	client := fake.NewClientset(node)
	stub := &stubAutoscaling{}

	require.NoError(t, evictASG(context.Background(), client, stub, "my-asg", []string{"ip-1"}, newDrainOpts(false)))

	got, err := client.CoreV1().Nodes().Get(context.Background(), "ip-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, got.Spec.Unschedulable)

	assert.Equal(t, []string{"my-asg"}, stub.scaledASGs)
	assert.Equal(t, int32(0), stub.scaledTo["my-asg"])
}

func TestEvictASG_NodeAlreadyGone_StillScales(t *testing.T) {
	client := fake.NewClientset()
	stub := &stubAutoscaling{}

	require.NoError(t, evictASG(context.Background(), client, stub, "my-asg", []string{"ip-1"}, newDrainOpts(false)))

	assert.Equal(t, []string{"my-asg"}, stub.scaledASGs, "ASG scale-down still happens even with no nodes left")
}

func TestEvictASG_DryRun(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-0123456789abcdef0"},
	}
	client := fake.NewClientset(node)
	stub := &stubAutoscaling{}

	require.NoError(t, evictASG(context.Background(), client, stub, "my-asg", []string{"ip-1"}, newDrainOpts(true)))

	got, err := client.CoreV1().Nodes().Get(context.Background(), "ip-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.False(t, got.Spec.Unschedulable, "dry-run must not mutate the node")
	assert.Empty(t, stub.scaledASGs, "dry-run must not call UpdateAutoScalingGroup")
}

func TestEvictASG_NonEC2ProviderID_DrainsAndScales(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/fargate-ip-10-0-1-2"},
	}
	client := fake.NewClientset(node)
	stub := &stubAutoscaling{}

	require.NoError(t, evictASG(context.Background(), client, stub, "my-asg", []string{"ip-1"}, newDrainOpts(false)))

	assert.Equal(t, []string{"my-asg"}, stub.scaledASGs)
}

// TestEvictASG_DrainFailure_LeavesASGUntouched is the safety regression: when
// a node fails to drain (e.g. a PDB-blocked pod prevents it from emptying),
// the ASG must NOT be scaled to 0. The ASG is left at its current size so a
// re-run can pick up where this one stopped. Single-node setup keeps the
// scenario independent of the fake clientset's FieldSelector handling.
func TestEvictASG_DrainFailure_LeavesASGUntouched(t *testing.T) {
	stuckNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-stuck"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-aaaaaaaaaaaaaaaaa"},
	}
	stuckPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "blocker", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "ip-stuck"},
	}
	client := fake.NewClientset(stuckNode, stuckPod)
	// Eviction reactor returns success but the stuck pod stays in the
	// fake's store, so drainNode hits its NodeTimeout.
	installPodEvictionReactor(client)

	stub := &stubAutoscaling{}
	opts := nodeDrainOptions{
		EvictionTimeout: 50 * time.Millisecond,
		NodeTimeout:     100 * time.Millisecond,
		PollInterval:    10 * time.Millisecond,
	}
	err := evictASG(context.Background(), client, stub, "my-asg", []string{"ip-stuck"}, opts)
	require.Error(t, err, "ASG with a stuck drain should bubble up the drain error")

	assert.Empty(t, stub.scaledASGs, "ASG must not be scaled to 0 when drain failed")
}
