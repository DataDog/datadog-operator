// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"

	dash "github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
	"github.com/DataDog/datadog-operator/pkg/plugin/dashboard/tui"
	"github.com/DataDog/datadog-operator/pkg/rollout"
)

// liveRecorder records which mode a command run took.
type liveRecorder struct {
	static, live bool
	cfg          tui.Config
	goFunc       func(func())
}

// runMode runs the command with a terminal or not and records the mode.
func runMode(t *testing.T, tty bool, args ...string) (*liveRecorder, string, error) {
	t.Helper()
	o, out, errOut := newTestOptions(t, "https://127.0.0.1:1")
	rec := &liveRecorder{}
	o.isTerminal = func(io.Writer) bool { return tty }
	o.newStore = func(*options) (dash.Store, error) {
		rec.static = true
		return dash.NewFixtureStore(filepath.Join(fixtures, "steady")), nil
	}
	o.newLiveStore = func(_ *options, goFunc func(func())) (dash.Store, error) {
		rec.goFunc = goFunc
		return dash.NewFixtureStore(filepath.Join(fixtures, "steady")), nil
	}
	o.runTUI = func(_ context.Context, cfg tui.Config) error {
		rec.live, rec.cfg = true, cfg
		return nil
	}
	cmd := newCmd(o)
	cmd.SetArgs(args)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	err := cmd.Execute()
	return rec, out.String() + errOut.String(), err
}

func TestDashboardModeSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		tty  bool
		args []string
		live bool
	}{
		{"terminal", true, nil, true},
		{"terminal with --once", true, []string{"--once"}, false},
		{"terminal with -o json", true, []string{"-o", "json"}, false},
		{"not a terminal", false, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, _, err := runMode(t, tc.tty, tc.args...)
			require.NoError(t, err)
			assert.Equal(t, tc.live, rec.live)
			assert.Equal(t, !tc.live, rec.static)
		})
	}
}

func TestDashboardLiveConfig(t *testing.T) {
	rec, out, err := runMode(t, true, "-A", "--pods", "--stall-after=1m", "--ascii", "--hide-empty", "--hide-empty-force", "--sort=desired", "--max-unavailable=5%", "agent")
	require.NoError(t, err)
	require.True(t, rec.live)
	assert.Empty(t, out, "nothing is printed around the live view")
	assert.Equal(t, dash.BuildConfig{DDAName: "agent", Pods: true, StallAfter: time.Minute, HideEmpty: true, HideEmptyForce: true, Sort: dash.SortDesired, MaxUnavailable: rollout.MaxUnavailable{Value: 5, Percent: true}}, rec.cfg.Build)
	assert.True(t, rec.cfg.AllNamespaces)
	assert.True(t, rec.cfg.ASCII)
	assert.Equal(t, "datadog", rec.cfg.Namespace)
	assert.NotNil(t, rec.cfg.Store)
	assert.NotNil(t, rec.cfg.Fatal)

	// The store's goroutines report their panics to the view.
	done := make(chan struct{})
	rec.goFunc(func() {
		defer close(done)
		panic("store")
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the store goroutine did not finish")
	}
}

// hangingStore is a store whose Close blocks, as a Helm read in flight
// against an unreachable API server does.
type hangingStore struct {
	dash.Store
	closing chan struct{}
	release chan struct{}
	// viewCtx is the context the view ran with; ctxErr its error when
	// Close starts.
	viewCtx context.Context //nolint:containedctx // checked by Close
	ctxErr  error
}

func (s *hangingStore) Close() {
	s.ctxErr = s.viewCtx.Err()
	close(s.closing)
	<-s.release
}

// Quitting returns promptly although the store is slow to stop, and the
// signal handler is released before the store is closed.
func TestDashboardLiveQuitWithHangingStore(t *testing.T) {
	o, _, _ := newTestOptions(t, "https://127.0.0.1:1")
	o.isTerminal = func(io.Writer) bool { return true }
	o.closeTimeout = 50 * time.Millisecond
	store := &hangingStore{
		Store:   dash.NewFixtureStore(filepath.Join(fixtures, "steady")),
		closing: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer close(store.release)
	o.newLiveStore = func(*options, func(func())) (dash.Store, error) { return store, nil }
	o.runTUI = func(ctx context.Context, _ tui.Config) error {
		store.viewCtx = ctx
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- o.run(context.Background()) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return while the store was closing")
	}
	select {
	case <-store.closing:
	default:
		t.Fatal("the store was not closed")
	}
	require.Error(t, store.ctxErr, "the signal context is released before the store is closed")
}

func TestDashboardLiveStartError(t *testing.T) {
	o, _, _ := newTestOptions(t, "https://127.0.0.1:1")
	o.isTerminal = func(io.Writer) bool { return true }
	o.newLiveStore = func(*options, func(func())) (dash.Store, error) {
		return dash.NewFixtureStore(filepath.Join(fixtures, "does-not-exist")), nil
	}
	o.runTUI = func(context.Context, tui.Config) error {
		t.Fatal("the view must not start")
		return nil
	}
	require.Error(t, o.run(context.Background()))
}

func TestDashboardLiveErrorIsReturned(t *testing.T) {
	o, _, _ := newTestOptions(t, "https://127.0.0.1:1")
	o.isTerminal = func(io.Writer) bool { return true }
	o.newLiveStore = func(*options, func(func())) (dash.Store, error) {
		return dash.NewFixtureStore(filepath.Join(fixtures, "steady")), nil
	}
	boom := errors.New("internal error: boom")
	o.runTUI = func(context.Context, tui.Config) error { return boom }
	require.ErrorIs(t, o.run(context.Background()), boom)
}

func TestDashboardLiveLogFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "dashboard.log")
	o, _, _ := newTestOptions(t, "https://127.0.0.1:1")
	o.isTerminal = func(io.Writer) bool { return true }
	o.logFile = logFile
	o.newLiveStore = func(*options, func(func())) (dash.Store, error) {
		return dash.NewFixtureStore(filepath.Join(fixtures, "steady")), nil
	}
	o.runTUI = func(context.Context, tui.Config) error {
		klog.Warning("reflector warning")
		klog.Flush()
		return nil
	}
	require.NoError(t, o.run(context.Background()))
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "reflector warning")
}
