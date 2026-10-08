// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/DataDog/datadog-operator/pkg/rollout"
)

// ProfileSort is the order of the profile blocks (--sort).
type ProfileSort string

const (
	// SortName orders the profiles by name, the default.
	SortName ProfileSort = "name"
	// SortDesired orders the profiles by agent DaemonSet desired pods,
	// most first; ties keep the name order and profiles without a
	// DaemonSet come last.
	SortDesired ProfileSort = "desired"
)

// ProfileSorts are the valid --sort values.
var ProfileSorts = []ProfileSort{SortName, SortDesired}

// ParseProfileSort validates a --sort value.
func ParseProfileSort(s string) (ProfileSort, error) {
	if !slices.Contains(ProfileSorts, ProfileSort(s)) {
		return "", fmt.Errorf("unsupported profile sort %q: use %q or %q", s, SortName, SortDesired)
	}
	return ProfileSort(s), nil
}

// arrangeProfiles orders the profiles, given in name order, and drops the
// empty ones when cfg.HideEmpty or cfg.HideEmptyForce is set. It returns
// the shown and the hidden profiles, both in display order. It runs once health is computed.
func arrangeProfiles(daps []*DAPView, cfg BuildConfig) (shown, hidden []*DAPView) {
	shown = make([]*DAPView, 0, len(daps))
	for _, d := range daps {
		if (cfg.HideEmptyForce && forceEmptyProfile(d)) || (cfg.HideEmpty && emptyProfile(d)) {
			hidden = append(hidden, d)
			continue
		}
		shown = append(shown, d)
	}
	if cfg.Sort == SortDesired {
		slices.SortStableFunc(shown, func(a, b *DAPView) int {
			da, okA := profileDesired(a)
			db, okB := profileDesired(b)
			if c := cmp.Compare(boolRank(!okA), boolRank(!okB)); c != 0 {
				return c
			}
			return cmp.Compare(db, da)
		})
	}
	return shown, hidden
}

// emptyProfile reports whether a profile can be hidden by --hide-empty: its
// workloads have no desired pods (see zeroDesired) and the whole block is
// Healthy. A profile with an issue, a rollout in progress, no status, no
// DDAI or no DaemonSet yet is never empty.
func emptyProfile(d *DAPView) bool {
	return !d.StatusUnknown && d.Health == BadgeHealthy && zeroDesired(d) && d.DDAI.Health == BadgeHealthy
}

// forceEmptyProfile reports whether a profile can be hidden by
// --hide-empty-force, whatever its conditions and badge: it has a status,
// its workloads have no desired pods (see zeroDesired) and the DDAI rollout
// is done (Complete: every workload observed its generation). A profile
// with no status, no DDAI, no DaemonSet yet or a rollout in progress is
// never hidden.
func forceEmptyProfile(d *DAPView) bool {
	return !d.StatusUnknown && zeroDesired(d) && rollout.Phase(d.DDAI.Rollout.Phase).Severity() == rollout.SeverityDone
}

// zeroDesired reports whether a profile has a DDAI whose agent DaemonSet is
// found with 0 desired pods and whose other workloads, if any, are found
// with 0 desired pods too.
func zeroDesired(d *DAPView) bool {
	if desired, ok := profileDesired(d); !ok || desired != 0 {
		return false
	}
	for _, w := range []*WorkloadView{d.DDAI.ClusterAgent, d.DDAI.ClusterChecksRunner, d.DDAI.OtelAgentGateway} {
		if w != nil && (w.Missing || w.Counts.Desired != 0) {
			return false
		}
	}
	return true
}

// hideProfileIssues removes from issues the Error and Warning conditions of
// the hidden profiles: those reported by the DAP, its DDAI or its
// workloads. It returns the remaining issues, the number of hidden
// profiles that had at least one issue and the number of issues removed.
func hideProfileIssues(issues []Cond, hidden []*DAPView) (kept []Cond, withIssues, removed int) {
	owner := map[string]int{}
	for i, d := range hidden {
		for _, c := range profileConds(d) {
			owner[c.Object.String()] = i
		}
	}
	had := make([]bool, len(hidden))
	kept = make([]Cond, 0, len(issues))
	for _, c := range issues {
		if i, ok := owner[c.Object.String()]; ok {
			had[i] = true
			removed++
			continue
		}
		kept = append(kept, c)
	}
	for _, h := range had {
		if h {
			withIssues++
		}
	}
	return kept, withIssues, removed
}

// profileConds lists the conditions of a profile block: the DAP, its DDAI
// and the DDAI workloads.
func profileConds(d *DAPView) []Cond {
	conds := slices.Clone(d.Conditions)
	if d.DDAI == nil {
		return conds
	}
	conds = append(conds, d.DDAI.Conditions...)
	for _, w := range []*WorkloadView{d.DDAI.Agent, d.DDAI.ClusterAgent, d.DDAI.ClusterChecksRunner, d.DDAI.OtelAgentGateway} {
		if w != nil {
			conds = append(conds, w.Conditions...)
		}
	}
	return conds
}

// profileDesired is the desired pod count of the profile agent DaemonSet;
// false when the profile has no DDAI or the DaemonSet is not found.
func profileDesired(d *DAPView) (int32, bool) {
	if d.DDAI == nil || d.DDAI.Agent == nil || d.DDAI.Agent.Missing {
		return 0, false
	}
	return d.DDAI.Agent.Counts.Desired, true
}
