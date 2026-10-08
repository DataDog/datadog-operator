// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package render

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
	"github.com/DataDog/datadog-operator/pkg/rollout"
)

// DefaultWidth is used when Theme.Width is not positive.
const DefaultWidth = 80

// minWidth is the narrowest layout the renderer draws. Narrower outputs get
// that layout with every line cut to the actual width.
const minWidth = 40

// Theme selects how Text draws. The caller always sets it explicitly: the
// renderer never inspects the terminal or the environment, so the output only
// depends on the View, the Theme and the clock.
type Theme struct {
	// Color is the color profile of the output. The zero value and
	// colorprofile.NoTTY produce plain text without any escape sequence;
	// colorprofile.ASCII keeps bold and faint but no colors.
	Color colorprofile.Profile
	// ASCII replaces box drawing, bars and symbols with ASCII.
	ASCII bool
	// Width is the output width in columns. Every line is truncated to it.
	Width int
	// NoLegend leaves out the bar legend; the live view draws it in its
	// footer.
	NoLegend bool
}

// width is the output width.
func (t Theme) width() int {
	if t.Width <= 0 {
		return DefaultWidth
	}
	return t.Width
}

// layoutWidth is the width the layout is drawn at, at least minWidth.
func (t Theme) layoutWidth() int { return max(t.width(), minWidth) }

func (t Theme) styled() bool { return t.Color > colorprofile.NoTTY }

// glyphs are the characters the layout is drawn with.
type glyphs struct {
	// replacement stands for control characters and invalid UTF-8 in
	// cluster-provided strings.
	replacement                                         rune
	ok, err, warn, info, progressing, unknown, disabled string
	barFull, barPartial, barEmpty, barOpen, barClose    string
	arrow, ellipsis, sep, approx                        string
	tee, elbow, pipe                                    string
	boxTL, boxTR, boxBL, boxBR, boxH, boxV              string
}

var unicodeGlyphs = glyphs{
	replacement: '\uFFFD',
	ok:          "✓", err: "✗", warn: "⚠", info: "•", progressing: "↻", unknown: "?", disabled: "–",
	barFull: "█", barPartial: "▒", barEmpty: "░",
	arrow: "→", ellipsis: "…", sep: "·", approx: "~",
	tee: "├─ ", elbow: "└─ ", pipe: "│  ",
	boxTL: "┌", boxTR: "┐", boxBL: "└", boxBR: "┘", boxH: "─", boxV: "│",
}

var asciiGlyphs = glyphs{
	replacement: '?',
	ok:          "+", err: "x", warn: "!", info: "i", progressing: "~", unknown: "?", disabled: "-",
	barFull: "#", barPartial: "+", barEmpty: ".", barOpen: "[", barClose: "]",
	arrow: "->", ellipsis: "...", sep: "-", approx: "~",
	tee: "|- ", elbow: "`- ", pipe: "|  ",
	boxTL: "+", boxTR: "+", boxBL: "+", boxBR: "+", boxH: "-", boxV: "|",
}

// tone is the semantic color of a piece of text.
type tone int

const (
	toneNone tone = iota
	toneOK
	toneError
	toneWarn
	toneProgress
	toneDim
	toneBold
)

// palette uses the 16 basic ANSI colors so that the user's terminal theme
// decides the actual shades, on light and dark backgrounds alike.
var palette = map[tone]lipgloss.Style{
	toneOK:       lipgloss.NewStyle().Foreground(lipgloss.Green),
	toneError:    lipgloss.NewStyle().Foreground(lipgloss.Red).Bold(true),
	toneWarn:     lipgloss.NewStyle().Foreground(lipgloss.Yellow),
	toneProgress: lipgloss.NewStyle().Foreground(lipgloss.Cyan),
	toneDim:      lipgloss.NewStyle().Faint(true),
	toneBold:     lipgloss.NewStyle().Bold(true),
}

// paint styles s, or returns it unchanged when the theme has no styling.
func (r *renderer) paint(t tone, s string) string {
	if !r.theme.styled() || s == "" {
		return s
	}
	st, ok := palette[t]
	if !ok {
		return s
	}
	return st.Render(s)
}

// badge returns the symbol and tone of a health badge. The symbol always
// accompanies the word, so health is never conveyed by color alone.
func (r *renderer) badge(b dashboard.Badge) (string, tone) {
	switch b {
	case dashboard.BadgeHealthy:
		return r.g.ok, toneOK
	case dashboard.BadgeProgressing:
		return r.g.progressing, toneProgress
	case dashboard.BadgeDegraded:
		return r.g.warn, toneWarn
	case dashboard.BadgeError:
		return r.g.err, toneError
	default:
		return r.g.unknown, toneDim
	}
}

// severity returns the symbol, word and tone of a condition severity.
func (r *renderer) severity(s dashboard.Severity) (string, string, tone) {
	switch s {
	case dashboard.SeverityError:
		return r.g.err, "Error", toneError
	case dashboard.SeverityWarning:
		return r.g.warn, "Warn", toneWarn
	default:
		return r.g.info, "Info", toneDim
	}
}

// phaseTone colors a rollout phase. Unknown phases render as-is.
func phaseTone(phase string) tone {
	switch rollout.Phase(phase) {
	case rollout.PhaseComplete, rollout.PhaseSucceeded:
		return toneOK
	case rollout.PhaseRolling, rollout.PhaseSettling, rollout.PhasePending, rollout.PhaseBaking:
		return toneProgress
	case rollout.PhaseStalled, rollout.PhasePaused:
		return toneWarn
	case rollout.PhaseFailed, rollout.PhaseTimedOut, rollout.PhaseRolledBack:
		return toneError
	default:
		return toneNone
	}
}
