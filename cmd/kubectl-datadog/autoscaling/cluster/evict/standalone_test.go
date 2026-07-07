package evict

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

type stubEC2 struct {
	terminated []string
	err        error
}

func (s *stubEC2) TerminateInstances(_ context.Context, in *ec2.TerminateInstancesInput, _ ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	s.terminated = append(s.terminated, in.InstanceIds...)
	return &ec2.TerminateInstancesOutput{}, s.err
}

func TestEvictStandalone(t *testing.T) {
	ec2Node := func(name, az, id string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       corev1.NodeSpec{ProviderID: "aws:///" + az + "/" + id},
		}
	}
	stuckPod := func() *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "blocker", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "ip-stuck"},
		}
	}
	gceNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ip-1"},
		Spec:       corev1.NodeSpec{ProviderID: "gce:///zone/instance"},
	}
	fastDrain := nodeDrainOptions{
		EvictionTimeout: 50 * time.Millisecond,
		NodeTimeout:     100 * time.Millisecond,
		PollInterval:    10 * time.Millisecond,
	}

	for _, tc := range []struct {
		name                   string
		objects                []runtime.Object
		installEvictionReactor bool
		nodes                  []string
		opts                   nodeDrainOptions
		// stubErr, when set, makes every TerminateInstances call fail.
		stubErr error
		wantErr bool
		// wantTerminated is the expected set of instance IDs in
		// stubEC2.terminated. nil ⇒ TerminateInstances must not be called.
		wantTerminated []string
		// wantUnschedulable: per-node expected `Spec.Unschedulable`.
		wantUnschedulable map[string]bool
	}{
		{
			name:              "happy path terminates and cordons two nodes",
			objects:           []runtime.Object{ec2Node("ip-1", "eu-west-3a", "i-aaa"), ec2Node("ip-2", "eu-west-3b", "i-bbb")},
			nodes:             []string{"ip-1", "ip-2"},
			opts:              newDrainOpts(false),
			wantTerminated:    []string{"i-aaa", "i-bbb"},
			wantUnschedulable: map[string]bool{"ip-1": true, "ip-2": true},
		},
		{
			name:              "dry-run touches nothing",
			objects:           []runtime.Object{ec2Node("ip-1", "eu-west-3a", "i-aaa")},
			nodes:             []string{"ip-1"},
			opts:              newDrainOpts(true),
			wantUnschedulable: map[string]bool{"ip-1": false},
		},
		{
			// Node already gone from K8s ⇒ no instance ID extracted ⇒
			// no TerminateInstances call.
			name:  "node already gone skips terminate",
			nodes: []string{"ip-1"},
			opts:  newDrainOpts(false),
		},
		{
			// Non-EC2 providerID can't yield an instance ID, but the
			// node is still cordoned and drained.
			name:              "non-EC2 providerID skips terminate but still cordons",
			objects:           []runtime.Object{gceNode},
			nodes:             []string{"ip-1"},
			opts:              newDrainOpts(false),
			wantUnschedulable: map[string]bool{"ip-1": true},
		},
		{
			// Safety regression: drain failure must prevent the
			// instance from being terminated (otherwise the EC2 dies
			// mid-grace-period).
			name:                   "drain failure prevents terminate",
			objects:                []runtime.Object{ec2Node("ip-stuck", "eu-west-3a", "i-aaaaaaaaaaaaaaaaa"), stuckPod()},
			installEvictionReactor: true,
			nodes:                  []string{"ip-stuck"},
			opts:                   fastDrain,
			wantErr:                true,
		},
		{
			// A TerminateInstances failure on one node must propagate as an
			// error yet not stop the loop: every drained node's instance is
			// still attempted.
			name:           "terminate failure propagates but loop continues",
			objects:        []runtime.Object{ec2Node("ip-1", "eu-west-3a", "i-aaa"), ec2Node("ip-2", "eu-west-3b", "i-bbb")},
			nodes:          []string{"ip-1", "ip-2"},
			opts:           newDrainOpts(false),
			stubErr:        errors.New("terminate boom"),
			wantErr:        true,
			wantTerminated: []string{"i-aaa", "i-bbb"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientset(tc.objects...)
			if tc.installEvictionReactor {
				installPodEvictionReactor(client)
			}
			stub := &stubEC2{err: tc.stubErr}

			err := evictStandalone(t.Context(), client, stub, tc.nodes, tc.opts)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.wantTerminated == nil {
				assert.Empty(t, stub.terminated)
			} else {
				assert.ElementsMatch(t, tc.wantTerminated, stub.terminated)
			}
			for nodeName, want := range tc.wantUnschedulable {
				got, getErr := client.CoreV1().Nodes().Get(t.Context(), nodeName, metav1.GetOptions{})
				require.NoError(t, getErr)
				assert.Equal(t, want, got.Spec.Unschedulable, "Spec.Unschedulable for %s", nodeName)
			}
		})
	}
}

// TestEvictStandaloneCordonsAllBeforeDraining locks in the cordon-all-up-front
// behavior: every node of the group is cordoned before ANY node starts
// draining, so a pod evicted off one node is never rescheduled onto a sibling
// node that is itself about to be drained. It asserts that at the moment ip-1's
// pod is evicted, the sibling ip-2 has already been cordoned.
func TestEvictStandaloneCordonsAllBeforeDraining(t *testing.T) {
	ec2Node := func(name, az, id string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       corev1.NodeSpec{ProviderID: "aws:///" + az + "/" + id},
		}
	}
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "ip-1"},
	}
	client := fake.NewClientset(ec2Node("ip-1", "eu-west-3a", "i-aaa"), ec2Node("ip-2", "eu-west-3b", "i-bbb"), pod1)

	var (
		recorded                     bool
		node2CordonedAtFirstEviction bool
	)
	// On the first pod eviction (ip-1's drain), record whether ip-2 is already
	// cordoned, then delete the evicted pod so the node becomes empty and the
	// drain completes. Read/mutate via the tracker, not the typed client:
	// Fake.Invokes holds a global lock for the whole reaction, so re-entering
	// the typed client from inside a reactor would deadlock.
	client.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ca, ok := action.(clienttesting.CreateAction)
		if !ok || ca.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		if !recorded {
			recorded = true
			if obj, err := client.Tracker().Get(corev1.SchemeGroupVersion.WithResource("nodes"), "", "ip-2"); err == nil {
				node2CordonedAtFirstEviction = obj.(*corev1.Node).Spec.Unschedulable
			}
		}
		evic := ca.GetObject().(*policyv1.Eviction)
		_ = client.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), evic.Namespace, evic.Name)
		return true, ca.GetObject(), nil
	})

	stub := &stubEC2{}
	err := evictStandalone(t.Context(), client, stub, []string{"ip-1", "ip-2"}, newDrainOpts(false))
	require.NoError(t, err)
	assert.True(t, node2CordonedAtFirstEviction, "ip-2 must be cordoned before ip-1 starts draining")
	assert.ElementsMatch(t, []string{"i-aaa", "i-bbb"}, stub.terminated)
}
