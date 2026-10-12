// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package render draws a dashboard View as styled text or JSON. It has no I/O
// beyond the writer it is given and never inspects the terminal: the caller
// passes the color profile, the ASCII mode and the width in a Theme.
package render

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
	"github.com/DataDog/datadog-operator/pkg/rollout"
)

const (
	// barWidth and barMinWidth bound the cells of a rollout progress bar.
	barWidth    = 10
	barMinWidth = 5
	// labelMinWidth is the narrowest workload label before the rollout
	// column gives up its optional parts.
	labelMinWidth = 24
	// maxFailingPods and maxSummaryFailures bound the detail lines; the
	// JSON output has them all.
	maxFailingPods     = 5
	maxSummaryFailures = 5
)

// Text renders the View as a static dashboard.
// now is the clock for ages; its location is used for the refresh time.
// Cluster strings are sanitized first.
func Text(v dashboard.View, t Theme, now time.Time) string {
	out, _ := TextBars(v, t, now)
	return out
}

// TextBars is Text, and also reports whether a rollout bar was drawn, i.e.
// whether the bar legend applies.
func TextBars(v dashboard.View, t Theme, now time.Time) (string, bool) {
	r := &renderer{theme: t, g: unicodeGlyphs, width: t.layoutWidth(), now: now}
	if t.ASCII {
		r.g = asciiGlyphs
	}
	v = sanitizeView(v, r.g.replacement)
	r.ns = v.DDA.Namespace
	r.header(v.Header, profilesNotInstalled(v.Sources))
	if len(v.MultipleDDAs) > 0 {
		r.multipleDDAs(v.MultipleDDAs)
	} else {
		r.ddaBox(v)
		if r.barDrawn && !t.NoLegend {
			r.line(r.paint(toneDim, Legend(t.ASCII, r.width)))
		}
	}
	r.summary(v.Summary)
	r.notices(v)

	// Below the narrowest layout, cut the lines to the actual width.
	if w := t.width(); w < r.width {
		for i, l := range r.lines {
			r.lines[i] = r.truncate(l, w)
		}
	}
	out := strings.Join(r.lines, "\n") + "\n"
	if !t.styled() {
		return out, r.barDrawn
	}
	// Styles are rendered with full-fidelity colors; downsample them to the
	// requested profile.
	var b strings.Builder
	w := &colorprofile.Writer{Forward: &b, Profile: t.Color}
	_, _ = io.WriteString(w, out)
	return b.String(), r.barDrawn
}

type renderer struct {
	theme Theme
	g     glyphs
	width int
	now   time.Time
	// ns is the DDA namespace; objects in it are shown without namespace.
	ns    string
	lines []string
	// barDrawn is set once a rollout bar is drawn, to add the legend.
	barDrawn bool
}

// Legend is the plain-text legend of the rollout bars and percentage, in
// ASCII when ascii is set. It is shortened to fit width: the percentage
// note is compacted, then dropped, then the rest is truncated.
func Legend(ascii bool, width int) string {
	g := unicodeGlyphs
	if ascii {
		g = asciiGlyphs
	}
	glyphs := g.barFull + " updated+ready  " + g.barPartial + " updated, not ready  " + g.barEmpty + " not updated"
	for _, l := range []string{
		glyphs + "   % = updated+ready / desired",
		glyphs + "   % = updated+ready/desired",
		glyphs,
	} {
		if lipgloss.Width(l) <= width {
			return l
		}
	}
	return ansi.Truncate(glyphs, max(width, 0), g.ellipsis)
}

// line appends one output line, truncated to the width.
func (r *renderer) line(s string) {
	r.lines = append(r.lines, r.truncate(s, r.width))
}

func (r *renderer) truncate(s string, width int) string {
	return ansi.Truncate(s, width, r.g.ellipsis)
}

func (r *renderer) sep() string { return r.paint(toneDim, " "+r.g.sep+" ") }

func (r *renderer) dimLabel(label, value string) string {
	return r.paint(toneDim, label) + " " + value
}

