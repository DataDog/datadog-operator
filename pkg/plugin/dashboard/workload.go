// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-operator/pkg/rollout"
)

const (
	controllerRevisionHashLabel = "controller-revision-hash"
	podTemplateHashLabel        = "pod-template-hash"
	deploymentRevisionAnnot     = "deployment.kubernetes.io/revision"

	kindDaemonSet  = "DaemonSet"
	kindDeployment = "Deployment"
	kindReplicaSet = "ReplicaSet"
)

// benignWaitingReasons are container waiting reasons of a pod that is
// starting normally.
var benignWaitingReasons = map[string]bool{"ContainerCreating": true, "PodInitializing": true}

// workload builds a DaemonSet or Deployment node and evaluates its rollout.
// Rollout facts come from the workload's own status.
func (b *builder) workload(u *unstructured.Unstructured, kind, component string, parent *node) (*WorkloadView, rollout.Result) {
	wv := &WorkloadView{Kind: kind, Name: u.GetName(), Component: component}
	n := b.newNode(ObjectRef{Kind: kind, Namespace: u.GetNamespace(), Name: u.GetName()}, parent, "", false, &wv.Conditions, &wv.Health)

	var w rollout.Workload
	podOwnerKind, podOwners := kind, map[string]bool{u.GetName(): true}
	switch kind {
	case kindDaemonSet:
		var ds appsv1.DaemonSet
		if err := fromUnstructured(u, &ds); err != nil {
			return b.undecodable(wv, n, err)
		}
		st := ds.Status
		w = rollout.Workload{
			Kind: rollout.KindDaemonSet, Name: ds.Name,
			Generation: ds.Generation, ObservedGeneration: st.ObservedGeneration,
			Desired: st.DesiredNumberScheduled, Updated: st.UpdatedNumberScheduled,
			Available: st.NumberAvailable, Unavailable: st.NumberUnavailable,
			OnDelete: ds.Spec.UpdateStrategy.Type == appsv1.OnDeleteDaemonSetStrategyType,
		}
		wv.Counts = Counts{Desired: st.DesiredNumberScheduled, Ready: st.NumberReady, UpToDate: st.UpdatedNumberScheduled, Available: st.NumberAvailable, Unavailable: st.NumberUnavailable}
		wv.Strategy = daemonSetStrategy(&ds)
		w.CurrentRevision, w.RolloutStart, w.RolloutStartApprox = newestRevision(b.snap.Objects[SourceControllerRevisions], kindDaemonSet, &ds.ObjectMeta, controllerRevisionNumber, controllerRevisionHashLabel)
	case kindDeployment:
		var dep appsv1.Deployment
		if err := fromUnstructured(u, &dep); err != nil {
			return b.undecodable(wv, n, err)
		}
		st := dep.Status
		desired := deploymentDesired(&dep)
		w = rollout.Workload{
			Kind: rollout.KindDeployment, Name: dep.Name,
			Generation: dep.Generation, ObservedGeneration: st.ObservedGeneration,
			Desired: desired, Updated: st.UpdatedReplicas,
			Available: st.AvailableReplicas, Unavailable: st.UnavailableReplicas,
		}
		for _, c := range st.Conditions {
			if c.Type == appsv1.DeploymentProgressing && c.Reason == rollout.ReasonProgressDeadlineExceeded {
				w.ProgressDeadlineExceeded = true
			}
			b.raws = append(b.raws, rawCond{owner: n.owner, cond: metav1.Condition{
				Type: string(c.Type), Status: metav1.ConditionStatus(c.Status), Reason: c.Reason, Message: c.Message, LastTransitionTime: c.LastTransitionTime,
			}})
		}
		wv.Counts = Counts{Desired: desired, Ready: st.ReadyReplicas, UpToDate: st.UpdatedReplicas, Available: st.AvailableReplicas, Unavailable: st.UnavailableReplicas}
		wv.Strategy = deploymentStrategy(&dep)
		w.CurrentRevision, w.RolloutStart, w.RolloutStartApprox = newestRevision(b.snap.Objects[SourceReplicaSets], kindDeployment, &dep.ObjectMeta, replicaSetRevisionNumber, podTemplateHashLabel)
		podOwnerKind, podOwners = kindReplicaSet, ownedNames(b.snap.Objects[SourceReplicaSets], kindDeployment, &dep.ObjectMeta)
	}

	obs := b.obs.Observe(n.owner.id(), w, b.now)
	in := rollout.Input{
		Workload: w, FirstSeen: obs.FirstSeen, LastProgress: obs.LastProgress, ConvergedSince: obs.ConvergedSince,
		AvailabilityThreshold: b.availabilityThreshold(w), SettlingPeriod: rollout.DefaultSettlingPeriod,
		Now: b.now,
	}
	// Stale data is shown, but no Stalled verdict comes from it.
	in.Stale = b.snap.source(workloadSource(kind)).State == SourceDisconnected
	if pods := b.snap.source(SourcePods); b.cfg.Pods && pods.HasData() {
		in.PodsWatched = true
		in.Stale = in.Stale || pods.State == SourceDisconnected
		for _, p := range b.podsOf(u.GetNamespace(), podOwnerKind, podOwners) {
			f := podFact(p)
			in.Pods = append(in.Pods, f)
			if f.WaitingReason != "" {
				wv.FailingPods = append(wv.FailingPods, PodView{Name: f.Name, Node: f.Node, Reason: f.WaitingReason})
			}
		}
		slices.SortFunc(wv.FailingPods, func(a, b PodView) int { return cmp.Compare(a.Name, b.Name) })
	}
	res, _ := b.eval.Evaluate(in)
	wv.Rollout = rolloutView(res)
	wv.Rollout.AvailabilityThreshold = &in.AvailabilityThreshold
	n.facts.Phases = []rollout.Phase{res.Phase}

	// Synthesized conditions, so that rollout problems are listed with the
	// other issues.
	switch res.Phase.Severity() {
	case rollout.SeverityError:
		b.synth(n, SeverityError, condRollout, string(res.Phase), res.Reason)
	case rollout.SeverityDegraded:
		b.synth(n, SeverityWarning, condRollout, string(res.Phase), res.Reason)
	default:
	}
	if res.Reason == rollout.ReasonAvailabilityBelowThreshold {
		b.synth(n, SeverityWarning, condUnavailable, res.Reason, fmt.Sprintf("%d unavailable (> %d allowed)", w.Unavailable, in.AvailabilityThreshold))
	}
	return wv, res
}

