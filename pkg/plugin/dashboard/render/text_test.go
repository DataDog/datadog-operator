// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package render

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
)

var update = flag.Bool("update", false, "rewrite the golden files of the render tests")

// testNow is the fixed clock of the fixtures.
var testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// scenarios are the dashboard fixtures with a View golden. Their build
// configuration comes from dashboard.FixtureBuildConfig.
var scenarios = []string{"steady", "profiles", "reconcile-errors", "helm", "summary", "stalled", "no-dap-crd", "dap-no-status", "stale-daps", "stale-warnings", "availability"}

var widths = []int{80, 120}

// modes are the rendering variants of the goldens. The color profile is
// fixed so that the goldens don't depend on the terminal running the tests.
var modes = []struct {
	name  string
	theme Theme
}{
	{"plain", Theme{Color: colorprofile.NoTTY}},
	{"ansi256", Theme{Color: colorprofile.ANSI256}},
	{"ascii", Theme{Color: colorprofile.NoTTY, ASCII: true}},
}

func fixtureDir(name string) string {
	return filepath.Join("..", "testdata", name)
}

func buildView(t *testing.T, name string) dashboard.View {
	t.Helper()
	return buildViewWith(t, name, dashboard.FixtureBuildConfig(name))
}

func buildViewWith(t *testing.T, name string, cfg dashboard.BuildConfig) dashboard.View {
	t.Helper()
	store := dashboard.NewFixtureStore(fixtureDir(name))
	require.NoError(t, store.Start(context.Background()))
	t.Cleanup(store.Close)
	v, err := dashboard.Build(store.Snapshot(), cfg, dashboard.NopObserver{}, testNow)
	require.NoError(t, err)
	return v
}

func assertGolden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file; run go test ./pkg/plugin/dashboard/render/ -update")
	assert.Equal(t, string(want), got)
}

// TestTextGolden covers every fixture at 80 and 120 columns in plain, ANSI
// 256 and ASCII mode. Run with -update to rewrite.
func TestTextGolden(t *testing.T) {
	for _, name := range scenarios {
		textGoldens(t, name, buildView(t, name))
	}
	for _, fv := range dashboard.FixtureVariants {
		textGoldens(t, fv.Name, buildViewWith(t, fv.Scenario, fv.Config))
	}
}

// textGoldens checks the renderings of v against testdata/<name>.
func textGoldens(t *testing.T, name string, v dashboard.View) {
	for _, m := range modes {
		for _, w := range widths {
			t.Run(fmt.Sprintf("%s/%s-%d", name, m.name, w), func(t *testing.T) {
				theme := m.theme
				theme.Width = w
				got := Text(v, theme, testNow)
				assertGolden(t, filepath.Join("testdata", name, fmt.Sprintf("%s-%d.golden", m.name, w)), got)
				assertLayout(t, got, theme)
			})
		}
	}
}

// assertLayout checks the properties every rendering must have.
func assertLayout(t *testing.T, out string, theme Theme) {
	t.Helper()
	if !theme.styled() {
		assert.NotContains(t, out, "\x1b", "plain output has no escape sequences")
	} else {
		assert.Contains(t, out, "\x1b[", "styled output has escape sequences")
	}
	if theme.ASCII {
		for i, b := range []byte(out) {
			if b >= 0x80 {
				assert.Failf(t, "non-ASCII byte in --ascii output", "offset %d: %q", i, out[max(i-20, 0):min(i+20, len(out))])
				break
			}
		}
	}
	for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(l), theme.Width, "line wider than the output: %q", ansi.Strip(l))
	}
}

// TestTextSeveritySymbols checks that every issue line carries its symbol and
// word, so severity is not conveyed by color alone.
func TestTextSeveritySymbols(t *testing.T) {
	v := buildView(t, "reconcile-errors")
	require.NotEmpty(t, v.Issues)
	for _, m := range modes {
		theme := m.theme
		theme.Width = 200
		out := ansi.Strip(Text(v, theme, testNow))
		g := unicodeGlyphs
		if theme.ASCII {
			g = asciiGlyphs
		}
		for _, c := range v.Issues {
			want := map[dashboard.Severity]string{
				dashboard.SeverityError:   g.err + " Error",
				dashboard.SeverityWarning: g.warn + " Warn",
			}[c.Severity]
			found := false
			for _, l := range strings.Split(out, "\n") {
				if strings.Contains(l, c.Message) {
					found = true
					assert.Contains(t, l, want, "%s: %q", m.name, l)
				}
			}
			assert.True(t, found, "%s: issue %q not rendered", m.name, c.Message)
		}
	}
}

