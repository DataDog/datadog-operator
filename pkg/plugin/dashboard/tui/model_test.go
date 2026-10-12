// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dash "github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
)

var testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// fakeStore serves the snapshot set by the test and signals Changed.
type fakeStore struct {
	mu      sync.Mutex
	snap    *dash.Snapshot
	changed chan struct{}
	reads   atomic.Int32
}

func newFakeStore(s *dash.Snapshot) *fakeStore {
	f := &fakeStore{changed: make(chan struct{}, 1)}
	f.set(s)
	return f
}

func (f *fakeStore) set(s *dash.Snapshot) {
	f.mu.Lock()
	f.snap = s
	f.mu.Unlock()
	select {
	case f.changed <- struct{}{}:
	default:
	}
}

func (f *fakeStore) Start(context.Context) error { return nil }
func (f *fakeStore) Changed() <-chan struct{}    { return f.changed }
func (f *fakeStore) Close()                      {}
func (f *fakeStore) Snapshot() *dash.Snapshot {
	f.reads.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap
}

// fixture loads a scenario of the dashboard testdata.
func fixture(t *testing.T, name string) *dash.Snapshot {
	t.Helper()
	s, err := dash.LoadFixture(filepath.Join("..", "testdata", name))
	require.NoError(t, err)
	return s
}

// withSources returns a copy of s with some source states replaced.
func withSources(s *dash.Snapshot, mut func(map[string]dash.SourceInfo)) *dash.Snapshot {
	c := *s
	c.Sources = maps.Clone(s.Sources)
	mut(c.Sources)
	return &c
}