// wrap joins items with separators into lines no wider than width;
// continuation lines start with indent. An item wider than width gets a
// line of its own and is truncated later.
func (r *renderer) wrap(items []string, width int, indent string) []string {
	sep := r.sep()
	var out []string
	cur := ""
	for _, it := range items {
		switch {
		case it == "":
		case cur == "":
			cur = it
		case lipgloss.Width(cur)+lipgloss.Width(sep)+lipgloss.Width(it) <= width:
			cur += sep + it
		default:
			out = append(out, cur)
			cur = indent + it
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// ago is the age of t, clamped at zero against clock skew.
func (r *renderer) ago(t time.Time) string {
	return duration.HumanDuration(max(r.now.Sub(t), 0))
}

// until is the time left until t, clamped at zero.
func (r *renderer) until(t time.Time) string {
	return duration.HumanDuration(max(t.Sub(r.now), 0))
}

// header draws the context line(s). noProfiles adds a quiet
// note that the optional DAP CRD is not installed.
func (r *renderer) header(h dashboard.Header, noProfiles bool) {
	items := []string{r.paint(toneBold, "kubectl datadog dashboard")}
	add := func(label, value string) {
		if value != "" {
			items = append(items, r.dimLabel(label, value))
		}
	}
	add("ctx", h.Context)
	add("ns", h.Namespace)
	add("k8s", h.ServerVersion)
	add("plugin", h.PluginVersion)
	add("provider", r.provider(h.Provider))
	add("operator", r.operator(h.Operator))
	if noProfiles {
		add("profiles", r.paint(toneDim, "not installed"))
	}
	add("refreshed", h.RefreshedAt.In(r.now.Location()).Format("15:04:05"))
	switch {
	case h.NextPoll == nil:
	case h.NextPoll.After(r.now):
		add("next poll in", r.until(*h.NextPoll))
	default:
		// The poll is due or in flight, e.g. while the API server is
		// unreachable.
		items = append(items, "polling"+r.g.ellipsis)
	}
	for _, l := range r.wrap(items, r.width, "  ") {
		r.line(l)
	}
}

// profilesNotInstalled reports whether the DAP CRD is not installed.
func profilesNotInstalled(sources []dashboard.SourceView) bool {
	for _, s := range sources {
		if s.Key == dashboard.SourceDAP {
			return s.State == dashboard.SourceNotInstalled
		}
	}
	return false
}

// multipleDDAs replaces the DatadogAgent block when several are in scope.
func (r *renderer) multipleDDAs(names []string) {
	r.line(r.paint(toneWarn, r.g.warn+" more than one DatadogAgent found; pass the name of the one to show:"))
	for _, n := range names {
		r.line("  " + n)
	}
}

func (r *renderer) provider(p dashboard.ProviderView) string {
	name := p.Name
	if name == "" {
		name = "none"
	}
	if p.Source != "" {
		name += " (" + p.Source + ")"
	}
	return name
}

func (r *renderer) operator(o *dashboard.OperatorView) string {
	if o == nil {
		return r.paint(toneDim, "not found")
	}
	version := o.Version
	if version == "" {
		version = "?"
	}
	sym, tn := r.g.ok, toneOK
	if o.Desired == 0 || o.Ready < o.Desired {
		sym, tn = r.g.warn, toneWarn
	}
	s := version + " " + r.paint(tn, fmt.Sprintf("%s %d/%d ready", sym, o.Ready, o.Desired))
	if o.LeaseHolder != "" {
		s += " " + r.paint(toneDim, "leader") + " " + o.LeaseHolder
		if o.LeaseRenew != nil {
			s += " (" + r.ago(*o.LeaseRenew) + " ago)"
		}
	}
	return s
}

// ddaBox draws the DatadogAgent block: badge, totals, tree and conditions.
func (r *renderer) ddaBox(v dashboard.View) {
	d := v.DDA
	inner := r.width - 4

	sym, tn := r.badge(d.Health)
	unavailable := fmt.Sprintf("%d unavailable", d.Agents.Unavailable)
	if d.Agents.Unavailable > 0 {
		unavailable = r.paint(toneWarn, unavailable)
	}
	// The rollout parts wrap separately so that a narrow output keeps the
	// last progress and the deadline.
	ro := r.indicatorParts(d.Rollout, barWidth, true)
	ro[0] = r.dimLabel("rollout", ro[0])
	body := append([]string{""}, r.wrap(append([]string{
		r.paint(tn, sym+" "+string(d.Health)),
		r.dimLabel("agents", fmt.Sprintf("%d/%d ready", d.Agents.Ready, d.Agents.Desired)),
		fmt.Sprintf("%d/%d up-to-date", d.Agents.UpToDate, d.Agents.Desired),
		unavailable,
	}, ro...), inner, "  ")...)
	var extra []string
	if d.Helm != nil {
		extra = append(extra, r.helm(d.Helm)...)
	}
	if e := d.Experiment; e != nil {
		s := e.Phase
		if e.ID != "" {
			s += " " + e.ID
		}
		if e.StartedAt != nil {
			s += " (" + r.ago(*e.StartedAt) + ")"
		}
		extra = append(extra, r.dimLabel("experiment", r.paint(toneProgress, s)))
	}
	// Helm and experiment lines are indented like the wrapped badge line.
	for _, l := range r.wrap(extra, inner-2, "") {
		body = append(body, "  "+l)
	}

	body = append(body, "")
	body = append(body, r.tree(v, inner)...)
	body = append(body, "")
	body = append(body, r.conditions(v)...)

	title := r.paint(toneBold, "DatadogAgent "+d.Namespace+"/"+d.Name)
	if d.Created != nil {
		title += r.sep() + r.dimLabel("age", r.ago(*d.Created))
	}
	r.box(title, body)
}

func (r *renderer) helm(h *dashboard.HelmView) []string {
	var items []string
	if h.Chart != "" || h.Version != "" {
		version := h.Version
		if h.PreviousVersion != "" && h.PreviousVersion != h.Version {
			version = h.PreviousVersion + " " + r.g.arrow + " " + version
		}
		items = append(items, r.dimLabel("chart", strings.TrimSpace(h.Chart+" "+version)))
	}
	if h.Release != "" {
		s := h.Release
		if h.Namespace != "" {
			s = h.Namespace + "/" + s
		}
		if h.Revision > 0 {
			s += " rev "
			if h.PreviousRevision > 0 && h.PreviousRevision != h.Revision {
				s += strconv.Itoa(h.PreviousRevision) + " " + r.g.arrow + " "
			}
			s += strconv.Itoa(h.Revision)
		}
		items = append(items, r.dimLabel("release", s))
	}
	return items
}

// box draws body inside a border whose top edge carries the title.
func (r *renderer) box(title string, body []string) {
	g := r.g
	inner := r.width - 4
	title = r.truncate(title, r.width-5)
	top := g.boxTL + g.boxH + " " + title + " "
	r.line(top + strings.Repeat(g.boxH, max(r.width-1-lipgloss.Width(top), 0)) + g.boxTR)
	for _, l := range body {
		l = r.truncate(l, inner)
		r.line(g.boxV + " " + l + strings.Repeat(" ", inner-lipgloss.Width(l)) + " " + g.boxV)
	}
	r.line(g.boxBL + strings.Repeat(g.boxH, r.width-2) + g.boxBR)
}

// item is a node of the object tree. Workload rows have counts and are
// aligned as a table; heading rows have none.
type item struct {
	label string
	// counts are READY, UPD and UNAV; nil for heading rows.
	counts   []string
	rollout  rowRollout
	details  []string
	children []item
}

// rowRollout is the ROLLOUT cell of a workload row, drawn once the column
// width is known.
type rowRollout struct {
	ro       dashboard.Rollout
	strategy string
	// missing replaces the rollout with a "missing" marker.
	missing string
}

type treeLine struct {
	label   string
	counts  []string
	rollout rowRollout
}

// flatten draws the tree branches: first prefixes the item's own row, rest
// prefixes its details and children.
func (r *renderer) flatten(it item, first, rest string, out *[]treeLine) {
	*out = append(*out, treeLine{label: first + it.label, counts: it.counts, rollout: it.rollout})
	for _, d := range it.details {
		*out = append(*out, treeLine{label: rest + "   " + d})
	}
	for i, c := range it.children {
		if i == len(it.children)-1 {
			r.flatten(c, rest+r.g.elbow, rest+"   ", out)
		} else {
			r.flatten(c, rest+r.g.tee, rest+r.g.pipe, out)
		}
	}
}

// tree lays out DDA → {default DDAI → workloads, DAP → DDAI → DaemonSet},
// then the Unattached group, with aligned count columns.
func (r *renderer) tree(v dashboard.View, inner int) []string {
	root := item{label: r.badgeSymbol(v.DDA.Health) + " " + r.paint(toneDim, "DDA") + " " + v.DDA.Name}
	if v.DDA.Default != nil {
		root.children = append(root.children, r.ddaiItem(v.DDA.Default, " (default)", true))
	} else {
		root.children = append(root.children, item{label: r.paint(toneDim, r.g.unknown+" no default DDAI")})
	}
	for i := range v.DDA.Profiles {
		root.children = append(root.children, r.dapItem(&v.DDA.Profiles[i]))
	}
	if v.DDA.HiddenProfiles > 0 {
		room := inner - lipgloss.Width(r.g.elbow)
		root.children = append(root.children, item{label: r.paint(toneDim, r.hiddenProfiles(&v.DDA, room))})
	}
	items := []item{root}
	if u := v.Unattached; u != nil {
		group := item{label: r.paint(toneWarn, r.g.warn+" Unattached")}
		for i := range u.DDAIs {
			group.children = append(group.children, r.ddaiItem(&u.DDAIs[i], "", false))
		}
		for i := range u.Workloads {
			w := &u.Workloads[i]
			group.children = append(group.children, r.workloadItem(workloadTag(w), w))
		}
		items = append(items, group)
	}

	var lines []treeLine
	for _, it := range items {
		r.flatten(it, "", "", &lines)
	}

	headers := []string{"READY", "UPD", "UNAV"}
	widths := make([]int, len(headers))
	for i := range widths {
		widths[i] = len(headers[i])
	}
	labelWidth, core, hasRows := 0, 0, false
	for _, l := range lines {
		if l.counts == nil {
			continue
		}
		hasRows = true
		labelWidth = max(labelWidth, lipgloss.Width(l.label))
		for i := range widths {
			widths[i] = max(widths[i], lipgloss.Width(l.counts[i]))
		}
		core = max(core, lipgloss.Width(r.rolloutCore(l.rollout, true)))
	}
	if !hasRows {
		out := make([]string, len(lines))
		for i, l := range lines {
			out[i] = l.label
		}
		return out
	}

	// The rollout column keeps a minimal bar, the percentage, the phase and
	// its age; labels shrink to make room, down to labelMinWidth.
	fixed := 2
	for _, w := range widths {
		fixed += 2 + w
	}
	barOverhead := lipgloss.Width(r.g.barOpen+r.g.barClose) + 1
	labelWidth = min(labelWidth, max(min(labelWidth, labelMinWidth), inner-fixed-barMinWidth-barOverhead-core))
	room := inner - labelWidth - fixed
	bar := min(max(room-barOverhead-core, barMinWidth), barWidth)

	row := func(label string, counts []string, ro string) string {
		label = r.truncate(label, labelWidth)
		var b strings.Builder
		b.WriteString(label + strings.Repeat(" ", labelWidth-lipgloss.Width(label)))
		for i, c := range counts {
			b.WriteString("  " + strings.Repeat(" ", widths[i]-lipgloss.Width(c)) + c)
		}
		b.WriteString("  " + ro)
		return b.String()
	}
	out := []string{r.paint(toneDim, row("", headers, "ROLLOUT"))}
	for _, l := range lines {
		if l.counts == nil {
			out = append(out, l.label)
		} else {
			out = append(out, row(l.label, l.counts, r.rolloutCell(l.rollout, bar, room)))
		}
	}
	return out
}

// rolloutCore is the percentage and the phase, with the phase age when
// withSince is set; the bar is not included.
func (r *renderer) rolloutCore(c rowRollout, withSince bool) string {
	if c.missing != "" {
		return c.missing
	}
	parts := r.indicatorParts(c.ro, 0, withSince)
	return parts[0]
}

// rolloutCell draws a ROLLOUT cell no wider than room. The bar, percentage
// and phase are always drawn; the phase age, last progress, deadline and
// strategy follow, in that order, while they fit.
func (r *renderer) rolloutCell(c rowRollout, bar, room int) string {
	if c.missing != "" {
		return c.missing
	}
	parts := r.indicatorParts(c.ro, bar, true)
	out := parts[0]
	if lipgloss.Width(out) > room {
		return r.indicatorParts(c.ro, bar, false)[0]
	}
	var optional []string
	for _, p := range parts[1:] {
		optional = append(optional, r.sep()+p)
	}
	if c.strategy != "" {
		optional = append(optional, " "+r.paint(toneDim, "("+c.strategy+")"))
	}
	for _, p := range optional {
		if lipgloss.Width(out)+lipgloss.Width(p) > room {
			break
		}
		out += p
	}
	return out
}

func (r *renderer) badgeSymbol(b dashboard.Badge) string {
	sym, tn := r.badge(b)
	return r.paint(tn, sym)
}

func (r *renderer) ddaiItem(d *dashboard.DDAIView, suffix string, components bool) item {
	it := item{label: r.badgeSymbol(d.Health) + " " + r.paint(toneDim, "DDAI") + " " + d.Name + suffix}
	if d.Agent != nil {
		it.children = append(it.children, r.workloadItem("agent", d.Agent))
	} else {
		it.children = append(it.children, item{label: r.paint(toneDim, r.g.disabled+" agent none")})
	}
	for _, c := range []struct {
		tag string
		w   *dashboard.WorkloadView
	}{
		{"DCA", d.ClusterAgent},
		{"CCR", d.ClusterChecksRunner},
		{"OTel", d.OtelAgentGateway},
	} {
		switch {
		case c.w != nil:
			it.children = append(it.children, r.workloadItem(c.tag, c.w))
		case components:
			// A disabled component shows as "–".
			it.children = append(it.children, item{label: r.paint(toneDim, r.g.disabled+" "+c.tag+" not enabled")})
		}
	}
	return it
}

func (r *renderer) workloadItem(tag string, w *dashboard.WorkloadView) item {
	it := item{label: r.badgeSymbol(w.Health) + " " + r.paint(toneDim, tag) + " " + w.Name}
	if w.Missing {
		it.counts = []string{"", "", ""}
		it.rollout = rowRollout{missing: r.paint(toneError, r.g.err+" missing")}
		return it
	}
	c := w.Counts
	unavailable := strconv.Itoa(int(c.Unavailable))
	if c.Unavailable > 0 {
		unavailable = r.paint(toneWarn, unavailable)
	}
	it.counts = []string{
		fmt.Sprintf("%d/%d", c.Ready, c.Desired),
		fmt.Sprintf("%d/%d", c.UpToDate, c.Desired),
		unavailable,
	}
	it.rollout = rowRollout{ro: w.Rollout}
	// The DaemonSet strategy.
	if w.Kind == "DaemonSet" {
		it.rollout.strategy = w.Strategy
	}
	for i, p := range w.FailingPods {
		if i == maxFailingPods {
			it.details = append(it.details, r.paint(toneDim, fmt.Sprintf("%s %d more failing pods", r.g.ellipsis, len(w.FailingPods)-i)))
			break
		}
		s := r.paint(toneError, r.g.err) + " pod " + p.Name
		if p.Node != "" {
			s += " on " + p.Node
		}
		it.details = append(it.details, s+": "+p.Reason)
	}
	return it
}

func (r *renderer) dapItem(p *dashboard.DAPView) item {
	status := "valid " + r.flag(p.Valid) + " applied " + r.flag(p.Applied)
	if p.StatusUnknown {
		status = r.paint(toneDim, "status unknown (profiles controller not running?)")
	}
	parts := []string{
		r.badgeSymbol(p.Health) + " " + r.paint(toneDim, "DAP") + " " + p.Namespace + "/" + p.Name,
		status,
	}
	if p.Provider != nil && p.Provider.Name != "" {
		parts = append(parts, r.dimLabel("provider", r.provider(*p.Provider)))
	}
	if p.Helm != nil {
		parts = append(parts, r.helm(p.Helm)...)
	}
	it := item{label: strings.Join(parts, r.sep())}
	if p.DDAI != nil {
		it.children = append(it.children, r.ddaiItem(p.DDAI, "", false))
	} else {
		// No DDAI, so no DaemonSet row; the conditions say why.
		it.children = append(it.children, item{label: r.paint(toneDim, r.g.disabled+" no DDAI")})
	}
	return it
}

// hiddenProfiles is the note replacing the profiles hidden by --hide-empty
// or --hide-empty-force. With --hide-empty-force it
// counts the hidden profiles without and with warnings, in the longest
// wording that fits room.
func (r *renderer) hiddenProfiles(d *dashboard.DDAView, room int) string {
	n := d.HiddenProfiles
	noun := "profiles"
	if n == 1 {
		noun = "profile"
	}
	sep := " " + r.g.sep + " "
	if d.HiddenBy != dashboard.FilterHideEmptyForce {
		return fmt.Sprintf("%s %d %s hidden (0 desired)%s--hide-empty", r.g.disabled, n, noun, sep)
	}
	groups := func(without, with string) string {
		var parts []string
		if c := d.HiddenProfilesWithoutWarnings; c > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c, without))
		}
		if c := d.HiddenProfilesWithWarnings; c > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c, with))
		}
		return strings.Join(parts, sep)
	}
	candidates := []string{
		fmt.Sprintf("%d %s hidden (0 desired): %s", n, noun, groups("without warnings", "with warnings")),
		fmt.Sprintf("%d hidden (0 desired): %s", n, groups("without warnings", "with warnings")),
		fmt.Sprintf("%d hidden (0 desired): %s", n, groups("no warnings", "with warnings")),
		fmt.Sprintf("%d hidden: %s", n, groups("no warnings", "with warnings")),
		fmt.Sprintf("%d hidden: %s", n, groups("ok", "warn")),
	}
	var line string
	for _, c := range candidates {
		line = r.g.disabled + " " + c + sep + "--hide-empty-force"
		if lipgloss.Width(line) <= room {
			break
		}
	}
	return line
}