// TestTextTruncation checks that a long message is cut to the width in the
// text output while the JSON output keeps it whole.
func TestTextTruncation(t *testing.T) {
	v := buildView(t, "reconcile-errors")
	long := strings.Repeat("very long reconcile failure ", 20) + "END"
	v.Issues[0].Message = long

	for _, m := range modes {
		for _, w := range widths {
			theme := m.theme
			theme.Width = w
			out := Text(v, theme, testNow)
			assertLayout(t, out, theme)
			plain := ansi.Strip(out)
			assert.NotContains(t, plain, "END", "%s-%d", m.name, w)
			ellipsis := unicodeGlyphs.ellipsis
			if theme.ASCII {
				ellipsis = asciiGlyphs.ellipsis
			}
			assert.Contains(t, plain, "DatadogAgentReconcileError: very long reconcile", "%s-%d", m.name, w)
			assert.Contains(t, plain, ellipsis+" "+unicodeOrASCII(theme, unicodeGlyphs.boxV, asciiGlyphs.boxV), "%s-%d: truncated line ends with an ellipsis", m.name, w)
		}
	}

	var b bytes.Buffer
	require.NoError(t, JSON(&b, v))
	assert.Contains(t, b.String(), long)
}

func unicodeOrASCII(t Theme, u, a string) string {
	if t.ASCII {
		return a
	}
	return u
}

func TestTextWidth(t *testing.T) {
	v := buildView(t, "steady")
	for _, tc := range []struct{ in, want int }{{0, DefaultWidth}, {-1, DefaultWidth}, {10, 10}, {39, 39}, {150, 150}} {
		out := Text(v, Theme{Width: tc.in}, testNow)
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		maxWidth := 0
		for _, l := range lines {
			maxWidth = max(maxWidth, lipgloss.Width(l))
		}
		// The DDA box spans the whole width; below minWidth the layout is
		// cut to the width.
		assert.Equal(t, tc.want, maxWidth, "width %d", tc.in)
	}
}

func TestTextMissingWorkload(t *testing.T) {
	v := buildView(t, "steady")
	v.DDA.Default.ClusterAgent = &dashboard.WorkloadView{
		Kind: "Deployment", Name: "datadog-agent-cluster-agent", Missing: true, Health: dashboard.BadgeUnknown,
	}
	out := Text(v, Theme{Width: 120}, testNow)
	assert.Regexp(t, `├─ \? DCA datadog-agent-cluster-agent +✗ missing`, out)
}

func TestTextZeroThemeIsPlain(t *testing.T) {
	out := Text(buildView(t, "reconcile-errors"), Theme{}, testNow)
	assert.NotContains(t, out, "\x1b")
}

