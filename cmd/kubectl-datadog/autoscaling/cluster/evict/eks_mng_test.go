package evict

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

type stubEKS struct {
	gotInputs      []*eks.UpdateNodegroupConfigInput
	err            error
	describedMax   int32 // 0 ⇒ stub returns nil ScalingConfig
	describeErr    error
	describeCalled int
}

func (s *stubEKS) DescribeNodegroup(_ context.Context, in *eks.DescribeNodegroupInput, _ ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error) {
	s.describeCalled++
	if s.describeErr != nil {
		return nil, s.describeErr
	}
	out := &eks.DescribeNodegroupOutput{
		Nodegroup: &ekstypes.Nodegroup{
			ClusterName:   in.ClusterName,
			NodegroupName: in.NodegroupName,
		},
	}
	if s.describedMax > 0 {
		out.Nodegroup.ScalingConfig = &ekstypes.NodegroupScalingConfig{
			MaxSize: awssdk.Int32(s.describedMax),
		}
	}
	return out, nil
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

func TestEvictEKSManagedNodeGroup(t *testing.T) {
	mngNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ip-1",
			Labels: map[string]string{"eks.amazonaws.com/nodegroup": "my-mng"},
		},
	}
	apiErr := errors.New("api error")
	listErr := errors.New("apiserver unreachable")

	for _, tc := range []struct {
		name string
		// updateErr is what stubEKS returns from UpdateNodegroupConfig. nil
		// ⇒ success.
		updateErr error
		// describeErr is what stubEKS returns from DescribeNodegroup. nil
		// ⇒ success.
		describeErr error
		// describedMax is the MaxSize reported by the stubbed
		// DescribeNodegroup. 0 ⇒ stub returns nil ScalingConfig (simulating
		// a node group described without a scaling config).
		describedMax int32
		// nodes are pre-loaded into the fake clientset (i.e. nodes that EKS
		// would be draining).
		nodes []runtime.Object
		// listResponder, when non-nil, overrides the default "list nodes"
		// behaviour. Used to simulate the EKS-side drain finishing between
		// polls, or a transient List error.
		listResponder func(call int) (handle bool, resp runtime.Object, err error)
		dryRun        bool
		// shortTimeout uses a 100ms NodeTimeout to force the wait loop to
		// give up; default keeps the longer mngDrainOpts() value.
		shortTimeout bool

		wantDescribeCallCount int
		wantUpdateCallCount   int
		// wantUpdateMaxSize is the MaxSize the test asserts on the
		// UpdateNodegroupConfig input. Only checked when
		// wantUpdateCallCount > 0.
		wantUpdateMaxSize int32
		wantErr           bool
		// wantWrapsSentinel asserts errors.Is(err, errEKSDrainIncomplete).
		wantWrapsSentinel bool
		// wantNotWrapsSentinel asserts errors.Is(err, errEKSDrainIncomplete) is false.
		wantNotWrapsSentinel bool
		wantErrContains      string
	}{
		{
			name:                  "dry-run skips Describe + Update",
			dryRun:                true,
			wantDescribeCallCount: 0,
			wantUpdateCallCount:   0,
		},
		{
			name:                  "DescribeNodegroup failure short-circuits",
			describeErr:           apiErr,
			wantDescribeCallCount: 1,
			wantUpdateCallCount:   0,
			wantErr:               true,
			wantErrContains:       "DescribeNodegroup",
			wantNotWrapsSentinel:  true,
		},
		{
			name:                  "UpdateNodegroupConfig failure does not wrap sentinel",
			describedMax:          5,
			updateErr:             apiErr,
			wantDescribeCallCount: 1,
			wantUpdateCallCount:   1,
			wantUpdateMaxSize:     5,
			wantErr:               true,
			wantErrContains:       "UpdateNodegroupConfig",
			wantNotWrapsSentinel:  true,
		},
		{
			// EKS finishes draining between two polls: first List sees the
			// node, second returns empty. Counter-based reactor avoids the
			// time.Sleep + goroutine race the goroutine version had under
			// loaded CI.
			name:         "waits for nodes to disappear",
			nodes:        []runtime.Object{mngNode},
			describedMax: 5,
			listResponder: func(call int) (bool, runtime.Object, error) {
				if call > 1 {
					return true, &corev1.NodeList{}, nil
				}
				return false, nil, nil
			},
			wantDescribeCallCount: 1,
			wantUpdateCallCount:   1,
			wantUpdateMaxSize:     5,
		},
		{
			// Described scaling config is nil ⇒ Update must still pass an
			// API-valid MaxSize (>= 1). Ensures evict never blindly sends 0.
			name:                  "describe without scaling config falls back to max=1",
			nodes:                 []runtime.Object{mngNode},
			describedMax:          0,
			shortTimeout:          true,
			wantDescribeCallCount: 1,
			wantUpdateCallCount:   1,
			wantUpdateMaxSize:     1,
			wantErr:               true,
			wantErrContains:       "my-mng",
			wantWrapsSentinel:     true,
		},
		{
			// Wait timeout while nodes persist: orchestrator must keep
			// temp PDBs in place ⇒ sentinel wrapped.
			name:                  "wait timeout wraps the sentinel",
			nodes:                 []runtime.Object{mngNode},
			describedMax:          5,
			shortTimeout:          true,
			wantDescribeCallCount: 1,
			wantUpdateCallCount:   1,
			wantUpdateMaxSize:     5,
			wantErr:               true,
			wantErrContains:       "my-mng",
			wantWrapsSentinel:     true,
		},
		{
			// EKS scaling change succeeded, but a subsequent K8s Node list
			// call errors out. EKS may still be draining ⇒ sentinel wrapped.
			name:                  "post-Update list failure wraps the sentinel",
			describedMax:          5,
			listResponder:         func(_ int) (bool, runtime.Object, error) { return true, nil, listErr },
			wantDescribeCallCount: 1,
			wantUpdateCallCount:   1,
			wantUpdateMaxSize:     5,
			wantErr:               true,
			wantWrapsSentinel:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubEKS{
				err:          tc.updateErr,
				describeErr:  tc.describeErr,
				describedMax: tc.describedMax,
			}
			client := fake.NewClientset(tc.nodes...)
			if tc.listResponder != nil {
				var listCalls int
				client.PrependReactor("list", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
					listCalls++
					return tc.listResponder(listCalls)
				})
			}
			opts := mngDrainOpts()
			opts.DryRun = tc.dryRun
			if tc.shortTimeout {
				opts.NodeTimeout = 100 * time.Millisecond
				opts.PollInterval = 30 * time.Millisecond
			}

			err := evictEKSManagedNodeGroup(context.Background(), stub, client, "my-cluster", "my-mng", opts)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrContains != "" {
					assert.Contains(t, err.Error(), tc.wantErrContains)
				}
			} else {
				require.NoError(t, err)
			}
			if tc.wantWrapsSentinel {
				assert.ErrorIs(t, err, errEKSDrainIncomplete)
			}
			if tc.wantNotWrapsSentinel {
				assert.NotErrorIs(t, err, errEKSDrainIncomplete)
			}
			assert.Equal(t, tc.wantDescribeCallCount, stub.describeCalled)
			require.Len(t, stub.gotInputs, tc.wantUpdateCallCount)
			for _, in := range stub.gotInputs {
				assert.Equal(t, "my-cluster", awssdk.ToString(in.ClusterName))
				assert.Equal(t, "my-mng", awssdk.ToString(in.NodegroupName))
				require.NotNil(t, in.ScalingConfig)
				assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.MinSize))
				assert.Equal(t, tc.wantUpdateMaxSize, awssdk.ToInt32(in.ScalingConfig.MaxSize))
				assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.DesiredSize))
			}
		})
	}
}
