// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rollout

import "time"

// WorstOf rolls workload results up into a step result (DDAI, DAP or DDA).
// Phase, Reason and Source come from the most severe result (the first on
// ties, except that Settling, nearly done, yields to the other phases of
// its severity); counts are summed; Since is the earliest start not done,
// LastProgress the latest, Deadline the earliest; SettlingUntil is the
// latest when the rolled up phase is Settling; StepOrder is the first set.
// An empty input gives an Unknown result.
func WorstOf(results []Result) Result {
	if len(results) == 0 {
		return Result{Phase: PhaseUnknown}
	}
	out := results[0]
	out.Updated, out.Desired, out.UpdatedReady = 0, 0, 0
	out.Since, out.SinceApprox = time.Time{}, false
	out.LastProgress, out.Deadline, out.SettlingUntil = time.Time{}, time.Time{}, time.Time{}
	for _, r := range results {
		if sev := r.Phase.Severity(); sev > out.Phase.Severity() ||
			(sev == out.Phase.Severity() && out.Phase == PhaseSettling && r.Phase != PhaseSettling) {
			out.Phase, out.Reason, out.Source = r.Phase, r.Reason, r.Source
		}
		out.Updated += r.Updated
		out.Desired += r.Desired
		out.UpdatedReady += r.UpdatedReady
		if r.Phase.Severity() != SeverityDone && !r.Since.IsZero() &&
			(out.Since.IsZero() || r.Since.Before(out.Since)) {
			out.Since, out.SinceApprox = r.Since, r.SinceApprox
		}
		if r.LastProgress.After(out.LastProgress) {
			out.LastProgress = r.LastProgress
		}
		if !r.Deadline.IsZero() && (out.Deadline.IsZero() || r.Deadline.Before(out.Deadline)) {
			out.Deadline = r.Deadline
		}
		if r.SettlingUntil.After(out.SettlingUntil) {
			out.SettlingUntil = r.SettlingUntil
		}
		if out.StepOrder == nil {
			out.StepOrder = r.StepOrder
		}
	}
	if out.Phase != PhaseSettling {
		out.SettlingUntil = time.Time{}
	}
	return out
}