// TestTextRollout covers the rollout indicator variants.
func TestTextRollout(t *testing.T) {
	r := &renderer{theme: Theme{Width: 120}, g: unicodeGlyphs, width: 120, now: testNow}
	at := func(d time.Duration) *time.Time {
		t := testNow.Add(d)
		return &t
	}
	for _, tc := range []struct {
		name string
		ro   dashboard.Rollout
		bar  int
		want string
	}{
		{"complete", dashboard.Rollout{Phase: "Complete", Percent: 100, Since: at(-time.Hour)}, 0, "100% Complete"},
		{"rolling", dashboard.Rollout{Phase: "Rolling", Percent: 57, Since: at(-10 * time.Minute)}, 0, " 57% Rolling 10m"},
		{"approximate since", dashboard.Rollout{Phase: "Rolling", Percent: 5, Since: at(-10 * time.Minute), SinceApprox: true}, 0, "  5% Rolling ~10m"},
		{"bar: ready, not ready, not updated", dashboard.Rollout{Phase: "Rolling", Updated: 5, UpdatedReady: 3, Desired: 10, Percent: 30}, 10, "███▒▒░░░░░  30% Rolling"},
		{"bar: all updated, some not ready", dashboard.Rollout{Phase: "Rolling", Updated: 10, UpdatedReady: 6, Desired: 10, Percent: 60}, 10, "██████▒▒▒▒  60% Rolling"},
		{"bar: rounding keeps not ready visible", dashboard.Rollout{Phase: "Rolling", Updated: 4, UpdatedReady: 3, Desired: 10, Percent: 30}, 7, "█▒░░░░░  30% Rolling"},
		{"bar: nothing to roll", dashboard.Rollout{Phase: "Complete", Percent: 100}, 5, "█████ 100% Complete"},
		{"nothing to roll and unknown: phase only", dashboard.Rollout{Phase: "Unknown", Percent: 100}, 10, "Unknown"},
		{"stalled with last progress and deadline", dashboard.Rollout{
			Phase: "Stalled", Percent: 25, Since: at(-30 * time.Minute), LastProgress: at(-20 * time.Minute), Deadline: at(-10 * time.Minute),
		}, 0, " 25% Stalled 30m · last progress 20m ago · deadline 10m ago"},
		{"future deadline", dashboard.Rollout{Phase: "Baking", Percent: 100, Deadline: at(5 * time.Minute)}, 0, "100% Baking · deadline in 5m"},
		{"unknown phase renders as-is", dashboard.Rollout{Phase: "Exotic", Percent: 0}, 0, "  0% Exotic"},
		{"empty phase", dashboard.Rollout{Desired: 1}, 0, "  0% Unknown"},
		{"clock skew", dashboard.Rollout{Phase: "Rolling", Percent: 1, Since: at(time.Minute)}, 0, "  1% Rolling 0s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, strings.Join(r.indicatorParts(tc.ro, tc.bar, true), r.sep()))
		})
	}
	r.g = asciiGlyphs
	assert.Equal(t, "[##+.......]  20% Rolling", strings.Join(r.indicatorParts(dashboard.Rollout{Phase: "Rolling", Updated: 3, UpdatedReady: 2, Desired: 10, Percent: 20}, 10, true), r.sep()))
}

// The legend drops its percentage note before cutting the glyph part.
func TestLegend(t *testing.T) {
	glyphs := "█ updated+ready  ▒ updated, not ready  ░ not updated"
	for _, tc := range []struct {
		name  string
		ascii bool
		width int
		want  string
	}{
		{"full", false, 120, glyphs + "   % = updated+ready / desired"},
		{"compact at 80", false, 80, glyphs + "   % = updated+ready/desired"},
		{"glyphs only", false, 60, glyphs},
		{"truncated", false, 30, "█ updated+ready  ▒ updated, n…"},
		{"ascii", true, 120, "# updated+ready  + updated, not ready  . not updated   % = updated+ready / desired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Legend(tc.ascii, tc.width)
			assert.Equal(t, tc.want, got)
			assert.LessOrEqual(t, lipgloss.Width(got), tc.width)
		})
	}
}

// The legend follows the DatadogAgent box only when a bar is drawn.
func TestTextLegend(t *testing.T) {
	v := buildView(t, "profiles")
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			th := m.theme
			th.Width = 80
			lines := strings.Split(ansi.Strip(Text(v, th, testNow)), "\n")
			i := slices.Index(lines, Legend(th.ASCII, 80))
			require.Positive(t, i)
			assert.Regexp(t, `^(└─+┘|\+-+\+)$`, lines[i-1], "directly under the box")
		})
	}
	v.DDA.Default, v.DDA.Profiles, v.Unattached, v.DDA.Rollout = nil, nil, nil, dashboard.Rollout{Phase: "Unknown"}
	assert.NotContains(t, Text(v, Theme{Width: 80}, testNow), "updated+ready")
}

// The live view draws the legend in its footer.
func TestTextNoLegend(t *testing.T) {
	v := buildView(t, "profiles")
	out := Text(v, Theme{Width: 120, NoLegend: true}, testNow)
	assert.NotContains(t, out, "updated, not ready")
	assert.Contains(t, out, "100%")
}

func TestTextBars(t *testing.T) {
	v := buildView(t, "steady")
	_, bars := TextBars(v, Theme{Width: 120}, testNow)
	assert.True(t, bars)
	v.DDA.Default, v.DDA.Profiles, v.DDA.Rollout = nil, nil, dashboard.Rollout{}
	out, bars := TextBars(v, Theme{Width: 120}, testNow)
	assert.False(t, bars)
	assert.NotContains(t, out, "updated, not ready")
}

