// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/util/duration"

	dash "github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
	"github.com/DataDog/datadog-operator/pkg/plugin/dashboard/render"
)

// Smallest terminal the live view draws in.
const (
	MinWidth  = 80
	MinHeight = 20
)

// pollOverdue is how long a poll may go unanswered before the view shows
// it is reconnecting.
const pollOverdue = 10 * time.Second

// model is the Bubble Tea model of the live view. Data comes only from
// frames built off the UI goroutine; Update swaps them in.
type model struct {
	ctx   context.Context //nolint:containedctx // bounds the commands
	b     *builder
	fatal *Fatal
	cfg   Config
	tick  time.Duration

	width, height int
	frame         *frame
	// at is the clock the text is drawn with: the last frame or tick.
	at time.Time
	// tickBusy is set while a tick build is in flight.
	tickBusy bool
	err      error
	content  string
}

func newModel(ctx context.Context, cfg Config, b *builder, fatal *Fatal) *model {
	return &model{ctx: ctx, b: b, fatal: fatal, cfg: cfg, tick: cfg.Tick, at: cfg.Now()}
}

// Init implements tea.Model.
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.b.waitForChange(m.ctx), tick(m.tick), waitForFatal(m.ctx, m.fatal))
}

// Update implements tea.Model.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case viewMsg:
		var cmd tea.Cmd
		if msg.change {
			cmd = m.b.waitForChange(m.ctx)
		} else {
			m.tickBusy = false
		}
		if msg.frame == nil || (m.frame != nil && msg.frame.seq <= m.frame.seq) {
			return m, cmd
		}
		m.frame, m.at = msg.frame, msg.frame.at
		m.content = m.render()
		return m, cmd
	case tickMsg:
		m.at = m.cfg.Now()
		cmds := []tea.Cmd{tick(m.tick)}
		if !m.tickBusy {
			m.tickBusy = true
			cmds = append(cmds, m.b.tickBuild())
		}
		m.content = m.render()
		return m, tea.Batch(cmds...)
	case fatalMsg:
		m.err = msg.err
		return m, tea.Quit
	default:
		return m, nil
	}
	m.content = m.render()
	return m, nil
}

// View implements tea.Model.
func (m *model) View() tea.View {
	v := tea.NewView(m.content)
	v.AltScreen = true
	v.WindowTitle = "kubectl datadog dashboard"
	return v
}

// render draws the screen for the current frame, size and clock.
func (m *model) render() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return ""
	}
	if w < MinWidth || h < MinHeight {
		return m.finish(m.tooSmall(w, h), w)
	}
	theme := render.Theme{Color: m.cfg.Color, ASCII: m.cfg.ASCII, Width: w, NoLegend: true}
	var body []string
	legend := false
	switch f := m.frame; {
	case f == nil || f.loading != nil:
		body = m.loadingBody(f)
	case f.err != nil:
		body = m.errorBody(f)
	default:
		var text string
		text, legend = render.TextBars(f.view, theme, m.at)
		body = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}

	room := h - 2 // status line and footer
	if len(body) > room {
		hidden := len(body) - room + 1
		body = append(body[:room-1], m.paint(dim, fmt.Sprintf("%s %d more lines; enlarge the terminal to see them", m.g().ellipsis, hidden)))
	}
	lines := make([]string, 0, h)
	lines = append(lines, m.status())
	lines = append(lines, body...)
	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	lines = append(lines, m.footer(w, legend))
	return m.finish(lines, w)
}

// finish cuts the lines to the width and downsamples the colors to the
// output profile.
func (m *model) finish(lines []string, w int) string {
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, m.g().ellipsis)
	}
	out := strings.Join(lines, "\n")
	if m.cfg.Color <= colorprofile.NoTTY {
		return ansi.Strip(out)
	}
	var b strings.Builder
	cw := &colorprofile.Writer{Forward: &b, Profile: m.cfg.Color}
	_, _ = io.WriteString(cw, out)
	return b.String()
}

func (m *model) tooSmall(w, h int) []string {
	return []string{
		m.paint(warn, fmt.Sprintf("%s terminal too small: %dx%d", m.g().warn, w, h)),
		fmt.Sprintf("the dashboard needs at least %dx%d", MinWidth, MinHeight),
		m.paint(dim, "q quit"),
	}
}

