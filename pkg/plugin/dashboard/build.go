// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/rollout"
)

const (
	// profileLabel links a profile DDAI to its DAP.
	profileLabel = "agent.datadoghq.com/datadogagentprofile"
	// providerAnnotation sets the cluster provider on a DDA or DAP.
	providerAnnotation = "agent.datadoghq.com/cluster-provider"

	kindDDA  = "DatadogAgent"
	kindDDAI = "DatadogAgentInternal"
	kindDAP  = "DatadogAgentProfile"
)

var (
	// ErrNoDDA is returned when no DatadogAgent is in scope.
	ErrNoDDA = errors.New("no DatadogAgent found")
	// ErrMultipleDDAs is matched by MultipleDDAsError.
	ErrMultipleDDAs = errors.New("more than one DatadogAgent found")
	// ErrUnsupportedOperator is returned when the DDA or DDAI CRD is missing.
	// The DAP CRD is optional.
	ErrUnsupportedOperator = errors.New("unsupported operator version: the DatadogAgentInternal CRD is required (operator >= 1.21)")
)

// MultipleDDAsError lists the DatadogAgents in scope when there is more than
// one and none was named.
type MultipleDDAsError struct {
	// Names are "<namespace>/<name>", sorted.
	Names []string
}

func (e *MultipleDDAsError) Error() string {
	return fmt.Sprintf("%s: %s; pass a DatadogAgent name", ErrMultipleDDAs, strings.Join(e.Names, ", "))
}

// Is makes errors.Is(err, ErrMultipleDDAs) true.
func (e *MultipleDDAsError) Is(target error) bool { return target == ErrMultipleDDAs }

// BuildConfig configures Build.
type BuildConfig struct {
	// DDAName selects the DDA by name; empty means the only one in scope.
	DDAName string
	// Pods is true when agent pods are watched (--pods). It enables Stalled
	// detection and failing pod details.
	Pods bool
	// StallAfter is the heuristic stall threshold (--stall-after).
	StallAfter time.Duration
	// MaxUnavailable is the runtime availability threshold of every
	// DaemonSet and Deployment (--max-unavailable): a converged workload
	// with more unavailable pods is Settling, then Degraded. The zero value
	// tolerates none.
	MaxUnavailable rollout.MaxUnavailable
	// Evaluator overrides the rollout evaluator. It defaults to a Chain
	// holding the heuristic evaluator.
	Evaluator rollout.Evaluator
	// HideEmpty hides the healthy profiles whose agent DaemonSet has no
	// desired pods (--hide-empty).
	HideEmpty bool
	// HideEmptyForce implies HideEmpty and also hides the profiles with no
	// desired pods whatever their conditions, with their issues
	// (--hide-empty-force).
	HideEmptyForce bool
	// Sort orders the profiles (--sort); empty means SortName.
	Sort ProfileSort
}

// Build turns a snapshot into a View. It does no I/O and only reads the
// clock through now. When several DDAs are in scope and none is named, the
// View only has the header, the sources and View.MultipleDDAs.
func Build(s *Snapshot, cfg BuildConfig, obs Observer, now time.Time) (View, error) {
	if cfg.Sort != "" {
		if _, err := ParseProfileSort(string(cfg.Sort)); err != nil {
			return View{}, err
		}
	}
	for _, key := range []string{SourceDDA, SourceDDAI} {
		if s.source(key).State == SourceNotInstalled {
			return View{}, ErrUnsupportedOperator
		}
	}
	if info := s.source(SourceDDA); !info.HasData() {
		return View{}, fmt.Errorf("cannot read DatadogAgents (%s): %s", info.State, info.Err)
	}
	ddaObj, err := selectDDA(s.Objects[SourceDDA], cfg.DDAName)
	var multi *MultipleDDAsError
	if errors.As(err, &multi) {
		return multipleDDAsView(s, multi.Names, now), nil
	}
	if err != nil {
		return View{}, err
	}
	if obs == nil {
		obs = NopObserver{}
	}
	eval := cfg.Evaluator
	if eval == nil {
		eval = rollout.Chain{rollout.HeuristicEvaluator{StallAfter: cfg.StallAfter}}
	}
	b := &builder{snap: s, cfg: cfg, obs: obs, now: now, eval: eval, claimed: map[string]bool{}}
	return b.build(ddaObj), nil
}