// A poll that is due or in flight is not a zero countdown.
func TestTextPollDue(t *testing.T) {
	v := buildView(t, "steady")
	next := testNow.Add(20 * time.Second)
	v.Header.NextPoll = &next
	assert.Contains(t, Text(v, Theme{Width: 120}, testNow), "next poll in 20s")
	assert.Contains(t, Text(v, Theme{Width: 120}, next), "polling…")
	assert.Contains(t, Text(v, Theme{Width: 120}, next.Add(time.Minute)), "polling…")
}

func TestConditionText(t *testing.T) {
	assert.Equal(t, "Applied=False (Conflict): conflict with canary",
		conditionText(dashboard.Cond{Type: "Applied", Status: "False", Reason: "Conflict", Message: "conflict with canary"}))
	assert.Equal(t, "DatadogAgentReconcileError: boom",
		conditionText(dashboard.Cond{Type: "DatadogAgentReconcileError", Status: "True", Reason: "DatadogAgent_reconcile_error", Message: "boom"}))
	assert.Equal(t, "AgentReconcile=False (agent feature error): npm",
		conditionText(dashboard.Cond{Type: "AgentReconcile", Status: "False", Reason: "agent feature error", Message: "npm"}))
	assert.Equal(t, "Ready=Unknown", conditionText(dashboard.Cond{Type: "Ready", Status: "Unknown"}))
}

func TestSummaryItem(t *testing.T) {
	r := &renderer{theme: Theme{Width: 120}, g: unicodeGlyphs, width: 120, now: testNow}
	assert.Equal(t, "Monitors 5 (OK 1 Alert 1 No Data 2 Other 1 ✗ 1 ? 1)", r.summaryItem(dashboard.SummaryView{
		Label: "Monitors", State: dashboard.SourceOK, Total: 5, Errors: 1, Unknown: 1,
		Breakdown: map[string]int{"Other": 1, "No Data": 2, "Alert": 1, "OK": 1},
	}))
	assert.Equal(t, "SLOs 3", r.summaryItem(dashboard.SummaryView{Label: "SLOs", State: dashboard.SourceOK, Total: 3, OK: 3}))
	assert.Equal(t, "SLOs ⚠ no access", r.summaryItem(dashboard.SummaryView{Label: "SLOs", State: dashboard.SourceForbidden}))
	assert.Equal(t, "SLOs ✗ error", r.summaryItem(dashboard.SummaryView{Label: "SLOs", State: dashboard.SourceError}))
	assert.Equal(t, "SLOs loading", r.summaryItem(dashboard.SummaryView{Label: "SLOs", State: dashboard.SourceLoading}))
}

func TestNoticesReportUnreadableCoreSources(t *testing.T) {
	v := buildView(t, "summary")
	v.Sources = append(v.Sources, dashboard.SourceView{Key: dashboard.SourcePods, State: dashboard.SourceForbidden, Err: "pods is forbidden"})
	out := Text(v, Theme{Width: 120}, testNow)
	assert.Contains(t, out, "• DatadogAgentProfiles: listed in the DDA namespace only")
	assert.Contains(t, out, "⚠ pods: Forbidden: pods is forbidden")
	// Summary sources are reported in the summary row only.
	assert.NotContains(t, out, "sum/")
	assert.Contains(t, out, "SLOs ⚠ no access")
}

// TestJSON checks that the JSON renderer emits exactly the View schema
// guarded by the dashboard package goldens.
func TestJSON(t *testing.T) {
	for _, name := range scenarios {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			require.NoError(t, JSON(&b, buildView(t, name)))
			want, err := os.ReadFile(filepath.Join(fixtureDir(name), "view.golden.json"))
			require.NoError(t, err)
			assert.Equal(t, string(want), b.String())
		})
	}
	for _, fv := range dashboard.FixtureVariants {
		t.Run(fv.Name, func(t *testing.T) {
			var b bytes.Buffer
			require.NoError(t, JSON(&b, buildViewWith(t, fv.Scenario, fv.Config)))
			want, err := os.ReadFile(filepath.Join(fixtureDir(fv.Scenario), fv.Name+".golden.json"))
			require.NoError(t, err)
			assert.Equal(t, string(want), b.String())
		})
	}
}

