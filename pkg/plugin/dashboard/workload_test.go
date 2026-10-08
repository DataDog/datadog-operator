// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/pkg/rollout"
)

// The percentage counts updated and ready pods.
func TestRolloutViewPercent(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		updated, updatedReady, desired int32
		want                           int
	}{
		{"all updated, some not ready", 10, 6, 10, 60},
		{"complete", 10, 10, 10, 100},
		{"rounds down", 3, 2, 3, 66},
		{"nothing updated", 0, 0, 4, 0},
		{"nothing to roll", 0, 0, 0, 100},
		{"surge is clamped", 5, 5, 4, 100},
		{"negative is clamped", 0, -1, 4, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ro := rolloutView(rollout.Result{Phase: rollout.PhaseRolling, Updated: tc.updated, UpdatedReady: tc.updatedReady, Desired: tc.desired})
			assert.Equal(t, tc.want, ro.Percent)
			assert.Equal(t, tc.updated, ro.Updated, "updated count unchanged")
			assert.Equal(t, PercentBasisUpdatedReady, ro.PercentBasis)
		})
	}

	// Without pods, updatedReady is the lower bound updated - unavailable.
	in := rollout.Input{
		Workload: rollout.Workload{Kind: rollout.KindDaemonSet, Desired: 10, Updated: 10, Available: 6, Unavailable: 4},
		Now:      testNow,
	}
	res, _ := rollout.HeuristicEvaluator{}.Evaluate(in)
	ro := rolloutView(res)
	assert.Equal(t, int32(6), ro.UpdatedReady)
	assert.Equal(t, 60, ro.Percent)
}