// flag renders a "True"/"False" status with a symbol.
func (r *renderer) flag(status string) string {
	switch status {
	case "True":
		return r.paint(toneOK, r.g.ok)
	case "False":
		return r.paint(toneError, r.g.err)
	default:
		return r.paint(toneDim, r.g.unknown)
	}
}

func workloadTag(w *dashboard.WorkloadView) string {
	switch w.Component {
	case dashboard.ComponentAgent:
		return "agent"
	case dashboard.ComponentClusterAgent:
		return "DCA"
	case dashboard.ComponentClusterChecksRunner:
		return "CCR"
	case dashboard.ComponentOtelAgentGateway:
		return "OTel"
	}
	if w.Kind == "DaemonSet" {
		return "DS"
	}
	return w.Kind
}

// indicatorParts renders a rollout: the bar (bar
// cells, 0 for none), percentage of updated and ready pods and phase (with
// its age when withSince is set), then the last progress and the deadline
// when they apply. A rollout with nothing to roll and an Unknown phase has
// no bar nor percentage.
func (r *renderer) indicatorParts(ro dashboard.Rollout, bar int, withSince bool) []string {
	phase := cmp.Or(ro.Phase, string(rollout.PhaseUnknown))
	var parts []string
	if ro.Desired > 0 || phase != string(rollout.PhaseUnknown) {
		if bar > 0 {
			parts = append(parts, r.bar(ro, bar))
		}
		parts = append(parts, fmt.Sprintf("%3d%%", ro.Percent))
	}

	if age := r.phaseAge(ro); withSince && age != "" {
		phase += " " + age
	}
	parts = append(parts, r.paint(phaseTone(ro.Phase), phase))
	out := []string{strings.Join(parts, " ")}
	if isDone(ro) {
		return out
	}
	if ro.LastProgress != nil {
		out = append(out, r.paint(toneDim, "last progress "+r.ago(*ro.LastProgress)+" ago"))
	}
	if ro.Deadline != nil {
		if ro.Deadline.After(r.now) {
			out = append(out, r.paint(toneDim, "deadline in "+r.until(*ro.Deadline)))
		} else {
			out = append(out, r.paint(toneDim, "deadline "+r.ago(*ro.Deadline)+" ago"))
		}
	}
	return out
}