func TestOneLine(t *testing.T) {
	for in, want := range map[string]string{
		"plain":                              "plain",
		"  multi\nline\r\n\twith   spaces  ": "multi line with spaces",
		"evil \x1b]0;pwn\x07 \x1b[2J msg":    "evil \uFFFD]0;pwn\uFFFD \uFFFD[2J msg",
		"c1 \u009b31m":                       "c1 \uFFFD31m",
		"del\x7f":                            "del\uFFFD",
		"bad utf8 \xff":                      "bad utf8 \uFFFD",
		"wide 日本 🚀":                          "wide 日本 🚀",
	} {
		assert.Equal(t, want, oneLine(in, '\uFFFD'), "%q", in)
	}
}

// TestTextSanitizesClusterStrings checks that control characters and
// newlines coming from the cluster never reach the text output, whatever
// field carries them: the output is the same as with the strings cleaned
// beforehand, so no line is added and the layout holds.
func TestTextSanitizesClusterStrings(t *testing.T) {
	const evil = "evil \x1b]0;pwn\x07\n\x1b[2J line2"
	inject := func(s string) dashboard.View {
		v := buildView(t, "stalled")
		require.NotEmpty(t, v.Issues)
		require.NotNil(t, v.DDA.Default.Agent)
		v.Issues[0].Message = s
		v.DDA.Default.Name = s
		v.DDA.Default.Agent.FailingPods = []dashboard.PodView{{Name: "p" + s, Node: "n" + s, Reason: s}}
		v.DDA.Helm = &dashboard.HelmView{Chart: "datadog" + s, Version: "3.1" + s, Release: s}
		v.Notices = append(v.Notices, s)
		v.Sources = append(v.Sources, dashboard.SourceView{Key: dashboard.SourcePods, State: dashboard.SourceError, Err: "line1\nline2 " + s})
		return v
	}
	v := inject(evil)
	for _, m := range modes {
		g := unicodeGlyphs
		if m.theme.ASCII {
			g = asciiGlyphs
		}
		want := inject(oneLine(evil, g.replacement))
		for _, w := range widths {
			theme := m.theme
			theme.Width = w
			out := Text(v, theme, testNow)
			assertLayout(t, out, theme)
			assert.Equal(t, Text(want, theme, testNow), out, "%s-%d", m.name, w)
			plain := ansi.Strip(out)
			assert.NotContains(t, plain, "\x1b", "%s-%d", m.name, w)
			assert.NotContains(t, plain, "\x07", "%s-%d", m.name, w)
		}
	}
	out := Text(v, Theme{Width: 400}, testNow)
	assert.Contains(t, out, "⚠ pods: Error: line1 line2 evil \uFFFD]0;pwn\uFFFD \uFFFD[2J line2")
	// The View itself is not modified.
	assert.Equal(t, evil, v.Issues[0].Message)
	assert.Equal(t, evil, v.DDA.Default.Agent.FailingPods[0].Reason)
	assert.Contains(t, Text(v, Theme{Width: 400, ASCII: true}, testNow), "evil ?]0;pwn? ?[2J line2")
}

// TestTextWideCharacters checks that double-width names and messages keep
// the box and the columns at the output width.
func TestTextWideCharacters(t *testing.T) {
	v := buildView(t, "reconcile-errors")
	v.DDA.Name = "エージェント🚀"
	v.DDA.Default.Name = "デフォルト設定のエージェント"
	require.NotNil(t, v.DDA.Default.Agent)
	v.DDA.Default.Agent.Name = "代理人-🐶🐶🐶"
	v.Issues[0].Message = strings.Repeat("調整に失敗しました🔥 ", 20)

	for _, m := range modes[:2] {
		for _, w := range []int{40, 80, 120} {
			theme := m.theme
			theme.Width = w
			out := Text(v, theme, testNow)
			assertLayout(t, out, theme)
			// Every box line ends exactly at the width.
			inBox := false
			for _, l := range strings.Split(strings.TrimSuffix(ansi.Strip(out), "\n"), "\n") {
				if strings.HasPrefix(l, "┌") {
					inBox = true
				}
				if inBox {
					assert.Equal(t, w, lipgloss.Width(l), "%s-%d: %q", m.name, w, l)
				}
				if strings.HasPrefix(l, "└") {
					inBox = false
				}
			}
		}
	}
}

