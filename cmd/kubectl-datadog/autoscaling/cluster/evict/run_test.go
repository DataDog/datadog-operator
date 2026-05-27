package evict

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func TestEvictAllTargets(t *testing.T) {
	asgTarget := Target{Manager: clusterinfo.NodeManagerASG, Entity: "asg-1"}
	karpenterTarget := Target{Manager: clusterinfo.NodeManagerKarpenter, Entity: "np-1"}
	eksTarget := Target{Manager: clusterinfo.NodeManagerEKSManagedNodeGroup, Entity: "mng-1"}
	standaloneTarget := Target{Manager: clusterinfo.NodeManagerStandalone, Entity: ""}

	for _, tc := range []struct {
		name              string
		targets           []Target
		evictor           targetEvictor
		wantErrors        int
		wantEKSIncomplete bool
		wantErrIsSentinel bool
		// expectInvoked, when non-nil, lists managers that MUST have had
		// their evictor called (verifies that one failing target does not
		// abort the others).
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
			name:    "EKS sentinel propagates",
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
			// One failing target must not abort the rest — the loop keeps
			// going and every other manager is still invoked.
			name:       "failing target does not abort the others",
			targets:    []Target{asgTarget, karpenterTarget, standaloneTarget},
			evictor:    nil, // installed inline below to capture invocations.
			wantErrors: 1,
			expectInvoked: []clusterinfo.NodeManager{
				clusterinfo.NodeManagerASG,
				clusterinfo.NodeManagerKarpenter,
				clusterinfo.NodeManagerStandalone,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invoked := map[clusterinfo.NodeManager]int{}
			evictor := tc.evictor
			if evictor == nil {
				evictor = func(_ context.Context, target Target, _ nodeDrainOptions) error {
					invoked[target.Manager]++
					if target.Manager == clusterinfo.NodeManagerASG {
						return errors.New("boom")
					}
					return nil
				}
			}

			result := evictAllTargets(t.Context(), tc.targets, nodeDrainOptions{}, evictor)
			require.Len(t, result.Errors, tc.wantErrors)
			assert.Equal(t, tc.wantEKSIncomplete, result.EKSDrainIncomplete)
			if tc.wantErrIsSentinel {
				assert.ErrorIs(t, result.Errors[0], errEKSDrainIncomplete)
			}
			for _, mgr := range tc.expectInvoked {
				assert.NotZero(t, invoked[mgr], "expected %s evictor to be invoked", mgr)
			}
		})
	}
}