// multipleDDAsView is the View shown when several DDAs are in scope. The
// other sources are not read until a DDA is selected, so it has no summary.
func multipleDDAsView(s *Snapshot, names []string, now time.Time) View {
	v := View{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   now.UTC(),
		Header: Header{
			Context:       s.Cluster.Context,
			ServerVersion: s.Cluster.ServerVersion,
			PluginVersion: s.Cluster.PluginVersion,
			RefreshedAt:   now.UTC(),
		},
		MultipleDDAs: names,
		Issues:       []Cond{},
		Summary:      []SummaryView{},
	}
	v.Sources, v.Header.NextPoll = sourceViews(s)
	return v
}

// selectDDA picks the DDA.
func selectDDA(ddas []*unstructured.Unstructured, name string) (*unstructured.Unstructured, error) {
	var matches []*unstructured.Unstructured
	for _, d := range ddas {
		if name == "" || d.GetName() == name {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 0:
		if name != "" {
			return nil, fmt.Errorf("%w: %s", ErrNoDDA, name)
		}
		return nil, ErrNoDDA
	case 1:
		return matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.GetNamespace() + "/" + m.GetName()
		}
		slices.Sort(names)
		return nil, &MultipleDDAsError{Names: names}
	}
}

// Typed views of the operator objects. Only metadata and status are
// decoded; unknown fields are ignored, which tolerates version skew.
type ddaObject struct {
	metav1.ObjectMeta `json:"metadata"`

	Status v2alpha1.DatadogAgentStatus `json:"status"`
}

type ddaiObject struct {
	metav1.ObjectMeta `json:"metadata"`

	Status v1alpha1.DatadogAgentInternalStatus `json:"status"`
}

type dapObject struct {
	metav1.ObjectMeta `json:"metadata"`

	Status v1alpha1.DatadogAgentProfileStatus `json:"status"`
}

func fromUnstructured(u *unstructured.Unstructured, out any) error {
	return runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, out)
}

// node is a tree object whose conditions and health are filled in once all
// conditions are deduplicated.
type node struct {
	owner    *condOwner
	facts    HealthFacts
	children []*node
	conds    *[]Cond
	health   *Badge
	// selfUnknown makes this node's badge Unknown at best without
	// propagating to its ancestors (optional data).
	selfUnknown bool
}

type builder struct {
	snap    *Snapshot
	cfg     BuildConfig
	obs     Observer
	now     time.Time
	eval    rollout.Evaluator
	raws    []rawCond
	nodes   []*node
	notices []string
	// claimed holds the IDs of the workloads attached to a DDAI.
	claimed map[string]bool
	// deferred runs once health is computed, to copy views by value.
	deferred []func()
	// podsByOwner indexes the snapshot pods by owner; see podsOf.
	podsByOwner map[podOwner][]int
}

func (b *builder) newNode(ref ObjectRef, parent *node, ddai string, isDDAI bool, conds *[]Cond, health *Badge) *node {
	o := &condOwner{ref: ref, ddai: ddai, isDDAI: isDDAI}
	if parent != nil {
		o.path = slices.Clone(parent.owner.path)
	}
	o.path = append(o.path, o.id())
	n := &node{owner: o, conds: conds, health: health}
	if parent != nil {
		parent.children = append(parent.children, n)
	}
	b.nodes = append(b.nodes, n)
	return n
}

func (b *builder) addConds(n *node, conds []metav1.Condition) {
	for _, c := range conds {
		b.raws = append(b.raws, rawCond{owner: n.owner, cond: c})
	}
}