// availabilityThreshold resolves the runtime availability threshold of w
// against its desired pods. The workload's own rollingUpdate.maxUnavailable
// is a rollout budget and is not used.
func (b *builder) availabilityThreshold(w rollout.Workload) int32 {
	return b.cfg.MaxUnavailable.Resolve(w.Desired)
}

// workloadSource returns the source of a workload kind.
func workloadSource(kind string) string {
	if kind == kindDeployment {
		return SourceDeployments
	}
	return SourceDaemonSets
}

func (b *builder) undecodable(wv *WorkloadView, n *node, err error) (*WorkloadView, rollout.Result) {
	b.notices = append(b.notices, fmt.Sprintf("%s %s could not be decoded: %v", wv.Kind, wv.Name, err))
	n.facts.Unknown = true
	res := rollout.Result{Phase: rollout.PhaseUnknown}
	wv.Rollout = rolloutView(res)
	return wv, res
}

func (b *builder) synth(n *node, sev Severity, typ, reason, msg string) {
	b.raws = append(b.raws, rawCond{owner: n.owner, severity: sev, cond: metav1.Condition{
		Type: typ, Status: metav1.ConditionTrue, Reason: reason, Message: msg,
	}})
}

func deploymentDesired(dep *appsv1.Deployment) int32 {
	if dep.Spec.Replicas == nil {
		return 1
	}
	return *dep.Spec.Replicas
}

