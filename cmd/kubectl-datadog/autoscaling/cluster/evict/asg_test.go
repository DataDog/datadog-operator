package evict

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// stubAutoscaling implements AutoscalingAPI by recording each call. Returning
// a non-nil output keeps the production code happy without exercising any AWS
// SDK middleware.
type stubAutoscaling struct {
	updates              []autoscaling.UpdateAutoScalingGroupInput // captured UpdateAutoScalingGroup inputs, in order
	suspendedAZRebalance []string                                  // asg names for which AZRebalance was suspended
	terminated           []string                                  // instance IDs terminated in-ASG
	suspendErr           error                                     // returned by SuspendProcesses
	prepUpdateErr        error                                     // returned by the MinSize-only prep update
	lockErr              error                                     // returned by the final min=max=desired=0 lock update
	terminateErr         error                                     // returned by TerminateInstanceInAutoScalingGroup
}

func (s *stubAutoscaling) UpdateAutoScalingGroup(_ context.Context, in *autoscaling.UpdateAutoScalingGroupInput, _ ...func(*autoscaling.Options)) (*autoscaling.UpdateAutoScalingGroupOutput, error) {
	s.updates = append(s.updates, *in)
	// The prep update sets MinSize only (MaxSize left nil); the final lock
	// update sets all three fields. Fail the one the test asked for.
	if in.MaxSize == nil {
		return &autoscaling.UpdateAutoScalingGroupOutput{}, s.prepUpdateErr
	}
	return &autoscaling.UpdateAutoScalingGroupOutput{}, s.lockErr
}

func (s *stubAutoscaling) SuspendProcesses(_ context.Context, in *autoscaling.SuspendProcessesInput, _ ...func(*autoscaling.Options)) (*autoscaling.SuspendProcessesOutput, error) {
	if slices.Contains(in.ScalingProcesses, "AZRebalance") {
		s.suspendedAZRebalance = append(s.suspendedAZRebalance, awssdk.ToString(in.AutoScalingGroupName))
	}
	return &autoscaling.SuspendProcessesOutput{}, s.suspendErr
}

func (s *stubAutoscaling) TerminateInstanceInAutoScalingGroup(_ context.Context, in *autoscaling.TerminateInstanceInAutoScalingGroupInput, _ ...func(*autoscaling.Options)) (*autoscaling.TerminateInstanceInAutoScalingGroupOutput, error) {
	s.terminated = append(s.terminated, awssdk.ToString(in.InstanceId))
	return &autoscaling.TerminateInstanceInAutoScalingGroupOutput{}, s.terminateErr
}

// minSizeZeroPrepped reports whether a "prep" update (MinSize=0 only, MaxSize
// left untouched) was issued for asgName.
func (s *stubAutoscaling) minSizeZeroPrepped(asgName string) bool {
	return slices.ContainsFunc(s.updates, func(u autoscaling.UpdateAutoScalingGroupInput) bool {
		return awssdk.ToString(u.AutoScalingGroupName) == asgName &&
			u.MinSize != nil && *u.MinSize == 0 && u.MaxSize == nil
	})
}

// locked reports whether the final min=max=desired=0 update was issued for
// asgName.
func (s *stubAutoscaling) locked(asgName string) bool {
	return slices.ContainsFunc(s.updates, func(u autoscaling.UpdateAutoScalingGroupInput) bool {
		return awssdk.ToString(u.AutoScalingGroupName) == asgName &&
			u.MinSize != nil && *u.MinSize == 0 &&
			u.MaxSize != nil && *u.MaxSize == 0 &&
			u.DesiredCapacity != nil && *u.DesiredCapacity == 0
	})
}

func newDrainOpts(dryRun bool) nodeDrainOptions {
	return nodeDrainOptions{
		DryRun:          dryRun,
		EvictionTimeout: time.Second,
		NodeTimeout:     time.Second,
		PollInterval:    10 * time.Millisecond,
	}
}

// installPodEvictionReactor makes the fake clientset accept Eviction
// subresource creates and echo the eviction object back WITHOUT removing the
// pod from the tracker — so a drain-failure case can keep a pod "stuck" on its
// node past the eviction and drive drainNode to its NodeTimeout.
func installPodEvictionReactor(client *fake.Clientset) {
	client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ca, ok := action.(clienttesting.CreateAction)
		if !ok || ca.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		return true, ca.GetObject(), nil
	})
}

