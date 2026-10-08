// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"context"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	dash "github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
)

// frame is the immutable result of one build.
type frame struct {
	seq uint64
	// at is the clock of the build.
	at      time.Time
	cluster dash.ClusterInfo
	// sources is the state of every source, also when Build failed.
	sources map[string]dash.SourceInfo
	// loading lists the sources still being read; the view is empty while
	// it is not.
	loading []string
	view    dash.View
	err     error
}

// viewMsg delivers a frame. change is set for the frames of waitForChange,
// which re-arms on them; skipped is set when a tick did not build.
type viewMsg struct {
	frame   *frame
	change  bool
	skipped bool
}

// tickMsg refreshes the time-dependent text.
type tickMsg time.Time

// fatalMsg quits the program with an error.
type fatalMsg struct{ err error }

// builder runs Build off the UI goroutine. Builds are serialized by mu; the
// MemoryObserver is only written by them.
type builder struct {
	store dash.Store
	cfg   dash.BuildConfig
	obs   dash.Observer
	now   func() time.Time
	after func(time.Duration) <-chan time.Time
	// gap is the minimum time between two builds (at most 4/s).
	gap time.Duration

	mu        sync.Mutex
	seq       uint64
	lastStart time.Time
}

// waitForChange blocks until the store changed, waits out the minimum gap
// since the previous build, builds and returns the frame. Changes during
// the wait are coalesced into that build.
func (b *builder) waitForChange(ctx context.Context) tea.Cmd {
	changed := b.store.Changed()
	return guard(func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case <-changed:
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if d := b.lastStart.Add(b.gap).Sub(b.now()); d > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-b.after(d):
			}
		}
		return viewMsg{frame: b.buildLocked(), change: true}
	})
}

// tickBuild rebuilds for the clock (ages, stall thresholds) unless a build
// is in flight or one started less than the gap ago.
func (b *builder) tickBuild() tea.Cmd {
	return guard(func() tea.Msg {
		if !b.mu.TryLock() {
			return viewMsg{skipped: true}
		}
		defer b.mu.Unlock()
		if !b.lastStart.IsZero() && b.now().Sub(b.lastStart) < b.gap {
			return viewMsg{skipped: true}
		}
		return viewMsg{frame: b.buildLocked()}
	})
}

// buildLocked builds a frame from the current snapshot. b.mu must be held.
func (b *builder) buildLocked() *frame {
	now := b.now()
	b.lastStart = now
	b.seq++
	snap := b.store.Snapshot()
	f := &frame{seq: b.seq, at: now, cluster: snap.Cluster, sources: snap.Sources}
	if f.loading = loadingSources(snap, b.cfg.DDAName, b.cfg.Pods); f.loading != nil {
		return f
	}
	f.view, f.err = dash.Build(snap, b.cfg, b.obs, now)
	return f
}

// loadingSources lists the sources the view still waits for: the DDAs,
// then, once a single DDA is selected, its watched sources. Polled sources
// (operator, summary kinds) fill in later without blocking the view.
func loadingSources(s *dash.Snapshot, ddaName string, pods bool) []string {
	info, ok := s.Sources[dash.SourceDDA]
	if !ok || info.State == dash.SourceLoading {
		return []string{dash.SourceDDA}
	}
	if !info.HasData() || selected(s.Objects[dash.SourceDDA], ddaName) != 1 {
		// Build reports the missing or multiple DDAs.
		return nil
	}
	keys := []string{dash.SourceDDAI, dash.SourceDAP, dash.SourceDaemonSets, dash.SourceDeployments, dash.SourceControllerRevisions, dash.SourceReplicaSets}
	if pods {
		keys = append(keys, dash.SourcePods)
	}
	var out []string
	for _, k := range keys {
		if info, ok := s.Sources[k]; !ok || info.State == dash.SourceLoading {
			out = append(out, k)
		}
	}
	return out
}

// selected counts the DDAs matching ddaName, or all of them when it is
// empty.
func selected(ddas []*unstructured.Unstructured, ddaName string) int {
	if ddaName == "" {
		return len(ddas)
	}
	n := 0
	for _, d := range ddas {
		if d.GetName() == ddaName {
			n++
		}
	}
	return n
}

// tick fires after d.
func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// waitForFatal returns the first fatal error.
func waitForFatal(ctx context.Context, f *Fatal) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case err := <-f.ch:
			return fatalMsg{err: err}
		}
	}
}
