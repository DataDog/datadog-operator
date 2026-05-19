package evict

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func TestEvictAllTargetsParallel(t *testing.T) {
	asgTarget := Target{Manager: clusterinfo.NodeManagerASG, Entity: "asg-1"}
	karpenterTarget := Target{Manager: clusterinfo.NodeManagerKarpenter, Entity: "np-1"}
	eksTarget := Target{Manager: clusterinfo.NodeManagerEKSManagedNodeGroup, Entity: "mng-1"}
	standaloneTarget := Target{Manager: clusterinfo.NodeManagerStandalone, Entity: ""}

	for _, tc := range []struct {
		name    string
		targets []Target
		// evictor is the fake dispatcher. Receives the per-call mutex-tracked
		// invocation counter via the closure pattern below.
		evictor              targetEvictor
		wantErrors           int
		wantEKSIncomplete    bool
		wantErrIsSentinel    bool
		// expectInvoked, when non-nil, lists managers that MUST have had
		// their evictor called (verifies parallelism / independence).
		expectInvoked []clusterinfo.NodeManager
	}{
		{
			name:    "happy path",
			targets: []Target{asgTarget, karpenterTarget},
			evictor: func(_ context.Context, _ Target, _ nodeDrainOptions) error { return nil },
		},
		{
			// Safety-critical signal: an EKS MNG target returning
			// errEKSDrainIncomplete (wrapped) must flag the result so Run
			// skips the temp-PDB cleanup.
			name:    "EKS sentinel propagates through the goroutine + mutex",
			targets: []Target{eksTarget, asgTarget},
			evictor: func(_ context.Context, t Target, _ nodeDrainOptions) error {
				if t.Manager == clusterinfo.NodeManagerEKSManagedNodeGroup {
					return fmt.Errorf("simulated timeout: %w", errEKSDrainIncomplete)
				}
				return nil
			},
			wantErrors:        1,
			wantEKSIncomplete: true,
			wantErrIsSentinel: true,
		},
		{
			// An EKS failure that is NOT a wait timeout (e.g. an
			// UpdateNodegroupConfig API rejection) means EKS never started
			// draining, so temp-PDB cleanup is safe ⇒ flag stays false.
			name:    "non-sentinel EKS error does not flag",
			targets: []Target{eksTarget},
			evictor: func(_ context.Context, _ Target, _ nodeDrainOptions) error {
				return errors.New("invalid scaling config")
			},
			wantErrors: 1,
		},
		{
			// An ASG drain failure has nothing to do with EKS — must NOT
			// flag.
			name:    "non-EKS error does not flag",
			targets: []Target{asgTarget},
			evictor: func(_ context.Context, _ Target, _ nodeDrainOptions) error {
				return errors.New("ASG drain failed")
			},
			wantErrors: 1,
		},
		{
			// One failing type must not abort the others — each manager
			// type goroutine runs to completion regardless of siblings.
			name:    "failing target type does not abort the others",
			targets: []Target{asgTarget, karpenterTarget, standaloneTarget},
			evictor: nil, // installed inline below to capture mu.
			wantErrors: 1,
			expectInvoked: []clusterinfo.NodeManager{
				clusterinfo.NodeManagerASG,
				clusterinfo.NodeManagerKarpenter,
				clusterinfo.NodeManagerStandalone,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invoked := &mapMu{m: map[clusterinfo.NodeManager]int{}}
			evictor := tc.evictor
			if evictor == nil {
				// Default evictor used by the "independence" case: tracks
				// invocations and makes ASG fail.
				evictor = func(_ context.Context, target Target, _ nodeDrainOptions) error {
					invoked.inc(target.Manager)
					if target.Manager == clusterinfo.NodeManagerASG {
						return errors.New("boom")
					}
					return nil
				}
			}

			result := evictAllTargetsParallel(context.Background(), tc.targets, nodeDrainOptions{}, evictor)
			require.Len(t, result.Errors, tc.wantErrors)
			assert.Equal(t, tc.wantEKSIncomplete, result.EKSDrainIncomplete)
			if tc.wantErrIsSentinel {
				assert.ErrorIs(t, result.Errors[0], errEKSDrainIncomplete)
			}
			for _, mgr := range tc.expectInvoked {
				invoked.assertCalled(t, mgr)
			}
		})
	}
}

// mapMu serializes writes to a counter map shared across goroutines.
type mapMu struct {
	mu sync.Mutex
	m  map[clusterinfo.NodeManager]int
}

func (mm *mapMu) inc(k clusterinfo.NodeManager) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.m[k]++
}

func (mm *mapMu) assertCalled(t *testing.T, k clusterinfo.NodeManager) {
	t.Helper()
	mm.mu.Lock()
	defer mm.mu.Unlock()
	assert.NotZero(t, mm.m[k], "expected %s evictor to be invoked", k)
}