func (b *builder) build(ddaUnstructured *unstructured.Unstructured) View {
	s := b.snap
	var dda ddaObject
	if err := fromUnstructured(ddaUnstructured, &dda); err != nil {
		b.notices = append(b.notices, fmt.Sprintf("DatadogAgent status could not be decoded: %v", err))
		dda.ObjectMeta = metav1.ObjectMeta{Name: ddaUnstructured.GetName(), Namespace: ddaUnstructured.GetNamespace(), UID: ddaUnstructured.GetUID()}
	}

	v := View{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   b.now.UTC(),
		Header: Header{
			Context:       s.Cluster.Context,
			Namespace:     dda.Namespace,
			ServerVersion: s.Cluster.ServerVersion,
			PluginVersion: s.Cluster.PluginVersion,
			RefreshedAt:   b.now.UTC(),
			Provider:      ddaProvider(&dda),
			Operator:      buildOperator(s, dda.Namespace),
		},
		DDA: DDAView{
			Namespace: dda.Namespace,
			Name:      dda.Name,
			Created:   timePtr(dda.CreationTimestamp.Time),
		},
	}
	root := b.newNode(ObjectRef{Kind: kindDDA, Namespace: dda.Namespace, Name: dda.Name}, nil, "", false, &v.DDA.Conditions, &v.DDA.Health)
	b.addConds(root, dda.Status.Conditions)
	if exp := dda.Status.Experiment; exp != nil && exp.Phase != "" {
		v.DDA.Experiment = &ExperimentView{Phase: string(exp.Phase), ID: exp.ID}
		if exp.StartedAt != nil {
			v.DDA.Experiment.StartedAt = timePtr(exp.StartedAt.Time)
		}
		root.facts.Experiment = exp.Phase == v2alpha1.ExperimentPhaseRunning
	}
	for _, key := range []string{SourceDaemonSets, SourceDeployments, SourceControllerRevisions, SourceReplicaSets} {
		if !s.source(key).HasData() {
			root.facts.Unknown = true
		}
	}

	defaultDDAIs, profileDDAIs, unattachedDDAIs := b.classifyDDAIs(&dda)
	// The default DDAI is named after the DDA; extra ones are unattached.
	slices.SortStableFunc(defaultDDAIs, func(a, _ *ddaiObject) int {
		if a.Name == dda.Name {
			return -1
		}
		return 0
	})
	var results []rollout.Result
	if len(defaultDDAIs) > 0 {
		var res []rollout.Result
		v.DDA.Default, res = b.buildDDAI(defaultDDAIs[0], root, "")
		results = append(results, res...)
		root.ddaiName(defaultDDAIs[0].Name)
		unattachedDDAIs = append(unattachedDDAIs, defaultDDAIs[1:]...)
	}

	// DAPs, by name, each with its DDAI.
	daps := make([]*DAPView, 0, len(s.Objects[SourceDAP]))
	for _, u := range sortedDAPs(s.Objects[SourceDAP], dda.Namespace) {
		dap := &dapObject{}
		if err := fromUnstructured(u, dap); err != nil {
			b.notices = append(b.notices, fmt.Sprintf("DatadogAgentProfile %s status could not be decoded: %v", u.GetName(), err))
			dap.ObjectMeta = metav1.ObjectMeta{Name: u.GetName(), Namespace: u.GetNamespace(), Annotations: u.GetAnnotations(), Labels: u.GetLabels()}
		}
		dv := &DAPView{
			Namespace: dap.Namespace,
			Name:      dap.Name,
			Valid:     string(dap.Status.Valid),
			Applied:   string(dap.Status.Applied),
			// No status at all: the profile controller is likely not
			// running.
			StatusUnknown: dap.Status.Valid == "" && dap.Status.Applied == "" && len(dap.Status.Conditions) == 0,
		}
		if p := dap.Annotations[providerAnnotation]; p != "" {
			dv.Provider = &ProviderView{Name: p, Source: ProviderUserSet}
		}
		ddais := profileDDAIs[dap.Name]
		delete(profileDDAIs, dap.Name)
		ddaiName := ""
		if len(ddais) > 0 {
			ddaiName = ddais[0].Name
			unattachedDDAIs = append(unattachedDDAIs, ddais[1:]...)
		}
		n := b.newNode(ObjectRef{Kind: kindDAP, Namespace: dap.Namespace, Name: dap.Name}, root, ddaiName, false, &dv.Conditions, &dv.Health)
		n.selfUnknown = dv.StatusUnknown
		b.addConds(n, dap.Status.Conditions)
		var start time.Time
		if len(ddais) > 0 {
			var res []rollout.Result
			dv.DDAI, res = b.buildDDAI(ddais[0], n, dap.Name)
			results = append(results, res...)
			start = rolloutStart(dv.DDAI.Rollout)
		}
		dv.Helm = buildHelm(s, dap, start)
		daps = append(daps, dv)
	}
	for _, ddais := range profileDDAIs {
		unattachedDDAIs = append(unattachedDDAIs, ddais...) // no DAP
	}

	// Rollups.
	v.DDA.Rollout = rolloutView(rollout.WorstOf(results))
	v.DDA.Agents = b.agentTotals(v.DDA.Default, daps)
	v.DDA.Helm = buildHelm(s, &dda, rolloutStart(v.DDA.Rollout))

	v.Unattached = b.buildUnattached(unattachedDDAIs, dda.Name)

	// Conditions, then health bottom-up.
	conds := dedup(b.raws)
	byOwner := map[string]*node{}
	for _, n := range b.nodes {
		byOwner[n.owner.id()] = n
	}
	for _, c := range conds {
		if n := byOwner[c.Object.String()]; n != nil {
			*n.conds = append(*n.conds, c)
		}
		if c.Severity != SeverityInfo {
			v.Issues = append(v.Issues, c)
		}
	}
	if v.Issues == nil {
		v.Issues = []Cond{}
	}
	for _, n := range b.nodes {
		if len(n.owner.path) == 1 {
			finalize(n)
		}
	}

	for _, f := range b.deferred {
		f()
	}
	// Health and totals are computed above, hidden profiles included.
	shown, hidden := arrangeProfiles(daps, b.cfg)
	for _, d := range shown {
		v.DDA.Profiles = append(v.DDA.Profiles, *d)
	}
	if len(hidden) > 0 {
		v.DDA.HiddenBy = FilterHideEmpty
		if b.cfg.HideEmptyForce {
			v.DDA.HiddenBy = FilterHideEmptyForce
		}
		var withIssues int
		v.Issues, withIssues, v.DDA.HiddenIssues = hideProfileIssues(v.Issues, hidden)
		v.DDA.HiddenProfiles = len(hidden)
		v.DDA.HiddenProfilesWithWarnings = withIssues
		v.DDA.HiddenProfilesWithoutWarnings = len(hidden) - withIssues
		for _, d := range hidden {
			v.DDA.HiddenProfileNames = append(v.DDA.HiddenProfileNames, d.Namespace+"/"+d.Name)
		}
	}
	v.Summary = buildSummary(s)
	v.Sources, v.Header.NextPoll = sourceViews(s)
	for _, src := range v.Sources {
		// Sources may share a notice, e.g. the summary kinds.
		if src.Notice != "" && !slices.Contains(v.Notices, src.Notice) {
			v.Notices = append(v.Notices, src.Notice)
		}
	}
	v.Notices = append(v.Notices, b.notices...)
	return v
}

