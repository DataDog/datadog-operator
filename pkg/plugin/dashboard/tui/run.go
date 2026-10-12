// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tui is the live, full-screen dashboard. It is
// the only package that imports Bubble Tea. The body is drawn by
// render.Text, the same layout as the static output.
package tui

import (
	"cmp"
	"context"
	"errors"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	dash "github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
)

// Defaults of Config.
const (
	// DefaultMinGap is the minimum time between two builds (at most 4
	// redraws per second).
	DefaultMinGap = 250 * time.Millisecond
	// DefaultTick refreshes the ages and the poll countdown.
	DefaultTick = time.Second
)

// Config configures Run.
type Config struct {
	// Store is the started live store.
	Store dash.Store
	// Build configures dash.Build.
	Build dash.BuildConfig
	// Observer keeps the workload history across builds; it defaults to a
	// dash.MemoryObserver.
	Observer dash.Observer
	// Fatal quits the view with its error; it defaults to a new Fatal.
	Fatal *Fatal
	// Namespace and AllNamespaces explain a missing DDA.
	Namespace     string
	AllNamespaces bool
	// Color is the color profile of the output; ASCII selects ASCII
	// glyphs.
	Color colorprofile.Profile
	ASCII bool

	// Input and Output default to the terminal.
	Input  io.Reader
	Output io.Writer
	// Width and Height set the initial size when Output is not a terminal.
	Width, Height int
	// Now defaults to time.Now; After to time.After.
	Now   func() time.Time
	After func(time.Duration) <-chan time.Time
	// MinGap defaults to DefaultMinGap, Tick to DefaultTick.
	MinGap, Tick time.Duration
}

// Run shows the live view until the user quits, ctx is canceled (signals)
// or a fatal error is reported. The terminal is restored before it returns;
// the caller prints the returned error afterwards.
func Run(ctx context.Context, cfg Config) error {
	p, m, cancel := newProgram(ctx, cfg)
	defer cancel()
	_, err := p.Run()
	if m.err != nil {
		return m.err
	}
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil && !errors.Is(err, tea.ErrProgramPanic) {
		// Canceled by a signal: a clean exit.
		return nil
	}
	return err
}

// newProgram builds the program and its model. cancel stops the model's
// commands.
func newProgram(ctx context.Context, cfg Config) (*tea.Program, *model, context.CancelFunc) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.After == nil {
		cfg.After = time.After
	}
	cfg.MinGap = cmp.Or(cfg.MinGap, DefaultMinGap)
	cfg.Tick = cmp.Or(cfg.Tick, DefaultTick)
	if cfg.Observer == nil {
		cfg.Observer = dash.NewMemoryObserver()
	}
	if cfg.Fatal == nil {
		cfg.Fatal = NewFatal()
	}
	mctx, cancel := context.WithCancel(ctx)
	b := &builder{store: cfg.Store, cfg: cfg.Build, obs: cfg.Observer, now: cfg.Now, after: cfg.After, gap: cfg.MinGap}
	m := newModel(mctx, cfg, b, cfg.Fatal)

	opts := []tea.ProgramOption{
		tea.WithContext(ctx),
		// Signals are handled by the caller through ctx.
		tea.WithoutSignalHandler(),
		tea.WithColorProfile(cfg.Color),
	}
	if cfg.Input != nil {
		opts = append(opts, tea.WithInput(cfg.Input))
	}
	if cfg.Output != nil {
		opts = append(opts, tea.WithOutput(cfg.Output))
	}
	if cfg.Width > 0 && cfg.Height > 0 {
		opts = append(opts, tea.WithWindowSize(cfg.Width, cfg.Height))
	}
	return tea.NewProgram(m, opts...), m, cancel
}