// phaseAge is the time since the rollout started, or the settling time
// left of a Settling rollout; empty when it is done or its start is
// unknown.
func (r *renderer) phaseAge(ro dashboard.Rollout) string {
	if ro.Phase == string(rollout.PhaseSettling) && ro.SettlingUntil != nil {
		return r.until(*ro.SettlingUntil) + " left"
	}
	if isDone(ro) || ro.Since == nil {
		return ""
	}
	if ro.SinceApprox {
		return r.g.approx + r.ago(*ro.Since)
	}
	return r.ago(*ro.Since)
}

func isDone(ro dashboard.Rollout) bool {
	return ro.Phase == string(rollout.PhaseComplete) || ro.Phase == string(rollout.PhaseSucceeded)
}

// bar draws width cells: updated and ready pods in green, updated pods not
// ready yet in cyan, the rest empty. Each part has its own glyph so that
// color is never the only signal.
func (r *renderer) bar(ro dashboard.Rollout, width int) string {
	r.barDrawn = true
	ready, updated := barCells(ro, width)
	return r.g.barOpen +
		r.paint(toneOK, strings.Repeat(r.g.barFull, ready)) +
		r.paint(toneProgress, strings.Repeat(r.g.barPartial, updated-ready)) +
		r.paint(toneDim, strings.Repeat(r.g.barEmpty, width-updated)) +
		r.g.barClose
}

