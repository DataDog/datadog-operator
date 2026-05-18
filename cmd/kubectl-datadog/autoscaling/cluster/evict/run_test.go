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

func TestEvictAllTargetsParallel_HappyPath(t *testing.T) {
	targets := []Target{
		{Manager: clusterinfo.NodeManagerASG, Entity: "asg-1"},
		{Manager: clusterinfo.NodeManagerKarpenter, Entity: "np-1"},
	}
	evictor := func(_ context.Context, _ Target, _ nodeDrainOptions) error { return nil }
	result := evictAllTargetsParallel(context.Background(), targets, nodeDrainOptions{}, evictor)
	assert.Empty(t, result.Errors)
	assert.False(t, result.EKSDrainIncomplete)
}

// TestEvictAllTargetsParallel_EKSSentinelFlagsIncomplete locks in the
// safety-critical signal propagation: when an EKS MNG target returns an error
// wrapping errEKSDrainIncomplete, the result must reflect it so that Run skips
// the temp-PDB cleanup. The mutex-protected flag in the worker goroutines is
// exercised end-to-end without requiring real AWS clients.
func TestEvictAllTargetsParallel_EKSSentinelFlagsIncomplete(t *testing.T) {
	targets := []Target{
		{Manager: clusterinfo.NodeManagerEKSManagedNodeGroup, Entity: "mng-1"},
		{Manager: clusterinfo.NodeManagerASG, Entity: "asg-1"},
	}
	evictor := func(_ context.Context, t Target, _ nodeDrainOptions) error {
		if t.Manager == clusterinfo.NodeManagerEKSManagedNodeGroup {
			return fmt.Errorf("simulated timeout: %w", errEKSDrainIncomplete)
		}
		return nil
	}
	result := evictAllTargetsParallel(context.Background(), targets, nodeDrainOptions{}, evictor)
	assert.True(t, result.EKSDrainIncomplete, "errEKSDrainIncomplete from an EKS target must flag the result")
	require.Len(t, result.Errors, 1)
	assert.ErrorIs(t, result.Errors[0], errEKSDrainIncomplete, "sentinel must propagate through the per-target error wrapping")
}

// TestEvictAllTargetsParallel_EKSUpdateErrorDoesNotFlag locks in the
// classification: an EKS target failure that is NOT a wait timeout (e.g. an
// UpdateNodegroupConfig API rejection) means EKS never started draining, so
// temp-PDB cleanup is safe.
func TestEvictAllTargetsParallel_EKSUpdateErrorDoesNotFlag(t *testing.T) {
	targets := []Target{
		{Manager: clusterinfo.NodeManagerEKSManagedNodeGroup, Entity: "mng-1"},
	}
	evictor := func(_ context.Context, _ Target, _ nodeDrainOptions) error {
		return errors.New("invalid scaling config")
	}
	result := evictAllTargetsParallel(context.Background(), targets, nodeDrainOptions{}, evictor)
	assert.False(t, result.EKSDrainIncomplete, "non-sentinel EKS error must NOT flag drain incomplete")
	assert.Len(t, result.Errors, 1)
}

// TestEvictAllTargetsParallel_NonEKSErrorDoesNotFlag — an ASG drain failure
// should not be confused with an EKS drain-in-progress.
func TestEvictAllTargetsParallel_NonEKSErrorDoesNotFlag(t *testing.T) {
	targets := []Target{
		{Manager: clusterinfo.NodeManagerASG, Entity: "asg-1"},
	}
	evictor := func(_ context.Context, _ Target, _ nodeDrainOptions) error {
		return errors.New("ASG drain failed")
	}
	result := evictAllTargetsParallel(context.Background(), targets, nodeDrainOptions{}, evictor)
	assert.False(t, result.EKSDrainIncomplete)
	assert.Len(t, result.Errors, 1)
}

// TestEvictAllTargetsParallel_AllTypesIndependent verifies that a failing
// target type does not abort the others — each manager type's goroutine
// runs to completion regardless of what happens in the others.
func TestEvictAllTargetsParallel_AllTypesIndependent(t *testing.T) {
	targets := []Target{
		{Manager: clusterinfo.NodeManagerASG, Entity: "asg-1"},
		{Manager: clusterinfo.NodeManagerKarpenter, Entity: "np-1"},
		{Manager: clusterinfo.NodeManagerStandalone, Entity: ""},
	}
	mu := &mapMu{m: map[clusterinfo.NodeManager]int{}}
	evictor := func(_ context.Context, t Target, _ nodeDrainOptions) error {
		mu.inc(t.Manager)
		if t.Manager == clusterinfo.NodeManagerASG {
			return errors.New("boom")
		}
		return nil
	}
	result := evictAllTargetsParallel(context.Background(), targets, nodeDrainOptions{}, evictor)
	assert.Len(t, result.Errors, 1)
	mu.assertCalled(t, clusterinfo.NodeManagerASG)
	mu.assertCalled(t, clusterinfo.NodeManagerKarpenter)
	mu.assertCalled(t, clusterinfo.NodeManagerStandalone)
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
