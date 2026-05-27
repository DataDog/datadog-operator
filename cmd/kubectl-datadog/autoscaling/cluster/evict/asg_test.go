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
	"k8s.io/apimachinery/pkg/runtime"
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
		// wantErr=true requires evictASG to return a non-nil error.
		wantErr bool
		// wantScaledASGs is the expected `stubAutoscaling.scaledASGs`. nil ⇒
		// no UpdateAutoScalingGroup call should have happened.
		wantScaledASGs []string
		// wantUnschedulable: per-node expected `Spec.Unschedulable`. Nodes
		// absent from the map (or absent from the cluster) aren't checked.
		wantUnschedulable map[string]bool
	}{
		{
			name:              "happy path",
			objects:           []runtime.Object{ec2Node()},
			nodes:             []string{"ip-1"},
			opts:              newDrainOpts(false),
			wantScaledASGs:    []string{"my-asg"},
			wantUnschedulable: map[string]bool{"ip-1": true},
		},
		{
			// ASG scale-down still happens even when the requested K8s node
			// is already gone — the operator may be re-running after a
			// partial earlier execution and the AWS-side neutralization
			// must still land.
			name:           "node already gone",
			nodes:          []string{"ip-1"},
			opts:           newDrainOpts(false),
			wantScaledASGs: []string{"my-asg"},
		},
		{
			name:              "dry-run mutates nothing",
			objects:           []runtime.Object{ec2Node()},
			nodes:             []string{"ip-1"},
			opts:              newDrainOpts(true),
			wantUnschedulable: map[string]bool{"ip-1": false},
			// wantScaledASGs left nil ⇒ UpdateAutoScalingGroup must not be
			// called.
		},
		{
			// evictASG doesn't inspect the providerID anymore, but verify
			// it copes with a non-EC2 shape (e.g. Fargate-ish) without
			// crashing — useful as a regression guard.
			name:           "non-EC2 providerID still drains and scales",
			objects:        []runtime.Object{fargateNode()},
			nodes:          []string{"ip-1"},
			opts:           newDrainOpts(false),
			wantScaledASGs: []string{"my-asg"},
		},
		{
			// Safety regression: a node failing to drain (here a
			// PDB-blocked pod stays in the fake store past the eviction
			// reactor) must leave the ASG untouched so a re-run can pick
			// up where this one stopped.
			name:                   "drain failure leaves ASG untouched",
			objects:                []runtime.Object{stuckNode(), stuckPod()},
			installEvictionReactor: true,
			nodes:                  []string{"ip-stuck"},
			opts:                   fastDrain,
			wantErr:                true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientset(tc.objects...)
			if tc.installEvictionReactor {
				installPodEvictionReactor(client)
			}
			stub := &stubAutoscaling{}

			err := evictASG(t.Context(), client, stub, "my-asg", tc.nodes, tc.opts)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tc.wantScaledASGs == nil {
				assert.Empty(t, stub.scaledASGs, "ASG must not be scaled in this scenario")
			} else {
				assert.Equal(t, tc.wantScaledASGs, stub.scaledASGs)
				for _, asgName := range tc.wantScaledASGs {
					assert.Equal(t, int32(0), stub.scaledTo[asgName], "desired capacity for %s", asgName)
				}
			}

			for nodeName, want := range tc.wantUnschedulable {
				got, getErr := client.CoreV1().Nodes().Get(t.Context(), nodeName, metav1.GetOptions{})
				require.NoError(t, getErr)
				assert.Equal(t, want, got.Spec.Unschedulable, "Spec.Unschedulable for %s", nodeName)
			}
		})
	}
}