// classifyDDAIs splits the DDAIs of the DDA namespace by owner and profile
// label: default DDAIs and profile DDAIs owned by the DDA, and unattached
// ones. DDAIs owned by another DDA in scope are skipped.
func (b *builder) classifyDDAIs(dda *ddaObject) (defaults []*ddaiObject, profiles map[string][]*ddaiObject, unattached []*ddaiObject) {
	ownedByOtherDDA := func(ref *metav1.OwnerReference) bool {
		for _, d := range b.snap.Objects[SourceDDA] {
			other := metav1.ObjectMeta{Name: d.GetName(), UID: d.GetUID()}
			if d.GetNamespace() == dda.Namespace && d.GetName() != dda.Name && ownedBy(ref, &other) {
				return true
			}
		}
		return false
	}
	profiles = map[string][]*ddaiObject{}
	for _, u := range sortedObjects(b.snap.Objects[SourceDDAI]) {
		if u.GetNamespace() != dda.Namespace {
			continue
		}
		ddai := &ddaiObject{}
		if err := fromUnstructured(u, ddai); err != nil {
			b.notices = append(b.notices, fmt.Sprintf("DatadogAgentInternal %s status could not be decoded: %v", u.GetName(), err))
			ddai.ObjectMeta = metav1.ObjectMeta{Name: u.GetName(), Namespace: u.GetNamespace(), UID: u.GetUID(), Labels: u.GetLabels(), OwnerReferences: u.GetOwnerReferences()}
		}
		switch owner := ddaOwner(ddai.OwnerReferences); {
		case owner != nil && ownedBy(owner, &dda.ObjectMeta):
			if p := ddai.Labels[profileLabel]; p != "" {
				profiles[p] = append(profiles[p], ddai)
			} else {
				defaults = append(defaults, ddai)
			}
		case owner != nil && ownedByOtherDDA(owner):
		default:
			unattached = append(unattached, ddai)
		}
	}
	return defaults, profiles, unattached
}

// ddaiName records the default DDAI on the DDA owner, for mirror dedup.
func (n *node) ddaiName(name string) { n.owner.ddai = name }

