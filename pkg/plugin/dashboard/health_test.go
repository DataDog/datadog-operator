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

// TestHealth covers every row of the health model.
func TestHealth(t *testing.T) {
	complete := []rollout.Phase{rollout.PhaseComplete}
	tests := []struct {
		name  string
		facts HealthFacts
		want  Badge
	}{
		{"error condition", HealthFacts{WorstCondition: SeverityError, Phases: complete}, BadgeError},
		{"failed", HealthFacts{Phases: []rollout.Phase{rollout.PhaseRolling, rollout.PhaseFailed}}, BadgeError},
		{"timed out", HealthFacts{Phases: []rollout.Phase{rollout.PhaseTimedOut}}, BadgeError},
		{"rolled back", HealthFacts{Phases: []rollout.Phase{rollout.PhaseRolledBack}}, BadgeError},
		{"error beats degraded", HealthFacts{WorstCondition: SeverityError, Phases: []rollout.Phase{rollout.PhaseStalled}}, BadgeError},
		{"stalled", HealthFacts{Phases: []rollout.Phase{rollout.PhaseComplete, rollout.PhaseStalled}}, BadgeDegraded},
		{"paused", HealthFacts{Phases: []rollout.Phase{rollout.PhasePaused}}, BadgeDegraded},
		{"warning condition", HealthFacts{WorstCondition: SeverityWarning, Phases: complete}, BadgeDegraded},
		{"warning condition without workloads", HealthFacts{WorstCondition: SeverityWarning}, BadgeDegraded},
		{"degraded beats progressing", HealthFacts{WorstCondition: SeverityWarning, Phases: []rollout.Phase{rollout.PhaseRolling}}, BadgeDegraded},
		{"rolling", HealthFacts{Phases: []rollout.Phase{rollout.PhaseComplete, rollout.PhaseRolling}}, BadgeProgressing},
		{"settling", HealthFacts{Phases: []rollout.Phase{rollout.PhaseComplete, rollout.PhaseSettling}}, BadgeProgressing},
		{"pending", HealthFacts{Phases: []rollout.Phase{rollout.PhasePending}}, BadgeProgressing},
		{"baking", HealthFacts{Phases: []rollout.Phase{rollout.PhaseBaking}}, BadgeProgressing},
		{"experiment running", HealthFacts{Experiment: true, Phases: complete}, BadgeProgressing},
		{"progressing beats unknown", HealthFacts{Unknown: true, Phases: []rollout.Phase{rollout.PhaseRolling}}, BadgeProgressing},
		{"complete", HealthFacts{Phases: complete}, BadgeHealthy},
		{"succeeded", HealthFacts{Phases: []rollout.Phase{rollout.PhaseSucceeded, rollout.PhaseComplete}}, BadgeHealthy},
		{"info condition", HealthFacts{WorstCondition: SeverityInfo, Phases: complete}, BadgeHealthy},
		{"missing data", HealthFacts{Unknown: true, Phases: complete}, BadgeUnknown},
		{"no workload", HealthFacts{}, BadgeUnknown},
		{"unknown phase", HealthFacts{Phases: []rollout.Phase{rollout.PhaseComplete, rollout.PhaseUnknown}}, BadgeUnknown},
		{"future phase", HealthFacts{Phases: []rollout.Phase{"Canarying"}}, BadgeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Health(tt.facts))
		})
	}
}

func TestHealthFactsAdd(t *testing.T) {
	f := HealthFacts{WorstCondition: SeverityWarning, Phases: []rollout.Phase{rollout.PhaseComplete}}
	f.Add(HealthFacts{WorstCondition: SeverityInfo, Phases: []rollout.Phase{rollout.PhaseRolling}, Unknown: true})
	f.Add(HealthFacts{Experiment: true})
	assert.Equal(t, HealthFacts{
		WorstCondition: SeverityWarning,
		Phases:         []rollout.Phase{rollout.PhaseComplete, rollout.PhaseRolling},
		Experiment:     true,
		Unknown:        true,
	}, f)
	f.Add(HealthFacts{WorstCondition: SeverityError})
	assert.Equal(t, SeverityError, f.WorstCondition)
}
