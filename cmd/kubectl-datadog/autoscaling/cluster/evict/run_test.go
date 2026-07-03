package evict

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func TestEvictAllTargets(t *testing.T) {
	asgTarget := Target{Manager: clusterinfo.NodeManagerASG, Entity: "asg-1"}
	karpenterTarget := Target{Manager: clusterinfo.NodeManagerKarpenter, Entity: "np-1"}
	standaloneTarget := Target{Manager: clusterinfo.NodeManagerStandalone, Entity: ""}

	for _, tc := range []struct {
		name       string
		targets    []Target
		evictor    targetEvictor
		wantErrors int
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

			errs := evictAllTargets(t.Context(), tc.targets, nodeDrainOptions{}, evictor)
			require.Len(t, errs, tc.wantErrors)
			for _, mgr := range tc.expectInvoked {
				assert.NotZero(t, invoked[mgr], "expected %s evictor to be invoked", mgr)
			}
		})
	}
}