func TestEvictASG(t *testing.T) {
	ec2Node := func() *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
			Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-0123456789abcdef0"},
		}
	}
	fargateNode := func() *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
			Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/fargate-ip-10-0-1-2"},
		}
	}
	stuckNode := func() *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "ip-stuck"},
			Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-aaaaaaaaaaaaaaaaa"},
		}
	}
	stuckPod := func() *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "blocker", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "ip-stuck"},
		}
	}
	fastDrain := nodeDrainOptions{
		EvictionTimeout: 50 * time.Millisecond,
		NodeTimeout:     100 * time.Millisecond,
		PollInterval:    10 * time.Millisecond,
	}

	for _, tc := range []struct {
		name string
		// objects pre-populate the fake clientset.
		objects []runtime.Object
		// installEvictionReactor wires a 200-OK Eviction subresource reactor
		// that keeps the pod alive in the store (so drainNode hits its node
		// timeout). Used only by the drain-failure case.
		installEvictionReactor bool
		// nodes is the slice passed to evictASG.
		nodes []string
		opts  nodeDrainOptions
		// terminateErr / lockErr inject AWS failures into the per-instance
		// termination and the final lock update respectively.
		terminateErr error
		lockErr      error
		// wantErr=true requires evictASG to return a non-nil error.
		wantErr bool
		// wantPrepped: the ASG was prepped for termination (AZRebalance
		// suspended + MinSize=0).
		wantPrepped bool
		// wantTerminated is the expected set of instance IDs terminated via
		// TerminateInstanceInAutoScalingGroup. nil ⇒ none.
		wantTerminated []string
		// wantLocked: the final min=max=desired=0 update was issued.
		wantLocked bool
		// wantUnschedulable: per-node expected `Spec.Unschedulable`. Nodes
		// absent from the map (or absent from the cluster) aren't checked.
		wantUnschedulable map[string]bool
	}{
		{
			name:              "happy path",
			objects:           []runtime.Object{ec2Node()},
			nodes:             []string{"ip-1"},
			opts:              newDrainOpts(false),
			wantPrepped:       true,
			wantTerminated:    []string{"i-0123456789abcdef0"},
			wantLocked:        true,
			wantUnschedulable: map[string]bool{"ip-1": true},
		},
		{
			// ASG neutralization still happens even when the requested K8s
			// node is already gone — the operator may be re-running after a
			// partial earlier execution and the AWS-side scale-to-zero must
			// still land. No per-instance termination: the node (hence its
			// instance ID) is gone.
			name:        "node already gone",
			nodes:       []string{"ip-1"},
			opts:        newDrainOpts(false),
			wantPrepped: true,
			wantLocked:  true,
		},
		{
			name:              "dry-run mutates nothing",
			objects:           []runtime.Object{ec2Node()},
			nodes:             []string{"ip-1"},
			opts:              newDrainOpts(true),
			wantUnschedulable: map[string]bool{"ip-1": false},
			// wantPrepped/wantLocked false, wantTerminated nil ⇒ no AWS
			// mutation must happen.
		},
		{
			// A node with a non-EC2 providerID (e.g. Fargate-ish) yields no
			// instance ID: it still drains, but per-instance termination is
			// skipped and the instance is cleaned up by the final
			// scale-to-zero instead.
			name:              "non-EC2 providerID drains, skips per-instance terminate, still locks",
			objects:           []runtime.Object{fargateNode()},
			nodes:             []string{"ip-1"},
			opts:              newDrainOpts(false),
			wantPrepped:       true,
			wantLocked:        true,
			wantUnschedulable: map[string]bool{"ip-1": true},
		},
		{
			// Safety regression: a node failing to drain (here a PDB-blocked
			// pod stays in the fake store past the eviction reactor) must
			// leave the instance running and the ASG unlocked (MaxSize not
			// forced to 0) so a re-run can pick up where this one stopped.
			// The up-front prep (MinSize=0, AZRebalance suspended) has
			// happened and is idempotent on re-run.
			name:                   "drain failure leaves ASG unlocked",
			objects:                []runtime.Object{stuckNode(), stuckPod()},
			installEvictionReactor: true,
			nodes:                  []string{"ip-stuck"},
			opts:                   fastDrain,
			wantErr:                true,
			wantPrepped:            true,
			wantLocked:             false,
		},
		{
			// A per-instance termination failure must propagate and, like a
			// drain failure, leave the ASG unlocked for a re-run.
			name:           "terminate failure leaves ASG unlocked",
			objects:        []runtime.Object{ec2Node()},
			nodes:          []string{"ip-1"},
			opts:           newDrainOpts(false),
			terminateErr:   errTerminate,
			wantErr:        true,
			wantPrepped:    true,
			wantTerminated: []string{"i-0123456789abcdef0"},
			wantLocked:     false,
		},
		{
			// A failure of the final lock update must surface as an error
			// (the lock call was still attempted).
			name:           "final lock failure surfaces error",
			objects:        []runtime.Object{ec2Node()},
			nodes:          []string{"ip-1"},
			opts:           newDrainOpts(false),
			lockErr:        errLock,
			wantErr:        true,
			wantPrepped:    true,
			wantTerminated: []string{"i-0123456789abcdef0"},
			wantLocked:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientset(tc.objects...)
			if tc.installEvictionReactor {
				installPodEvictionReactor(client)
			}
			stub := &stubAutoscaling{terminateErr: tc.terminateErr, lockErr: tc.lockErr}

			err := evictASG(t.Context(), client, stub, "my-asg", tc.nodes, tc.opts)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tc.wantPrepped {
				assert.Equal(t, []string{"my-asg"}, stub.suspendedAZRebalance, "AZRebalance must be suspended once")
				assert.True(t, stub.minSizeZeroPrepped("my-asg"), "MinSize=0 prep update must be issued")
			} else {
				assert.Empty(t, stub.suspendedAZRebalance, "AZRebalance must not be suspended")
				assert.Empty(t, stub.updates, "no UpdateAutoScalingGroup call must happen")
			}

			if tc.wantTerminated == nil {
				assert.Empty(t, stub.terminated, "no instance must be terminated")
			} else {
				assert.ElementsMatch(t, tc.wantTerminated, stub.terminated)
			}

			assert.Equal(t, tc.wantLocked, stub.locked("my-asg"), "final scale-to-zero")

			for nodeName, want := range tc.wantUnschedulable {
				got, getErr := client.CoreV1().Nodes().Get(t.Context(), nodeName, metav1.GetOptions{})
				require.NoError(t, getErr)
				assert.Equal(t, want, got.Spec.Unschedulable, "Spec.Unschedulable for %s", nodeName)
			}
		})
	}
}