// barCells returns how many of width cells are updated and ready, and
// updated.
func barCells(ro dashboard.Rollout, width int) (ready, updated int) {
	if ro.Desired <= 0 {
		updated = min(max(ro.Percent, 0), 100) * width / 100
		return updated, updated
	}
	cells := func(n int32) int { return int(min(max(int64(n), 0)*int64(width)/int64(ro.Desired), int64(width))) }
	updated, ready = cells(ro.Updated), cells(ro.UpdatedReady)
	// Rounding must not hide the updated pods that are not ready.
	if ro.UpdatedReady < ro.Updated && ready >= updated && updated > 0 {
		ready = updated - 1
	}
	return min(ready, updated), updated
}

// conditions lists the Error and Warning issues first, sorted by the View,
// then the Info conditions in tree order. Each is shown once, on the object
// that owns it.
func (r *renderer) conditions(v dashboard.View) []string {
	conds := slices.Clone(v.Issues)
	for _, c := range treeConditions(v) {
		if c.Severity == dashboard.SeverityInfo {
			conds = append(conds, c)
		}
	}
	if len(conds) == 0 {
		return []string{r.paint(toneOK, r.g.ok+" no issues")}
	}
	out := make([]string, 0, len(conds))
	for _, c := range conds {
		sym, word, tn := r.severity(c.Severity)
		out = append(out, r.paint(tn, fmt.Sprintf("%s %-5s", sym, word))+"  "+r.object(c.Object)+"  "+conditionText(c))
	}
	return out
}

