// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// syncBuffer is a buffer safe for the program's writer and the test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// runProgram runs the live view on a pipe and a buffer and returns the
// input writer, the output and the result of Run.
func runProgram(t *testing.T, ctx context.Context, cfg Config) (io.Writer, *syncBuffer, <-chan error) {
	t.Helper()
	in, inW := io.Pipe()
	t.Cleanup(func() { _ = inW.Close() })
	out := &syncBuffer{}
	cfg.Input, cfg.Output, cfg.Width, cfg.Height = in, out, 120, 40
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()
	return inW, out, done
}

func waitDone(t *testing.T, done <-chan error, within time.Duration) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(within):
		t.Fatalf("Run did not return within %s", within)
		return nil
	}
}

func TestRunQuitKeys(t *testing.T) {
	for name, key := range map[string]string{"q": "q", "ctrl+c": "\x03"} {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore(fixture(t, "steady"))
			in, out, done := runProgram(t, context.Background(), Config{Store: store})
			require.Eventually(t, func() bool { return strings.Contains(out.String(), "DatadogAgent datadog/datadog") }, 5*time.Second, 10*time.Millisecond)

			start := time.Now()
			_, err := io.WriteString(in, key)
			require.NoError(t, err)
			require.NoError(t, waitDone(t, done, 2*time.Second))
			// The target is 100 ms; the bound leaves room for loaded runners
			// and the race detector.
			assert.Less(t, time.Since(start), time.Second)
			t.Logf("quit after %s", time.Since(start))

			o := out.String()
			assert.Contains(t, o, "\x1b[?1049h", "the view uses the alternate screen")
			assert.Contains(t, o, "\x1b[?1049l", "the alternate screen is left")
		})
	}
}

func TestRunCanceledContextIsCleanExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := newFakeStore(fixture(t, "steady"))
	_, out, done := runProgram(t, ctx, Config{Store: store})
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "DatadogAgent") }, 5*time.Second, 10*time.Millisecond)
	cancel() // as a SIGTERM or SIGHUP does through signal.NotifyContext
	require.NoError(t, waitDone(t, done, 2*time.Second))
	assert.Contains(t, out.String(), "\x1b[?1049l")
}

func TestRunLiveUpdates(t *testing.T) {
	steady := fixture(t, "steady")
	store := newFakeStore(steady)
	in, out, done := runProgram(t, context.Background(), Config{Store: store, MinGap: time.Millisecond})
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "DatadogAgent datadog/datadog") }, 5*time.Second, 10*time.Millisecond)
	store.set(fixture(t, "multiple-ddas"))
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "more than one DatadogAgent") }, 5*time.Second, 10*time.Millisecond)
	_, _ = io.WriteString(in, "q")
	require.NoError(t, waitDone(t, done, 2*time.Second))
}

func TestRunFatal(t *testing.T) {
	fatal := NewFatal()
	store := newFakeStore(fixture(t, "steady"))
	_, out, done := runProgram(t, context.Background(), Config{Store: store, Fatal: fatal})
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "DatadogAgent") }, 5*time.Second, 10*time.Millisecond)

	fatal.Go(func() { panic("store goroutine") })
	err := waitDone(t, done, 2*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal error: store goroutine")
	assert.Contains(t, out.String(), "\x1b[?1049l", "the terminal is restored")
	assert.NotContains(t, out.String(), "internal error", "the error is printed by the caller after the restore")
}

func TestRunPanicInCommand(t *testing.T) {
	store := newFakeStore(fixture(t, "steady"))
	store.snap = nil // Build panics on a nil snapshot
	_, out, done := runProgram(t, context.Background(), Config{Store: store})
	err := waitDone(t, done, 5*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal error")
	assert.Contains(t, out.String(), "\x1b[?1049l")
}

func TestSafeGo(t *testing.T) {
	errs := make(chan error, 1)
	safeGo(func(err error) { errs <- err }, func() { panic("boom") })
	select {
	case err := <-errs:
		assert.Contains(t, err.Error(), "internal error: boom")
	case <-time.After(time.Second):
		t.Fatal("the panic was not reported")
	}

	msg := guard(func() tea.Msg { panic("in a command") })()
	require.IsType(t, fatalMsg{}, msg)
	assert.Contains(t, msg.(fatalMsg).err.Error(), "in a command")

	f := NewFatal()
	f.Report(errors.New("first"))
	f.Report(errors.New("second"))
	assert.EqualError(t, <-f.ch, "first")
}

func TestRoutePanics(t *testing.T) {
	f := NewFatal()
	restore := RoutePanics(f)
	t.Cleanup(restore)
	func() {
		defer utilruntime.HandleCrash()
		panic("client-go goroutine")
	}()
	select {
	case err := <-f.ch:
		assert.Contains(t, err.Error(), "client-go goroutine")
	default:
		t.Fatal("the panic was not routed")
	}
	restore()
	assert.True(t, utilruntime.ReallyCrash)
}
