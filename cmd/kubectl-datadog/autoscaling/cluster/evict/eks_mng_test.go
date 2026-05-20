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

		wantUpdateCallCount int
		wantErr             bool
		// wantWrapsSentinel asserts errors.Is(err, errEKSDrainIncomplete).
		wantWrapsSentinel bool
		// wantNotWrapsSentinel asserts errors.Is(err, errEKSDrainIncomplete) is false.
		wantNotWrapsSentinel bool
		wantErrContains      string
	}{
		{
			name:                "dry-run skips Update",
			dryRun:              true,
			wantUpdateCallCount: 0,
		},
		{
			name:                 "UpdateNodegroupConfig failure does not wrap sentinel",
			updateErr:            apiErr,
			wantUpdateCallCount:  1,
			wantErr:              true,
			wantErrContains:      "UpdateNodegroupConfig",
			wantNotWrapsSentinel: true,
		},
		{
			// EKS finishes draining between two polls: first List sees the
			// node, second returns empty. Counter-based reactor avoids the
			// time.Sleep + goroutine race the goroutine version had under
			// loaded CI.
			name:  "waits for nodes to disappear",
			nodes: []runtime.Object{mngNode},
			listResponder: func(call int) (bool, runtime.Object, error) {
				if call > 1 {
					return true, &corev1.NodeList{}, nil
				}
				return false, nil, nil
			},
			wantUpdateCallCount: 1,
		},
		{
			// Wait timeout while nodes persist: orchestrator must keep
			// temp PDBs in place ⇒ sentinel wrapped.
			name:                "wait timeout wraps the sentinel",
			nodes:               []runtime.Object{mngNode},
			shortTimeout:        true,
			wantUpdateCallCount: 1,
			wantErr:             true,
			wantErrContains:     "my-mng",
			wantWrapsSentinel:   true,
		},
		{
			// EKS scaling change succeeded, but a subsequent K8s Node list
			// call errors out. EKS may still be draining ⇒ sentinel wrapped.
			name:                "post-Update list failure wraps the sentinel",
			listResponder:       func(_ int) (bool, runtime.Object, error) { return true, nil, listErr },
			wantUpdateCallCount: 1,
			wantErr:             true,
			wantWrapsSentinel:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubEKS{err: tc.updateErr}
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
			require.Len(t, stub.gotInputs, tc.wantUpdateCallCount)
			for _, in := range stub.gotInputs {
				assert.Equal(t, "my-cluster", awssdk.ToString(in.ClusterName))
				assert.Equal(t, "my-mng", awssdk.ToString(in.NodegroupName))
				require.NotNil(t, in.ScalingConfig)
				assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.MinSize))
				assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.MaxSize))
				assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.DesiredSize))
			}
		})
	}
}