func treeConditions(v dashboard.View) []dashboard.Cond {
	var out []dashboard.Cond
	workload := func(w *dashboard.WorkloadView) {
		if w != nil {
			out = append(out, w.Conditions...)
		}
	}
	ddai := func(d *dashboard.DDAIView) {
		if d == nil {
			return
		}
		out = append(out, d.Conditions...)
		workload(d.Agent)
		workload(d.ClusterAgent)
		workload(d.ClusterChecksRunner)
		workload(d.OtelAgentGateway)
	}
	out = append(out, v.DDA.Conditions...)
	ddai(v.DDA.Default)
	for i := range v.DDA.Profiles {
		out = append(out, v.DDA.Profiles[i].Conditions...)
		ddai(v.DDA.Profiles[i].DDAI)
	}
	if u := v.Unattached; u != nil {
		for i := range u.DDAIs {
			ddai(&u.DDAIs[i])
		}
		for i := range u.Workloads {
			workload(&u.Workloads[i])
		}
	}
	return out
}

var kindAbbrev = map[string]string{
	"DatadogAgent":         "DDA",
	"DatadogAgentInternal": "DDAI",
	"DatadogAgentProfile":  "DAP",
	"DaemonSet":            "DS",
	"Deployment":           "Deploy",
}

func (r *renderer) object(o dashboard.ObjectRef) string {
	kind := o.Kind
	if k, ok := kindAbbrev[kind]; ok {
		kind = k
	}
	name := o.Name
	if o.Namespace != "" && o.Namespace != r.ns {
		name = o.Namespace + "/" + name
	}
	return r.paint(toneDim, kind) + " " + name
}

