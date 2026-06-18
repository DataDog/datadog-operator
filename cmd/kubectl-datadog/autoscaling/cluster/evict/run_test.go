package evict

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

// TestReclaimLeakedTempPDBs covers the no-target recovery path: a temp PDB left
// behind by a prior interrupted run is reclaimed only when PDB management is
// enabled and not in dry-run.
func TestReclaimLeakedTempPDBs(t *testing.T) {
	leakedTempPDB := func() *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name: "leaked", Namespace: "default",
				Labels: map[string]string{
					pdbManagedByLabelKey: pdbManagedByLabelValue,
					pdbTempLabelKey:      pdbTempLabelValue,
				},
			},
		}
	}

	for _, tc := range []struct {
		name        string
		ensurePDBs  bool
		dryRun      bool
		wantDeleted bool
	}{
		{name: "enabled deletes leaked temp PDB", ensurePDBs: true, wantDeleted: true},
		{name: "disabled is a no-op", ensurePDBs: false, wantDeleted: false},
		{name: "dry-run does not delete", ensurePDBs: true, dryRun: true, wantDeleted: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).WithObjects(leakedTempPDB()).Build()

			reclaimLeakedTempPDBs(t.Context(), cli, tc.ensurePDBs, tc.dryRun)

			list := &policyv1.PodDisruptionBudgetList{}
			require.NoError(t, cli.List(t.Context(), list))
			if tc.wantDeleted {
				assert.Empty(t, list.Items)
			} else {
				require.Len(t, list.Items, 1)
				assert.Equal(t, "leaked", list.Items[0].Name)
			}
		})
	}

	// A cleanup error must be swallowed (logged, not propagated): the no-op
	// exit must still succeed even if reclaiming the leaked PDBs fails.
	t.Run("cleanup error is swallowed", func(t *testing.T) {
		cli := ctrlfake.NewClientBuilder().
			WithScheme(newCtrlScheme(t)).
			WithObjects(leakedTempPDB()).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(context.Context, ctrlclient.WithWatch, ctrlclient.ObjectList, ...ctrlclient.ListOption) error {
					return errors.New("boom")
				},
			}).
			Build()

		assert.NotPanics(t, func() {
			reclaimLeakedTempPDBs(t.Context(), cli, true, false)
		})
	})
}

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
