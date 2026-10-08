// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-operator/pkg/rollout"
)

// Observation is the in-memory history of a workload.
type Observation struct {
	// FirstSeen is when the current revision was first observed.
	FirstSeen time.Time
	// LastProgress is when the updated or available counts last changed.
	LastProgress time.Time
	// ConvergedSince is when every pod was first seen updated for the
	// current revision; zero when it was so from the first observation.
	ConvergedSince time.Time
}

// Observer keeps the observation history of workloads across builds.
// It is only called from Build.
type Observer interface {
	// Observe records the current facts of a workload and returns its
	// history. key identifies the workload across builds.
	Observe(key string, w rollout.Workload, now time.Time) Observation
}

// NopObserver keeps no history, for one-shot snapshots.
type NopObserver struct{}

var _ Observer = NopObserver{}

// Observe implements Observer.
func (NopObserver) Observe(string, rollout.Workload, time.Time) Observation {
	return Observation{}
}

// observerTTL is how long a workload not observed anymore is remembered.
const observerTTL = 10 * time.Minute

// MemoryObserver keeps the observation history of workloads in memory, for
// live mode:
//   - FirstSeen is set when a workload is first observed, and reset when its
//     current revision changes to another known revision. A revision that
//     becomes known after being unresolved keeps FirstSeen.
//   - LastProgress is set only when the updated or available counts change
//     between observations; it is zero until then.
//   - ConvergedSince is set when the workload is seen converged (generation
//     observed, every pod updated) after being seen not converged, or on a
//     revision change; it is cleared when it stops being converged. A
//     workload converged from the first observation keeps it zero: its
//     rollout start bounds the convergence.
//
// It is safe for concurrent use.
type MemoryObserver struct {
	mu        sync.Mutex
	entries   map[string]*observed
	lastPrune time.Time
}

type observed struct {
	revision           string
	updated, available int32
	converged          bool
	firstSeen          time.Time
	lastProgress       time.Time
	convergedSince     time.Time
	lastSeen           time.Time
}

var _ Observer = &MemoryObserver{}

// NewMemoryObserver returns an empty MemoryObserver.
func NewMemoryObserver() *MemoryObserver {
	return &MemoryObserver{entries: map[string]*observed{}}
}

// Observe implements Observer.
func (o *MemoryObserver) Observe(key string, w rollout.Workload, now time.Time) Observation {
	o.mu.Lock()
	defer o.mu.Unlock()
	converged := w.ObservedGeneration >= w.Generation && w.Updated >= w.Desired
	e, ok := o.entries[key]
	switch {
	case !ok:
		e = &observed{revision: w.CurrentRevision, updated: w.Updated, available: w.Available, converged: converged, firstSeen: now}
		o.entries[key] = e
	default:
		newRevision := false
		if w.CurrentRevision != "" && w.CurrentRevision != e.revision {
			if e.revision != "" {
				e.firstSeen = now
				newRevision = true
			}
			e.revision = w.CurrentRevision
		}
		if w.Updated != e.updated || w.Available != e.available {
			e.updated, e.available = w.Updated, w.Available
			e.lastProgress = now
		}
		switch {
		case !converged:
			e.convergedSince = time.Time{}
		case !e.converged || newRevision:
			e.convergedSince = now
		}
		e.converged = converged
	}
	e.lastSeen = now
	o.prune(now)
	return Observation{FirstSeen: e.firstSeen, LastProgress: e.lastProgress, ConvergedSince: e.convergedSince}
}

// prune forgets the workloads not observed for observerTTL, at most once
// per minute.
func (o *MemoryObserver) prune(now time.Time) {
	if now.Sub(o.lastPrune) < time.Minute {
		return
	}
	o.lastPrune = now
	for key, e := range o.entries {
		if now.Sub(e.lastSeen) > observerTTL {
			delete(o.entries, key)
		}
	}
}