// conditionText is "Type[=Status] (Reason): message" on one line. The status
// is omitted when True, the reason when it only restates the type.
func conditionText(c dashboard.Cond) string {
	s := c.Type
	if c.Status != "" && c.Status != "True" {
		s += "=" + c.Status
	}
	if c.Reason != "" && normalize(c.Reason) != normalize(c.Type) {
		s += " (" + c.Reason + ")"
	}
	if c.Message != "" {
		s += ": " + c.Message
	}
	return s
}

func normalize(s string) string {
	return strings.ToLower(strings.NewReplacer("_", "", " ", "", "-", "").Replace(s))
}

// summary draws the other Datadog CRs row and their failures.
func (r *renderer) summary(rows []dashboard.SummaryView) {
	if len(rows) == 0 {
		return
	}
	items := []string{r.paint(toneBold, "Other resources")}
	for _, row := range rows {
		items = append(items, r.summaryItem(row))
	}
	for _, l := range r.wrap(items, r.width, "  ") {
		r.line(l)
	}

	shown, total := 0, 0
	for _, row := range rows {
		for _, f := range row.Failures {
			total++
			if shown == maxSummaryFailures {
				continue
			}
			shown++
			s := "  " + r.paint(toneError, r.g.err) + " " + row.Label + " " + f.Namespace + "/" + f.Name
			if f.Message != "" {
				s += ": " + f.Message
			}
			r.line(s)
		}
	}
	if total > shown {
		r.line("  " + r.paint(toneDim, fmt.Sprintf("%s %d more (see -o json)", r.g.ellipsis, total-shown)))
	}
}