// finalize computes the health of a subtree from its own facts, its
// conditions and its children.
func finalize(n *node) HealthFacts {
	f := n.facts
	f.Phases = slices.Clone(f.Phases)
	for _, c := range *n.conds {
		f.addSeverity(c.Severity)
	}
	for _, child := range n.children {
		f.Add(finalize(child))
	}
	own := f
	own.Unknown = own.Unknown || n.selfUnknown
	*n.health = Health(own)
	return f
}

// rolloutStart is the start of a rollout in progress; zero when the
// rollout is done or its start unknown.
func rolloutStart(ro Rollout) time.Time {
	s := rollout.Phase(ro.Phase).Severity()
	if s == rollout.SeverityDone || s == rollout.SeverityUnknown || ro.Since == nil {
		return time.Time{}
	}
	return *ro.Since
}

// ddaProvider returns the cluster provider of the DDA: the annotation wins,
// then the detected status.
func ddaProvider(dda *ddaObject) ProviderView {
	if p := dda.Annotations[providerAnnotation]; p != "" {
		return ProviderView{Name: p, Source: ProviderUserSet}
	}
	pv := ProviderView{Name: dda.Status.ClusterProvider}
	for _, c := range dda.Status.Conditions {
		if c.Type == condClusterProviderDetected {
			pv.Source, pv.Reason = ProviderDetected, c.Reason
		}
	}
	if pv.Name != "" {
		pv.Source = ProviderDetected
	}
	return pv
}

func ddaOwner(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i := range refs {
		if refs[i].Kind == kindDDA {
			return &refs[i]
		}
	}
	return nil
}

// ownedBy matches an owner reference by UID, or by name when either UID is
// unknown.
func ownedBy(ref *metav1.OwnerReference, obj *metav1.ObjectMeta) bool {
	if ref.UID != "" && obj.UID != "" {
		return ref.UID == obj.UID
	}
	return ref.Name == obj.Name
}

func sortedObjects(objs []*unstructured.Unstructured) []*unstructured.Unstructured {
	out := slices.Clone(objs)
	slices.SortFunc(out, func(a, b *unstructured.Unstructured) int {
		if c := cmp.Compare(a.GetNamespace(), b.GetNamespace()); c != 0 {
			return c
		}
		return cmp.Compare(a.GetName(), b.GetName())
	})
	return out
}

// sortedDAPs sorts DAPs by name; on equal names, the DDA namespace first.
func sortedDAPs(objs []*unstructured.Unstructured, ddaNamespace string) []*unstructured.Unstructured {
	out := slices.Clone(objs)
	slices.SortFunc(out, func(a, b *unstructured.Unstructured) int {
		if c := cmp.Compare(a.GetName(), b.GetName()); c != 0 {
			return c
		}
		if c := cmp.Compare(boolRank(a.GetNamespace() != ddaNamespace), boolRank(b.GetNamespace() != ddaNamespace)); c != 0 {
			return c
		}
		return cmp.Compare(a.GetNamespace(), b.GetNamespace())
	})
	return out
}

// buildDDAI builds a DDAI node with its workloads and returns the
// workload rollout results.
func (b *builder) buildDDAI(ddai *ddaiObject, parent *node, profile string) (*DDAIView, []rollout.Result) {
	dv := &DDAIView{Name: ddai.Name, Profile: profile}
	n := b.newNode(ObjectRef{Kind: kindDDAI, Namespace: ddai.Namespace, Name: ddai.Name}, parent, ddai.Name, true, &dv.Conditions, &dv.Health)
	b.addConds(n, ddai.Status.Conditions)
	st := ddai.Status

	var results []rollout.Result
	attach := func(component, kind, statusName string) *WorkloadView {
		wv, res := b.ddaiWorkload(ddai, n, component, kind, statusName)
		if wv != nil {
			results = append(results, res)
		}
		return wv
	}
	dsName := ""
	if st.Agent != nil {
		dsName = st.Agent.DaemonsetName
	}
	dv.Agent = attach(ComponentAgent, "DaemonSet", dsName)
	deployName := func(d *v2alpha1.DeploymentStatus) string {
		if d == nil {
			return ""
		}
		return d.DeploymentName
	}
	dv.ClusterAgent = attach(ComponentClusterAgent, "Deployment", deployName(st.ClusterAgent))
	dv.ClusterChecksRunner = attach(ComponentClusterChecksRunner, "Deployment", deployName(st.ClusterChecksRunner))
	// The OTel gateway status is only on the DDAI, never mirrored to the DDA.
	dv.OtelAgentGateway = attach(ComponentOtelAgentGateway, "Deployment", deployName(st.OtelAgentGateway))

	if dv.Agent == nil || (len(st.Conditions) == 0 && st.Agent == nil) {
		n.facts.Unknown = true
	}
	dv.Rollout = rolloutView(rollout.WorstOf(results))
	return dv, results
}

