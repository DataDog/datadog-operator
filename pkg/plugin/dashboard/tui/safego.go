// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"context"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"

	tea "charm.land/bubbletea/v2"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// Fatal carries the first fatal error of a live session, such as a panic in
// a goroutine, to the TUI, which then quits and restores the terminal
// It is safe for concurrent use.
type Fatal struct {
	ch chan error
}

// NewFatal returns an empty Fatal.
func NewFatal() *Fatal {
	return &Fatal{ch: make(chan error, 1)}
}

// Report records err. Only the first error is kept.
func (f *Fatal) Report(err error) {
	select {
	case f.ch <- err:
	default:
	}
}

// Go runs fn in a goroutine; a panic in fn is reported instead of crashing
// the process with the terminal in raw mode.
func (f *Fatal) Go(fn func()) {
	safeGo(f.Report, fn)
}

// RoutePanics routes the panics that client-go recovers in its goroutines
// (utilruntime.HandleCrash) to f, and keeps those goroutines from crashing
// the process. The returned function restores the previous handlers.
func RoutePanics(f *Fatal) (restore func()) {
	panicsMu.Lock()
	defer panicsMu.Unlock()
	prevHandlers, prevCrash := utilruntime.PanicHandlers, utilruntime.ReallyCrash
	utilruntime.PanicHandlers = append(slices.Clip(prevHandlers), func(_ context.Context, r any) {
		f.Report(panicError(r))
	})
	utilruntime.ReallyCrash = false
	return func() {
		panicsMu.Lock()
		defer panicsMu.Unlock()
		utilruntime.PanicHandlers, utilruntime.ReallyCrash = prevHandlers, prevCrash
	}
}

// panicsMu serializes RoutePanics and its restore functions.
var panicsMu sync.Mutex

// safeGo runs fn in a goroutine and reports a panic in it.
func safeGo(report func(error), fn func()) {
	go func() {
		defer recoverTo(report)
		fn()
	}()
}

// recoverTo reports a panic of the calling goroutine. It must be deferred.
func recoverTo(report func(error)) {
	if r := recover(); r != nil {
		report(panicError(r))
	}
}

// guard runs a command and turns a panic in it into a fatalMsg, so that the
// program quits cleanly and the error is printed once the terminal is
// restored.
func guard(fn func() tea.Msg) tea.Cmd {
	return func() (msg tea.Msg) {
		defer recoverTo(func(err error) { msg = fatalMsg{err: err} })
		return fn()
	}
}

func panicError(r any) error {
	return fmt.Errorf("internal error: %v\n%s", r, debug.Stack())
}
