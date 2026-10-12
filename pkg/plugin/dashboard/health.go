// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import "github.com/DataDog/datadog-operator/pkg/rollout"

// HealthFacts are the facts of a subtree the health badge is computed from.
// They only come from evaluator results and classified conditions; the
// rollout state is never re-derived here.
type HealthFacts struct {
	// WorstCondition is the most severe condition severity in the subtree,
	// including conditions collapsed by dedup; empty when none.
	WorstCondition Severity
	// Phases are the rollout phases of the subtree's workloads.
	Phases []rollout.Phase
	// Experiment is true when a Fleet experiment is running.
	Experiment bool
	// Unknown is true when some data is missing: a source not synced or not
	// accessible, or an object with no status yet.
	Unknown bool
}

// Add merges the facts of a child subtree.
func (f *HealthFacts) Add(o HealthFacts) {
	f.addSeverity(o.WorstCondition)
	f.Phases = append(f.Phases, o.Phases...)
	f.Experiment = f.Experiment || o.Experiment
	f.Unknown = f.Unknown || o.Unknown
}

func (f *HealthFacts) addSeverity(s Severity) {
	if s != "" && (f.WorstCondition == "" || s.rank() > f.WorstCondition.rank()) {
		f.WorstCondition = s
	}
}

// Health computes the health badge of a subtree. The first matching rule
// wins:
//
//	Error       an Error condition, or a phase of Error severity
//	            (Failed, TimedOut, RolledBack)
//	Degraded    a phase of Degraded severity (Stalled, Paused), or a Warning
//	            condition, e.g. unavailable pods above the threshold after
//	            settling
//	Progressing a phase of Progressing severity (Rolling, Settling,
//	            Pending, Baking), or a running experiment
//	Unknown     missing data, no workload, or an unknown phase
//	Healthy     otherwise: every workload Complete/Succeeded and ready
func Health(f HealthFacts) Badge {
	worst := rollout.SeverityDone
	unknownPhase := false
	for _, p := range f.Phases {
		s := p.Severity()
		if s > worst {
			worst = s
		}
		if s == rollout.SeverityUnknown {
			unknownPhase = true
		}
	}
	switch {
	case f.WorstCondition == SeverityError || worst == rollout.SeverityError:
		return BadgeError
	case worst == rollout.SeverityDegraded || f.WorstCondition == SeverityWarning:
		return BadgeDegraded
	case worst == rollout.SeverityProgressing || f.Experiment:
		return BadgeProgressing
	case f.Unknown || unknownPhase || len(f.Phases) == 0:
		return BadgeUnknown
	default:
		return BadgeHealthy
	}
}