// ddaiWorkload finds a DDAI workload by the name in the DDAI status, then by
// owner reference and component label. It returns nil when the component is
// not deployed.
func (b *builder) ddaiWorkload(ddai *ddaiObject, parent *node, component, kind, statusName string) (*WorkloadView, rollout.Result) {
	key := SourceDaemonSets
	if kind == "Deployment" {
		key = SourceDeployments
	}
	var found *unstructured.Unstructured
	for _, u := range sortedObjects(b.snap.Objects[key]) {
		if u.GetNamespace() != ddai.Namespace || b.claimed[workloadID(kind, u)] {
			continue
		}
		if statusName != "" && u.GetName() == statusName {
			found = u
			break
		}
		if found == nil && ownedByKind(u.GetOwnerReferences(), kindDDAI, &ddai.ObjectMeta) &&
			(kind == "DaemonSet" || u.GetLabels()[componentLabel] == component) {
			found = u
		}
	}
	if found == nil {
		if statusName == "" {
			return nil, rollout.Result{}
		}
		// Named in the status but not found.
		wv := &WorkloadView{Kind: kind, Name: statusName, Component: component, Missing: true}
		n := b.newNode(ObjectRef{Kind: kind, Namespace: ddai.Namespace, Name: statusName}, parent, "", false, &wv.Conditions, &wv.Health)
		n.facts.Unknown = true
		res := rollout.Result{Phase: rollout.PhaseUnknown}
		wv.Rollout = rolloutView(res)
		return wv, res
	}
	b.claimed[workloadID(kind, found)] = true
	return b.workload(found, kind, component, parent)
}

const (
	nameLabel      = "agent.datadoghq.com/name"
	componentLabel = "agent.datadoghq.com/component"
)

func workloadID(kind string, u *unstructured.Unstructured) string {
	return kind + "/" + u.GetNamespace() + "/" + u.GetName()
}

// ownedByKind reports whether an owner reference points to the object of
// the kind, matched by UID or, when either UID is unknown, by name.
func ownedByKind(refs []metav1.OwnerReference, kind string, owner *metav1.ObjectMeta) bool {
	for i := range refs {
		if refs[i].Kind == kind && ownedBy(&refs[i], owner) {
			return true
		}
	}
	return false
}

// buildUnattached groups the DDAIs without a resolvable parent and the
// unclaimed workloads of the DDA.
func (b *builder) buildUnattached(ddais []*ddaiObject, ddaName string) *UnattachedView {
	uv := &UnattachedView{}
	var health Badge
	var noConds []Cond
	group := b.newNode(ObjectRef{Kind: "Unattached", Name: "unattached"}, nil, "", false, &noConds, &health)
	slices.SortFunc(ddais, func(a, b *ddaiObject) int { return cmp.Compare(a.Name, b.Name) })
	ddaiViews := make([]*DDAIView, 0, len(ddais))
	for _, d := range ddais {
		dv, _ := b.buildDDAI(d, group, d.Labels[profileLabel])
		ddaiViews = append(ddaiViews, dv)
	}
	var workloads []*WorkloadView
	for _, kw := range []struct{ key, kind string }{{SourceDaemonSets, "DaemonSet"}, {SourceDeployments, "Deployment"}} {
		for _, u := range sortedObjects(b.snap.Objects[kw.key]) {
			if b.claimed[workloadID(kw.kind, u)] {
				continue
			}
			if name, ok := u.GetLabels()[nameLabel]; ok && name != ddaName {
				continue // another DDA's workload
			}
			b.claimed[workloadID(kw.kind, u)] = true
			wv, _ := b.workload(u, kw.kind, u.GetLabels()[componentLabel], group)
			workloads = append(workloads, wv)
		}
	}
	if len(ddaiViews) == 0 && len(workloads) == 0 {
		return nil
	}
	// Health is filled in by finalize: copy the views after it.
	b.deferred = append(b.deferred, func() {
		for _, d := range ddaiViews {
			uv.DDAIs = append(uv.DDAIs, *d)
		}
		for _, w := range workloads {
			uv.Workloads = append(uv.Workloads, *w)
		}
	})
	return uv
}