// status is the top line: live, loading or disconnected, with the age of
// the newest event of a watched source.
func (m *model) status() string {
	g := m.g()
	f := m.frame
	if f == nil || f.loading != nil {
		return m.paint(dim, g.loading+" loading")
	}
	var disconnected []string
	var last, overdue time.Time
	for key, s := range f.sources {
		if s.State == dash.SourceDisconnected {
			disconnected = append(disconnected, key)
		}
		if s.Mode == dash.RefreshWatch && s.LastOK.After(last) {
			last = s.LastOK
		}
		if s.Mode == dash.RefreshPoll && !s.NextPoll.IsZero() && (overdue.IsZero() || s.NextPoll.Before(overdue)) {
			overdue = s.NextPoll
		}
	}
	slices.Sort(disconnected)
	if last.IsZero() {
		last = f.at
	}
	age := duration.HumanDuration(max(m.at.Sub(last), 0))
	switch {
	case len(disconnected) > 0:
		// The source list comes last: it is cut to the width.
		return m.paint(warn, fmt.Sprintf("%s disconnected, reconnecting (%d %s)", g.warn, len(disconnected), plural(len(disconnected), "source"))) +
			m.paint(dim, " "+g.sep+" last change "+age+" ago "+g.sep+" "+strings.Join(disconnected, ", "))
	case !overdue.IsZero() && m.at.Sub(overdue) >= pollOverdue:
		// Watches only fail after the client's health check (about 45 s);
		// a poll with no answer shows a hung connection earlier.
		return m.paint(warn, fmt.Sprintf("%s reconnecting: no answer from the API server for %s", g.warn, duration.HumanDuration(m.at.Sub(overdue)))) +
			m.paint(dim, " "+g.sep+" last change "+age+" ago")
	}
	return m.paint(ok, g.live+" live") + m.paint(dim, " "+g.sep+" last change "+age+" ago")
}

// loadingBody is shown until the selected DDA's sources are read; it
// never shows counts.
func (m *model) loadingBody(f *frame) []string {
	head := "kubectl datadog dashboard"
	if f != nil && f.cluster.Context != "" {
		head += " " + m.g().sep + " ctx " + f.cluster.Context
	}
	lines := []string{m.paint(bold, head), ""}
	if f == nil {
		return append(lines, "Loading"+m.g().ellipsis)
	}
	return append(lines, "Loading"+m.g().ellipsis+" waiting for "+strings.Join(f.loading, ", "))
}

// errorBody explains a frame Build could not make. The view keeps waiting
// for the data to change, e.g. for a DDA to be created.
func (m *model) errorBody(f *frame) []string {
	head := "kubectl datadog dashboard"
	if f.cluster.Context != "" {
		head += " " + m.g().sep + " ctx " + f.cluster.Context
	}
	msg := f.err.Error()
	if errors.Is(f.err, dash.ErrNoDDA) {
		where := "in namespace " + m.cfg.Namespace
		if m.cfg.AllNamespaces {
			where = "in any namespace"
		}
		msg = fmt.Sprintf("%s %s; waiting for one to be created", f.err, where)
		if !m.cfg.AllNamespaces {
			msg += " (use -n or -A to look elsewhere)"
		}
	}
	return []string{m.paint(bold, head), "", m.paint(warn, m.g().warn+" "+msg)}
}

// footer is the bar legend and the keys.
func (m *model) footer(w int, legend bool) string {
	keys := "q quit"
	if !legend {
		return m.paint(dim, keys)
	}
	room := w - lipgloss.Width(keys) - 3
	return m.paint(dim, render.Legend(m.cfg.ASCII, room)+"   "+keys)
}

type tone int

const (
	dim tone = iota
	bold
	ok
	warn
)

var styles = map[tone]lipgloss.Style{
	dim:  lipgloss.NewStyle().Faint(true),
	bold: lipgloss.NewStyle().Bold(true),
	ok:   lipgloss.NewStyle().Foreground(lipgloss.Green),
	warn: lipgloss.NewStyle().Foreground(lipgloss.Yellow),
}

func (m *model) paint(t tone, s string) string {
	if m.cfg.Color <= colorprofile.NoTTY {
		return s
	}
	return styles[t].Render(s)
}

type glyphs struct{ live, loading, warn, sep, ellipsis string }

func (m *model) g() glyphs {
	if m.cfg.ASCII {
		return glyphs{live: "*", loading: "~", warn: "!", sep: "-", ellipsis: "..."}
	}
	return glyphs{live: "●", loading: "◌", warn: "⚠", sep: "·", ellipsis: "…"}
}

func plural(n int, s string) string {
	if n == 1 {
		return s
	}
	return s + "s"
}