var (
	errSuspend   = errors.New("suspend boom")
	errPrepMin   = errors.New("min-size boom")
	errTerminate = errors.New("terminate boom")
	errLock      = errors.New("lock boom")
)

// TestEvictASGCordonFailure verifies that when a node cannot be cordoned, it is
// left undrained and the ASG is NOT locked at MaxSize=0 — a re-run must be able
// to finish the job. The up-front prep (AZRebalance suspend + MinSize=0) still
// ran and is idempotent on the re-run.
func TestEvictASGCordonFailure(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-0123456789abcdef0"},
	}
	client := fake.NewClientset(node)
	// Every node Update fails with a non-conflict error so the cordon gives up.
	client.PrependReactor("update", "nodes", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("cordon boom"))
	})
	stub := &stubAutoscaling{}

	err := evictASG(t.Context(), client, stub, "my-asg", []string{"ip-1"}, newDrainOpts(false))
	require.Error(t, err)

	// Prep precedes the cordon, so it ran...
	assert.Equal(t, []string{"my-asg"}, stub.suspendedAZRebalance, "AZRebalance suspended up front")
	assert.True(t, stub.minSizeZeroPrepped("my-asg"), "MinSize=0 prep update issued up front")
	// ...but the un-cordoned node was never drained or terminated, and the ASG
	// stays unlocked so a re-run can finish.
	assert.Empty(t, stub.terminated, "no instance terminated when the node failed to cordon")
	assert.False(t, stub.locked("my-asg"), "ASG must not be locked when a node failed to cordon")
}

// TestEvictASGPrepFailure covers the up-front prepareASGForTermination failure
// branches: when either the AZRebalance suspend or the MinSize=0 update fails,
// evictASG must abort before touching any node (no cordon, no drain, no
// termination, no lock) so a re-run can start cleanly.
func TestEvictASGPrepFailure(t *testing.T) {
	ec2Node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///eu-west-3a/i-0123456789abcdef0"},
	}

	for _, tc := range []struct {
		name          string
		suspendErr    error
		prepUpdateErr error
		// wantMinSizeUpdate: the MinSize=0 prep update was attempted (only
		// reached when the suspend succeeded).
		wantMinSizeUpdate bool
	}{
		{
			name:       "suspend failure aborts before draining",
			suspendErr: errSuspend,
		},
		{
			name:              "min-size update failure aborts before draining",
			prepUpdateErr:     errPrepMin,
			wantMinSizeUpdate: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientset(ec2Node)
			stub := &stubAutoscaling{suspendErr: tc.suspendErr, prepUpdateErr: tc.prepUpdateErr}

			err := evictASG(t.Context(), client, stub, "my-asg", []string{"ip-1"}, newDrainOpts(false))
			require.Error(t, err)

			// The drain loop must never have run: the node stays schedulable
			// and no instance is terminated or locked.
			assert.Empty(t, stub.terminated, "no instance must be terminated")
			assert.False(t, stub.locked("my-asg"), "ASG must not be locked")
			assert.Equal(t, tc.wantMinSizeUpdate, stub.minSizeZeroPrepped("my-asg"), "MinSize=0 prep update attempted")

			got, getErr := client.CoreV1().Nodes().Get(t.Context(), "ip-1", metav1.GetOptions{})
			require.NoError(t, getErr)
			assert.False(t, got.Spec.Unschedulable, "node must not be cordoned when prep fails")
		})
	}
}