// daemonSetStrategy renders the DaemonSet update strategy.
func daemonSetStrategy(ds *appsv1.DaemonSet) string {
	s := ds.Spec.UpdateStrategy
	if s.Type == appsv1.OnDeleteDaemonSetStrategyType {
		return string(s.Type)
	}
	out := string(appsv1.RollingUpdateDaemonSetStrategyType)
	if ru := s.RollingUpdate; ru != nil {
		if ru.MaxUnavailable != nil {
			out += " maxUnavailable=" + ru.MaxUnavailable.String()
		}
		if ru.MaxSurge != nil && ru.MaxSurge.String() != "0" {
			out += " maxSurge=" + ru.MaxSurge.String()
		}
	}
	return out
}

func deploymentStrategy(dep *appsv1.Deployment) string {
	s := dep.Spec.Strategy
	if s.Type == appsv1.RecreateDeploymentStrategyType {
		return string(s.Type)
	}
	out := string(appsv1.RollingUpdateDeploymentStrategyType)
	if ru := s.RollingUpdate; ru != nil {
		if ru.MaxUnavailable != nil {
			out += " maxUnavailable=" + ru.MaxUnavailable.String()
		}
		if ru.MaxSurge != nil {
			out += " maxSurge=" + ru.MaxSurge.String()
		}
	}
	return out
}

func controllerRevisionNumber(u *unstructured.Unstructured) int64 {
	n, _, _ := unstructured.NestedInt64(u.Object, "revision")
	return n
}

func replicaSetRevisionNumber(u *unstructured.Unstructured) int64 {
	n, _ := strconv.ParseInt(u.GetAnnotations()[deploymentRevisionAnnot], 10, 64)
	return n
}

// newestRevision returns the hash and creation time of the owned revision
// object (ControllerRevision or ReplicaSet) with the highest revision
// number. The start is approximate when an older revision object was created
// after it: a rollback reused it.
func newestRevision(objs []*unstructured.Unstructured, ownerKind string, owner *metav1.ObjectMeta, number func(*unstructured.Unstructured) int64, hashLabel string) (hash string, start time.Time, approx bool) {
	var owned []*unstructured.Unstructured
	for _, u := range objs {
		if u.GetNamespace() == owner.Namespace && ownedByKind(u.GetOwnerReferences(), ownerKind, owner) {
			owned = append(owned, u)
		}
	}
	if len(owned) == 0 {
		return "", time.Time{}, false
	}
	newest := slices.MaxFunc(owned, func(a, b *unstructured.Unstructured) int { return cmp.Compare(number(a), number(b)) })
	start = newest.GetCreationTimestamp().Time
	for _, u := range owned {
		if u != newest && u.GetCreationTimestamp().After(start) {
			approx = true
		}
	}
	hash = newest.GetLabels()[hashLabel]
	if hash == "" {
		hash = strings.TrimPrefix(newest.GetName(), owner.Name+"-")
	}
	return hash, start, approx
}

// ownedNames returns the names of the objects owned by owner.
func ownedNames(objs []*unstructured.Unstructured, ownerKind string, owner *metav1.ObjectMeta) map[string]bool {
	names := map[string]bool{}
	for _, u := range objs {
		if u.GetNamespace() == owner.Namespace && ownedByKind(u.GetOwnerReferences(), ownerKind, owner) {
			names[u.GetName()] = true
		}
	}
	return names
}

// podsOf returns the pods of namespace owned by an object of kind named in
// owners, in snapshot order. The pods are indexed by owner on first use, so
// a build reads each pod once.
func (b *builder) podsOf(namespace, kind string, owners map[string]bool) []*corev1.Pod {
	if b.podsByOwner == nil {
		b.podsByOwner = map[podOwner][]int{}
		for i, p := range b.snap.Pods {
			for _, r := range p.OwnerReferences {
				key := podOwner{namespace: p.Namespace, kind: r.Kind, name: r.Name}
				if idx := b.podsByOwner[key]; len(idx) == 0 || idx[len(idx)-1] != i {
					b.podsByOwner[key] = append(idx, i)
				}
			}
		}
	}
	var idx []int
	for name := range owners {
		idx = append(idx, b.podsByOwner[podOwner{namespace: namespace, kind: kind, name: name}]...)
	}
	if len(owners) > 1 {
		slices.Sort(idx)
		idx = slices.Compact(idx)
	}
	pods := make([]*corev1.Pod, len(idx))
	for i, j := range idx {
		pods[i] = b.snap.Pods[j]
	}
	return pods
}