// TestTextASCIIProfile checks the bold-only colorprofile.ASCII mode: bold
// and faint, but no colors.
func TestTextASCIIProfile(t *testing.T) {
	out := Text(buildView(t, "reconcile-errors"), Theme{Color: colorprofile.ASCII, Width: 120}, testNow)
	assert.Contains(t, out, "\x1b[1m", "bold")
	assert.Contains(t, out, "\x1b[2m", "faint")
	assert.NotRegexp(t, `\x1b\[[0-9;]*(3[0-9]|4[0-9]|9[0-7])[;m]`, out, "no color codes")
}

func TestTextNoWorkloadRows(t *testing.T) {
	v := buildView(t, "steady")
	v.DDA.Default, v.DDA.Rollout = nil, dashboard.Rollout{Phase: "Unknown"}
	out := Text(v, Theme{Width: 80}, testNow)
	assert.NotContains(t, out, "READY", "no header without workload rows")
	assert.Contains(t, out, "rollout Unknown")
	assert.NotContains(t, out, "100% Unknown")
	assert.Regexp(t, `└─ \? no default DDAI`, out)
}

func TestTextMultipleDDAs(t *testing.T) {
	v := dashboard.View{Header: dashboard.Header{RefreshedAt: testNow}, MultipleDDAs: []string{"a/one", "b/two"}}
	out := Text(v, Theme{Width: 80}, testNow)
	assert.Contains(t, out, "more than one DatadogAgent found")
	assert.Contains(t, out, "\n  a/one\n  b/two\n")
	assert.NotContains(t, out, "┌─ DatadogAgent")
}

// At 80 columns the rollout column keeps its bar, percentage, phase and
// age, and drops the strategy first.
func TestTextRolloutColumnAt80(t *testing.T) {
	out := Text(buildView(t, "profiles"), Theme{Width: 80}, testNow)
	assert.Regexp(t, `agent canary-agent +3/4 +1/4 +1 +▒+░+ +0% Rolling 10m │`, out)
	assert.NotContains(t, out, "maxUnavailable")
}

// The hidden profiles note is only shown when --hide-empty hid something.
func TestTextHiddenProfiles(t *testing.T) {
	theme := Theme{Color: colorprofile.NoTTY, Width: 100}
	v := buildView(t, "stale-daps")
	assert.NotContains(t, Text(v, theme, testNow), "hidden")

	v.DDA.HiddenProfiles = 1
	assert.Contains(t, Text(v, theme, testNow), "└─ – 1 profile hidden (0 desired) · --hide-empty")
	theme.ASCII = true
	assert.Contains(t, Text(v, theme, testNow), "`- - 1 profile hidden (0 desired) - --hide-empty")
}

// The --hide-empty-force note counts the hidden profiles without and with
// warnings, omits an empty group and shortens its wording to fit.
func TestTextHiddenProfilesForce(t *testing.T) {
	v := buildView(t, "stale-daps")
	v.DDA.HiddenBy = dashboard.FilterHideEmptyForce
	v.DDA.HiddenProfiles, v.DDA.HiddenProfilesWithoutWarnings, v.DDA.HiddenProfilesWithWarnings = 16, 2, 14
	for width, want := range map[int]string{
		120: "└─ – 16 profiles hidden (0 desired): 2 without warnings · 14 with warnings · --hide-empty-force",
		90:  "└─ – 16 hidden (0 desired): 2 without warnings · 14 with warnings · --hide-empty-force",
		80:  "└─ – 16 hidden: 2 no warnings · 14 with warnings · --hide-empty-force",
		60:  "└─ – 16 hidden: 2 ok · 14 warn · --hide-empty-force",
	} {
		theme := Theme{Color: colorprofile.NoTTY, Width: width}
		assert.Contains(t, Text(v, theme, testNow), want, width)
	}

	theme := Theme{Color: colorprofile.NoTTY, Width: 100}
	v.DDA.HiddenProfiles, v.DDA.HiddenProfilesWithoutWarnings, v.DDA.HiddenProfilesWithWarnings = 1, 0, 1
	assert.Contains(t, Text(v, theme, testNow), "└─ – 1 profile hidden (0 desired): 1 with warnings · --hide-empty-force")
	v.DDA.HiddenProfiles, v.DDA.HiddenProfilesWithoutWarnings, v.DDA.HiddenProfilesWithWarnings = 3, 3, 0
	theme.ASCII = true
	assert.Contains(t, Text(v, theme, testNow), "`- - 3 profiles hidden (0 desired): 3 without warnings - --hide-empty-force")
}