// monitorStates is the display order of the monitor state breakdown.
var monitorStates = []string{"OK", "Alert", "Warn", "No Data"}

func (r *renderer) summaryItem(row dashboard.SummaryView) string {
	switch row.State {
	case dashboard.SourceOK:
	case dashboard.SourceForbidden:
		return row.Label + " " + r.paint(toneWarn, r.g.warn+" no access")
	case dashboard.SourceError, dashboard.SourceDisconnected:
		return row.Label + " " + r.paint(toneError, r.g.err+" "+strings.ToLower(string(row.State)))
	default:
		return row.Label + " " + r.paint(toneDim, strings.ToLower(string(row.State)))
	}

	var parts []string
	keys := make([]string, 0, len(row.Breakdown))
	for k := range row.Breakdown {
		if !slices.Contains(monitorStates, k) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range append(slices.Clone(monitorStates), keys...) {
		if n := row.Breakdown[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, n))
		}
	}
	if row.Errors > 0 {
		parts = append(parts, r.paint(toneError, fmt.Sprintf("%s %d", r.g.err, row.Errors)))
	}
	if row.Unknown > 0 {
		parts = append(parts, r.paint(toneDim, fmt.Sprintf("%s %d", r.g.unknown, row.Unknown)))
	}
	s := fmt.Sprintf("%s %d", row.Label, row.Total)
	if len(parts) > 0 {
		s += " (" + strings.Join(parts, " ") + ")"
	}
	return s
}

// notices draws the notes about partial data and the core sources that
// could not be read; summary sources are reported by the summary row.
func (r *renderer) notices(v dashboard.View) {
	for _, n := range v.Notices {
		r.line(r.paint(toneDim, r.g.info) + " " + n)
	}
	for _, s := range v.Sources {
		if dashboard.IsSummarySource(s.Key) || s.State == dashboard.SourceOK || s.State == dashboard.SourceNotInstalled {
			continue
		}
		l := r.paint(toneWarn, r.g.warn) + " " + s.Key + ": " + string(s.State)
		if s.Err != "" {
			l += ": " + s.Err
		}
		r.line(l)
	}
}