// podOwner identifies the owner of pods.
type podOwner struct {
	namespace, kind, name string
}

// podFact reduces a pod to the facts the evaluator uses.
func podFact(p *corev1.Pod) rollout.PodFact {
	f := rollout.PodFact{
		Name:          p.Name,
		Node:          p.Spec.NodeName,
		Revision:      p.Labels[controllerRevisionHashLabel],
		Created:       p.CreationTimestamp.Time,
		WaitingReason: podFailure(p),
	}
	if f.Revision == "" {
		f.Revision = p.Labels[podTemplateHashLabel]
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			f.ReadySince = c.LastTransitionTime.Time
		}
	}
	return f
}

// podFailure returns why a pod is failing, e.g. CrashLoopBackOff,
// ImagePullBackOff, OOMKilled or Unschedulable; empty when it is not.
func podFailure(p *corev1.Pod) string {
	for _, cs := range slices.Concat(p.Status.InitContainerStatuses, p.Status.ContainerStatuses) {
		if w := cs.State.Waiting; w != nil && w.Reason != "" && !benignWaitingReasons[w.Reason] {
			return w.Reason
		}
	}
	for _, cs := range p.Status.ContainerStatuses {
		if t := cs.State.Terminated; t != nil && t.Reason != "" && t.Reason != "Completed" {
			return t.Reason
		}
	}
	if p.Status.Phase == corev1.PodPending {
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
				return firstNonEmpty(c.Reason, "Unschedulable")
			}
		}
	}
	return ""
}

// rolloutView converts an evaluator result.
func rolloutView(r rollout.Result) Rollout {
	phase := r.Phase
	if phase == "" {
		phase = rollout.PhaseUnknown
	}
	return Rollout{
		Phase:         string(phase),
		Reason:        r.Reason,
		Updated:       r.Updated,
		Desired:       r.Desired,
		UpdatedReady:  r.UpdatedReady,
		Percent:       progressPercent(r.UpdatedReady, r.Desired),
		PercentBasis:  PercentBasisUpdatedReady,
		Since:         timePtr(r.Since),
		SinceApprox:   r.SinceApprox && !r.Since.IsZero(),
		LastProgress:  timePtr(r.LastProgress),
		Deadline:      timePtr(r.Deadline),
		SettlingUntil: timePtr(r.SettlingUntil),
		StepOrder:     r.StepOrder,
		Source:        r.Source,
	}
}

// progressPercent is updatedReady/desired, rounded down and clamped to
// [0, 100]; 100 when there is nothing to roll.
func progressPercent(updatedReady, desired int32) int {
	if desired <= 0 {
		return 100
	}
	return int(min(max(int64(updatedReady), 0)*100/int64(desired), 100))
}

// agentTotals sums the node agent counts over the DDA's DaemonSets.
func (b *builder) agentTotals(def *DDAIView, daps []*DAPView) Counts {
	var total Counts
	add := func(d *DDAIView) {
		if d == nil || d.Agent == nil {
			return
		}
		c := d.Agent.Counts
		total.Desired += c.Desired
		total.Ready += c.Ready
		total.UpToDate += c.UpToDate
		total.Available += c.Available
		total.Unavailable += c.Unavailable
	}
	add(def)
	for _, d := range daps {
		add(d.DDAI)
	}
	return total
}

// sourceViews lists the sources by key and returns the earliest next poll.
func sourceViews(s *Snapshot) ([]SourceView, *time.Time) {
	keys := make([]string, 0, len(s.Sources))
	for k := range s.Sources {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]SourceView, 0, len(keys))
	var next time.Time
	for _, k := range keys {
		info := s.Sources[k]
		out = append(out, SourceView{
			Key: k, State: info.State, Mode: info.Mode,
			LastOK: timePtr(info.LastOK), NextPoll: timePtr(info.NextPoll),
			Notice: info.Notice, Err: info.Err,
		})
		if info.Mode == RefreshPoll && !info.NextPoll.IsZero() && (next.IsZero() || info.NextPoll.Before(next)) {
			next = info.NextPoll
		}
	}
	return out, timePtr(next)
}