// fakeClock is a settable clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// testModel returns a model over store, sized 120x40, with a fake clock.
// Waits on the clock return at once and record the duration.
func testModel(t *testing.T, store dash.Store, cfg Config) (*model, *fakeClock, *[]time.Duration) {
	t.Helper()
	clock := &fakeClock{t: testNow}
	var waits []time.Duration
	cfg.Store = store
	cfg.Now = clock.Now
	cfg.After = func(d time.Duration) <-chan time.Time {
		waits = append(waits, d)
		clock.Add(d)
		ch := make(chan time.Time, 1)
		ch <- clock.Now()
		return ch
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	_, m, stop := newProgram(ctx, cfg)
	t.Cleanup(stop)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m, clock, &waits
}

// update applies msg and returns the command's message, if any.
func update(t *testing.T, m *model, msg tea.Msg) tea.Cmd {
	t.Helper()
	_, cmd := m.Update(msg)
	return cmd
}

// changeFrame runs waitForChange once and applies its frame.
func changeFrame(t *testing.T, m *model) {
	t.Helper()
	msg := m.b.waitForChange(m.ctx)()
	require.IsType(t, viewMsg{}, msg)
	require.True(t, msg.(viewMsg).change)
	update(t, m, msg)
}

func TestModelLoadingThenView(t *testing.T) {
	steady := fixture(t, "steady")
	loading := withSources(steady, func(s map[string]dash.SourceInfo) {
		s[dash.SourceDaemonSets] = dash.SourceInfo{State: dash.SourceLoading, Mode: dash.RefreshWatch}
		delete(s, dash.SourceDeployments)
	})
	store := newFakeStore(withSources(steady, func(s map[string]dash.SourceInfo) {
		s[dash.SourceDDA] = dash.SourceInfo{State: dash.SourceLoading, Mode: dash.RefreshWatch}
	}))
	m, _, _ := testModel(t, store, Config{})

	assert.Contains(t, m.content, "loading")
	changeFrame(t, m)
	assert.Contains(t, m.content, "waiting for dda")
	assert.NotContains(t, m.content, "agents")

	store.set(loading)
	changeFrame(t, m)
	assert.Contains(t, m.content, "waiting for ds, deploy")
	assert.NotContains(t, m.content, "agents", "never shows counts before the sync")
	assert.NotContains(t, m.content, "DatadogAgent datadog/")

	store.set(steady)
	changeFrame(t, m)
	assert.Contains(t, m.content, "live")
	assert.Contains(t, m.content, "DatadogAgent datadog/datadog")
	assert.Contains(t, m.content, "agents 3/3 ready")
	assert.Contains(t, m.content, "q quit")
	lines := strings.Split(m.content, "\n")
	assert.Len(t, lines, 40)
	assert.Contains(t, lines[39], "updated+ready", "the legend is in the footer")
	assert.Equal(t, 1, strings.Count(m.content, "updated, not ready"), "the body has no legend")
}

// The footer has the bar legend only when the body draws a rollout bar.
func TestModelLegendOnlyWithBars(t *testing.T) {
	steady := fixture(t, "steady")
	noWorkloads := *withSources(steady, func(s map[string]dash.SourceInfo) {
		s[dash.SourceDaemonSets] = dash.SourceInfo{State: dash.SourceForbidden, Mode: dash.RefreshWatch, Err: "forbidden"}
		s[dash.SourceDeployments] = dash.SourceInfo{State: dash.SourceForbidden, Mode: dash.RefreshWatch, Err: "forbidden"}
	})
	noWorkloads.Objects = maps.Clone(steady.Objects)
	delete(noWorkloads.Objects, dash.SourceDaemonSets)
	delete(noWorkloads.Objects, dash.SourceDeployments)
	m, _, _ := testModel(t, newFakeStore(&noWorkloads), Config{})
	changeFrame(t, m)
	require.Contains(t, m.content, "DatadogAgent datadog/datadog")
	lines := strings.Split(m.content, "\n")
	assert.NotContains(t, lines[len(lines)-1], "updated+ready")
	assert.Contains(t, lines[len(lines)-1], "q quit")
}

func TestModelCoalescesChanges(t *testing.T) {
	steady := fixture(t, "steady")
	store := newFakeStore(steady)
	m, clock, waits := testModel(t, store, Config{})

	// The first build runs at once.
	changeFrame(t, m)
	assert.Empty(t, *waits)
	assert.Equal(t, int32(1), store.reads.Load())
	first := m.frame.seq

	// A burst of changes 100 ms later is one build after the 250 ms gap.
	clock.Add(100 * time.Millisecond)
	for range 100 {
		store.set(steady)
	}
	changeFrame(t, m)
	assert.Equal(t, []time.Duration{150 * time.Millisecond}, *waits)
	assert.Equal(t, int32(2), store.reads.Load())
	assert.Equal(t, first+1, m.frame.seq)
	assert.Equal(t, testNow.Add(250*time.Millisecond), m.frame.at)

	// Nothing is pending after the burst.
	select {
	case <-store.Changed():
		t.Fatal("a change is still pending")
	default:
	}

	// A change long after the gap builds without waiting.
	clock.Add(time.Second)
	store.set(steady)
	changeFrame(t, m)
	assert.Len(t, *waits, 1)
}

func TestModelStaleFrameIgnored(t *testing.T) {
	store := newFakeStore(fixture(t, "steady"))
	m, _, _ := testModel(t, store, Config{})
	changeFrame(t, m)
	cur := m.frame
	update(t, m, viewMsg{frame: &frame{seq: cur.seq - 1}})
	assert.Same(t, cur, m.frame)
}

func TestModelTick(t *testing.T) {
	steady := fixture(t, "steady")
	store := newFakeStore(steady)
	m, clock, _ := testModel(t, store, Config{})
	changeFrame(t, m)
	reads := store.reads.Load()
	assert.Contains(t, m.content, "last change 0s ago")

	clock.Add(5 * time.Second)
	cmd := update(t, m, tickMsg(clock.Now()))
	require.NotNil(t, cmd)
	assert.True(t, m.tickBusy)
	assert.Contains(t, m.content, "last change 5s ago", "the tick redraws the ages at once")

	// A second tick does not start another build while one is in flight.
	m.Update(tickMsg(clock.Now()))
	assert.True(t, m.tickBusy)

	// The tick build rebuilds from the store.
	msg := m.b.tickBuild()()
	require.IsType(t, viewMsg{}, msg)
	require.NotNil(t, msg.(viewMsg).frame)
	update(t, m, msg)
	assert.False(t, m.tickBusy)
	assert.Equal(t, reads+1, store.reads.Load())
	assert.Equal(t, clock.Now(), m.frame.at)

	// A tick build is skipped while another build holds the builder, and
	// right after a build.
	m.b.mu.Lock()
	assert.True(t, m.b.tickBuild()().(viewMsg).skipped)
	m.b.mu.Unlock()
	assert.True(t, m.b.tickBuild()().(viewMsg).skipped)
	clock.Add(time.Second)
	assert.False(t, m.b.tickBuild()().(viewMsg).skipped)
}

func TestModelPollCountdown(t *testing.T) {
	steady := fixture(t, "steady")
	snap := withSources(steady, func(s map[string]dash.SourceInfo) {
		s[dash.SourceOperator] = dash.SourceInfo{State: dash.SourceOK, Mode: dash.RefreshPoll, LastOK: testNow, NextPoll: testNow.Add(30 * time.Second)}
	})
	m, clock, _ := testModel(t, newFakeStore(snap), Config{})
	changeFrame(t, m)
	assert.Contains(t, m.content, "next poll in 30s")
	clock.Add(10 * time.Second)
	m.Update(tickMsg(clock.Now()))
	assert.Contains(t, m.content, "next poll in 20s")
	clock.Add(25 * time.Second)
	m.Update(tickMsg(clock.Now()))
	assert.Contains(t, m.content, "polling…", "a late poll is not a negative countdown")
}

func TestModelTooSmall(t *testing.T) {
	m, _, _ := testModel(t, newFakeStore(fixture(t, "steady")), Config{})
	changeFrame(t, m)
	for _, size := range [][2]int{{79, 40}, {120, 19}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		assert.Contains(t, m.content, "terminal too small")
		assert.NotContains(t, m.content, "DatadogAgent")
		for _, l := range strings.Split(m.content, "\n") {
			assert.LessOrEqual(t, len([]rune(l)), size[0])
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	assert.NotContains(t, m.content, "terminal too small")
	lines := strings.Split(m.content, "\n")
	assert.Len(t, lines, 20)
	assert.Contains(t, lines[19], "scroll 1-18/", "a tall body scrolls")
	assert.Contains(t, lines[19], "q quit")
}

// The body scrolls between the fixed status line and footer, and keeps its
// position across rebuilds.
func TestModelScroll(t *testing.T) {
	store := newFakeStore(fixture(t, "profiles"))
	m, _, _ := testModel(t, store, Config{})
	changeFrame(t, m)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	total := len(strings.Split(m.content, "\n"))
	require.Equal(t, 20, total)

	footer := func() string { l := strings.Split(m.content, "\n"); return l[len(l)-1] }
	body := func() []string { l := strings.Split(m.content, "\n"); return l[1 : len(l)-1] }
	var n int
	_, err := fmt.Sscanf(footer()[strings.Index(footer(), "scroll 1-18/"):], "scroll 1-18/%d", &n)
	require.NoError(t, err)
	require.Greater(t, n, 18, "the profiles body overflows 18 lines")
	assert.Contains(t, m.content, "live", "the status line stays")

	first := body()
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Contains(t, footer(), fmt.Sprintf("scroll 2-19/%d", n))
	assert.Equal(t, first[1], body()[0], "one line down")

	m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	assert.Contains(t, footer(), fmt.Sprintf("scroll 1-18/%d", n))
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Contains(t, footer(), fmt.Sprintf("scroll 1-18/%d", n), "the top is a bound")

	m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	assert.Contains(t, footer(), fmt.Sprintf("scroll %d-%d/%d", n-17, n, n))
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Contains(t, footer(), fmt.Sprintf("scroll %d-%d/%d", n-17, n, n), "the bottom is a bound")

	m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	assert.Contains(t, footer(), fmt.Sprintf("scroll %d-", min(19, n-17)))
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	assert.Contains(t, footer(), fmt.Sprintf("scroll 1-18/%d", n))
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeySpace, Text: " "}, {Code: 'f', Text: "f"}} {
		m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
		m.Update(k)
		assert.NotContains(t, footer(), "scroll 1-18/", "%s pages down", k.String())
	}
	m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	assert.Contains(t, footer(), fmt.Sprintf("scroll 1-18/%d", n))

	// A rebuild keeps the position.
	m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	store.set(fixture(t, "profiles"))
	changeFrame(t, m)
	assert.Contains(t, footer(), fmt.Sprintf("scroll 2-19/%d", n))

	// A terminal tall enough shows everything, with no position.
	m.Update(tea.WindowSizeMsg{Width: 120, Height: n + 2})
	assert.NotContains(t, footer(), "scroll")
	assert.Equal(t, first[0], body()[0])
}

func TestModelQuitKeys(t *testing.T) {
	m, _, _ := testModel(t, newFakeStore(fixture(t, "steady")), Config{})
	for _, k := range []tea.KeyPressMsg{{Code: 'q', Text: "q"}, {Code: 'c', Mod: tea.ModCtrl}} {
		cmd := update(t, m, k)
		require.NotNil(t, cmd, k.String())
		assert.IsType(t, tea.QuitMsg{}, cmd(), k.String())
	}
	assert.Nil(t, update(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"}), "other keys do nothing")
}

func TestModelDisconnected(t *testing.T) {
	steady := fixture(t, "steady")
	snap := withSources(steady, func(s map[string]dash.SourceInfo) {
		for _, k := range []string{dash.SourceDaemonSets, dash.SourceDDAI, dash.SourceDDA} {
			info := s[k]
			info.State, info.Mode, info.Err = dash.SourceDisconnected, dash.RefreshWatch, "connection refused"
			info.LastOK = testNow.Add(-40 * time.Second)
			s[k] = info
		}
		for _, k := range []string{dash.SourceDeployments} {
			info := s[k]
			info.Mode, info.LastOK = dash.RefreshWatch, testNow.Add(-40*time.Second)
			s[k] = info
		}
	})
	m, _, _ := testModel(t, newFakeStore(snap), Config{})
	changeFrame(t, m)
	status := strings.Split(m.content, "\n")[0]
	assert.Equal(t, "⚠ disconnected, reconnecting (3 sources) · last change 40s ago · dda, ddai, ds", status)
	assert.Contains(t, m.content, "DatadogAgent datadog/datadog", "stale data stays visible")
	assert.Contains(t, m.content, "ds: Disconnected: connection refused")
}

func TestModelMultipleDDAs(t *testing.T) {
	m, _, _ := testModel(t, newFakeStore(fixture(t, "multiple-ddas")), Config{})
	changeFrame(t, m)
	assert.Contains(t, m.content, "more than one DatadogAgent found")
	assert.Contains(t, m.content, "live", "the view keeps running")
	assert.NotContains(t, m.content, "updated+ready")
}

func TestModelNoDDA(t *testing.T) {
	snap := fixture(t, "steady")
	snap = withSources(snap, func(map[string]dash.SourceInfo) {})
	snap.Objects = maps.Clone(snap.Objects)
	delete(snap.Objects, dash.SourceDDA)
	m, _, _ := testModel(t, newFakeStore(snap), Config{Namespace: "datadog"})
	changeFrame(t, m)
	assert.Contains(t, m.content, "no DatadogAgent found in namespace datadog; waiting for one to be created (use -n or -A to look elsewhere)")
}

func TestModelFatal(t *testing.T) {
	m, _, _ := testModel(t, newFakeStore(fixture(t, "steady")), Config{})
	cmd := update(t, m, fatalMsg{err: errors.New("boom")})
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
	assert.EqualError(t, m.err, "boom")
}

func TestModelColors(t *testing.T) {
	steady := fixture(t, "steady")
	plain, _, _ := testModel(t, newFakeStore(steady), Config{Color: colorprofile.NoTTY})
	changeFrame(t, plain)
	assert.NotContains(t, plain.content, "\x1b")

	styled, _, _ := testModel(t, newFakeStore(steady), Config{Color: colorprofile.ANSI256})
	changeFrame(t, styled)
	assert.Contains(t, styled.content, "\x1b[")

	ascii, _, _ := testModel(t, newFakeStore(steady), Config{ASCII: true})
	changeFrame(t, ascii)
	assert.Contains(t, ascii.content, "* live - last change")
	for _, r := range ascii.content {
		assert.Less(t, r, rune(128))
	}
}

func TestModelPollOverdue(t *testing.T) {
	steady := fixture(t, "steady")
	snap := withSources(steady, func(s map[string]dash.SourceInfo) {
		s[dash.SourceOperator] = dash.SourceInfo{State: dash.SourceOK, Mode: dash.RefreshPoll, LastOK: testNow, NextPoll: testNow.Add(30 * time.Second)}
	})
	m, clock, _ := testModel(t, newFakeStore(snap), Config{})
	changeFrame(t, m)
	clock.Add(39 * time.Second)
	m.Update(tickMsg(clock.Now()))
	assert.Contains(t, strings.Split(m.content, "\n")[0], "live", "a poll in flight for 9s is normal")
	clock.Add(time.Second)
	m.Update(tickMsg(clock.Now()))
	status := strings.Split(m.content, "\n")[0]
	assert.Contains(t, status, "reconnecting: no answer from the API server for 10s")
}
