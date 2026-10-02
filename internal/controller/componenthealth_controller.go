// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/componenthealth"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	componentHealthControllerName = "ComponentHealth"

	// defaultRestartThreshold is the cumulative container restart count at or
	// above which a component container is flagged as crash-looping even when it
	// is not currently backing off.
	defaultRestartThreshold = 5

	// defaultSnapshotInterval is the cadence at which the current active-issue
	// set is handed to the emitter.
	defaultSnapshotInterval = 60 * time.Second
)

var _ manager.LeaderElectionRunnable = &ComponentHealthReconciler{}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// componentHealthEmitter reports component-level issues on a fixed cadence. It is
// snapshot-based: the controller periodically walks its state (see
// ComponentHealthReconciler.Start) and hands Snapshot both the full set of
// currently-active issues and the set of issues that have cleared since the last
// acknowledged snapshot, so the backend gets the complete current picture plus an
// explicit RESOLVED signal for anything that just recovered.
//
// Snapshot is always called outside r.mu: the reconciler copies its state under
// the lock and releases it before calling Snapshot, so implementations may do
// blocking network I/O without serializing reconciliation of unrelated pods
// behind a slow or retrying intake call.
//
// It returns an error when the snapshot was not delivered. On error the
// controller keeps the resolved issues pending and retries them on the next tick,
// giving at-least-once delivery of resolve signals; a nil return acknowledges
// them.
//
// The default implementation (logEmitter) logs the snapshot. The production
// implementation (snapshotEmitter) POSTs a HealthReport to the agenthealth
// intake.
type componentHealthEmitter interface {
	Snapshot(ctx context.Context, active, resolved []componenthealth.ComponentIssue) error
}

// componentIssueKey identifies a component-level issue instance.
// Detection happens per pod, but this is the granularity at which issues are reported.
type componentIssueKey struct {
	namespace string
	component string
	issueType string
}

// componentIssueState is the live state of one component-level issue: the pods
// currently exhibiting it, keyed by pod name, plus its lifecycle timestamps. The
// issue is active while pods is non-empty.
type componentIssueState struct {
	pods      map[string]struct{}
	firstSeen time.Time // when the issue first appeared on the component
	lastSeen  time.Time // last time the issue was observed active
}

// resolvedIssue is a tombstone for a component-level issue that has cleared: it
// carries the lifecycle timestamps needed to report an explicit RESOLVED signal.
// It is held until a snapshot carrying it is acknowledged.
type resolvedIssue struct {
	firstSeen  time.Time
	lastSeen   time.Time
	resolvedAt time.Time
}

// ComponentHealthReconciler watches the managed cluster-level component pods
// (Datadog Cluster Agent and Cluster Check Runners) and reports Kubernetes
// health issues (OOMKills, crash loops, scheduling failures, image-pull
// failures) derived from their pod status.
//
// Detection is per pod, but issues are reported per component: the same issue
// type across several pods of a component is one issue instance, so a rolling
// restart where the problem hops between pods stays a single active issue rather
// than a churn of emit/resolve events.
//
// It is gated behind the ComponentHealth feature flag and depends on the pod
// status being retained in the cache (see pkg/config.CacheOptions).
type ComponentHealthReconciler struct {
	client           client.Client
	log              logr.Logger
	restartThreshold int32
	emitter          componentHealthEmitter
	snapshotInterval time.Duration
	clock            func() time.Time // overridable for tests; defaults to time.Now

	// mu guards reported and resolved. reported holds every currently-active
	// component-level issue and the set of pods exhibiting it. resolved holds the
	// issues that have cleared since the last successfully-sent snapshot, so the
	// next snapshot can carry an explicit RESOLVED signal for them rather than
	// relying on backend TTL expiry. An entry stays in resolved until a snapshot
	// carrying it is acknowledged (so a failed send retries it) or until the same
	// issue becomes active again.
	mu       sync.Mutex
	reported map[componentIssueKey]*componentIssueState
	resolved map[componentIssueKey]resolvedIssue
}

// NewComponentHealthReconciler builds a ComponentHealthReconciler with the
// default logging/metric emitter.
func NewComponentHealthReconciler(c client.Client, log logr.Logger) *ComponentHealthReconciler {
	return &ComponentHealthReconciler{
		client:           c,
		log:              log,
		restartThreshold: defaultRestartThreshold,
		emitter:          &logEmitter{log: log},
		snapshotInterval: defaultSnapshotInterval,
		clock:            time.Now,
		reported:         map[componentIssueKey]*componentIssueState{},
		resolved:         map[componentIssueKey]resolvedIssue{},
	}
}

// Reconcile evaluates a single managed component pod and folds its detected
// issues into the component-level state, emitting only the resulting transitions.
func (r *ComponentHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pod := &corev1.Pod{}
	if err := r.client.Get(ctx, req.NamespacedName, pod); err != nil {
		if apierrors.IsNotFound(err) {
			// Pod is gone: drop it from every component issue it contributed to.
			r.forgetPod(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// A pod being deleted is treated like a gone pod.
	if !pod.DeletionTimestamp.IsZero() {
		r.forgetPod(req.NamespacedName)
		return ctrl.Result{}, nil
	}

	component := componenthealth.ManagedComponent(pod)
	issues := componenthealth.DetectPodIssues(pod, r.restartThreshold)
	r.reconcilePod(req.NamespacedName, component, issues)
	return ctrl.Result{}, nil
}

// reconcilePod folds one pod's currently-detected issues into the component-level
// state: it registers the pod on the issues it now exhibits and removes it from
// the component issues it no longer does, emitting Emit/Resolve on the component's
// presence transitions.
func (r *ComponentHealthReconciler) reconcilePod(pod types.NamespacedName, component string, issues []componenthealth.DetectedIssue) {
	// A pod can carry the same issue type on more than one container (e.g. two
	// crash-looping containers)
	current := make(map[string]struct{}, len(issues))
	for _, issue := range issues {
		current[issue.IssueType] = struct{}{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove the pod from any component issue of this component that it no longer
	// exhibits.
	for key, state := range r.reported {
		if key.namespace != pod.Namespace || key.component != component {
			continue
		}
		if _, still := current[key.issueType]; still {
			continue
		}
		r.dropPod(key, state, pod.Name)
	}

	// Register the pod on every issue it currently exhibits.
	for issueType := range current {
		key := componentIssueKey{namespace: pod.Namespace, component: component, issueType: issueType}
		r.addPod(key, pod.Name)
	}
}

// addPod records that a pod exhibits a component issue. On the transition from
// absent to present on a component (the first affected pod), it counts the issue
// and logs the edge; the active set itself is reported to the emitter on the
// snapshot cadence (see Start), not here. Caller holds mu.
func (r *ComponentHealthReconciler) addPod(key componentIssueKey, pod string) {
	now := r.clock()
	state := r.reported[key]
	if state == nil {
		state = &componentIssueState{pods: map[string]struct{}{}}
		r.reported[key] = state
	}
	firstPod := len(state.pods) == 0
	state.pods[pod] = struct{}{}
	// lastSeen advances on every observation of the issue; firstSeen is stamped
	// once, when it first appears.
	state.lastSeen = now

	if firstPod {
		state.firstSeen = now
		// The issue is active again: drop any pending resolve so we don't send a
		// stale RESOLVED signal for something that is currently broken.
		delete(r.resolved, key)
		metrics.ComponentHealthIssuesDetected.WithLabelValues(key.component, key.issueType).Inc()
		issue := r.issueFor(key, state)
		r.log.Info("component health issue detected",
			"component", issue.Component,
			"issue_type", issue.IssueType,
			"severity", issue.Severity,
			"namespace", issue.Namespace,
			"affected_pods", issue.AffectedPods,
		)
	}
	r.setActiveGauge(key)
}

// dropPod removes a pod from a component issue, logging the edge and clearing the
// state when the last affected pod is gone. Caller holds mu.
func (r *ComponentHealthReconciler) dropPod(key componentIssueKey, state *componentIssueState, pod string) {
	if _, ok := state.pods[pod]; !ok {
		return
	}
	delete(state.pods, pod)
	if len(state.pods) == 0 {
		r.log.Info("component health issue resolved",
			"component", key.component,
			"issue_type", key.issueType,
			"namespace", key.namespace,
		)
		delete(r.reported, key)
		// Remember the clear so the next snapshot carries an explicit RESOLVED
		// signal for it, carrying the lifecycle timestamps; it is dropped once that
		// snapshot is acknowledged.
		r.resolved[key] = resolvedIssue{
			firstSeen:  state.firstSeen,
			lastSeen:   state.lastSeen,
			resolvedAt: r.clock(),
		}
		r.setActiveGauge(key)
		return
	}
	r.setActiveGauge(key)
}

// forgetPod removes a pod from every component issue it contributed to (used when
// the pod disappears), clearing any issue whose last pod it was.
func (r *ComponentHealthReconciler) forgetPod(pod types.NamespacedName) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, state := range r.reported {
		if key.namespace != pod.Namespace {
			continue
		}
		r.dropPod(key, state, pod.Name)
	}
}

// collectSnapshot returns the current active component-level issues and the
// issues that have cleared since the last acknowledged snapshot. It briefly holds
// r.mu to copy the state and returns before any emission happens, so the emitter
// is never called while the lock is held.
func (r *ComponentHealthReconciler) collectSnapshot() (active, resolved []componenthealth.ComponentIssue) {
	r.mu.Lock()
	defer r.mu.Unlock()

	active = make([]componenthealth.ComponentIssue, 0, len(r.reported))
	for key, state := range r.reported {
		active = append(active, r.issueFor(key, state))
	}
	sortIssues(active)

	resolved = make([]componenthealth.ComponentIssue, 0, len(r.resolved))
	for key, ri := range r.resolved {
		resolved = append(resolved, componenthealth.ComponentIssue{
			Component:  key.component,
			IssueType:  key.issueType,
			Severity:   componenthealth.SeverityForIssueType(key.issueType),
			Namespace:  key.namespace,
			FirstSeen:  ri.firstSeen,
			LastSeen:   ri.lastSeen,
			ResolvedAt: ri.resolvedAt,
		})
	}
	sortIssues(resolved)

	return active, resolved
}

// ackResolved drops the given resolved issues from the pending set once a snapshot
// carrying them has been delivered. Issues that became active again in the
// meantime were already removed by addPod, so deleting them here is a no-op.
func (r *ComponentHealthReconciler) ackResolved(resolved []componenthealth.ComponentIssue) {
	if len(resolved) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, issue := range resolved {
		delete(r.resolved, componentIssueKey{namespace: issue.Namespace, component: issue.Component, issueType: issue.IssueType})
	}
}

// emitSnapshot hands the current active and recently-resolved issues to the
// emitter. The snapshot is computed (and the lock released) before the emitter
// call, so no lock is held during emission. Resolved issues are acknowledged
// (dropped from the pending set) only on a successful send, so a failed send
// retries them on the next tick.
func (r *ComponentHealthReconciler) emitSnapshot(ctx context.Context) {
	active, resolved := r.collectSnapshot()
	if err := r.emitter.Snapshot(ctx, active, resolved); err != nil {
		r.log.V(1).Info("component health snapshot not delivered, will retry resolved issues", "error", err)
		return
	}
	r.ackResolved(resolved)
}

// sortIssues orders issues deterministically by component, namespace, issue type.
func sortIssues(issues []componenthealth.ComponentIssue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Component != issues[j].Component {
			return issues[i].Component < issues[j].Component
		}
		if issues[i].Namespace != issues[j].Namespace {
			return issues[i].Namespace < issues[j].Namespace
		}
		return issues[i].IssueType < issues[j].IssueType
	})
}

// Start runs the snapshot loop: on a fixed cadence it hands the current
// active-issue set to the emitter. It implements manager.Runnable and is
// registered with the manager in SetupWithManager.
//
// TODO(CONTP-2137): close the restart resolve-gap. State is in-memory, so if an
// issue was active before an operator restart and clears while the operator is
// down (or before it re-observes the pods), no present->absent transition occurs
// and no RESOLVED signal is sent — the backend only clears it via TTL. Once
// detection has stabilized on startup, emit explicit RESOLVED signals for the
// catalog issue types not currently active on each managed component, so the
// backend reconciles any issue left stuck from before the restart. (Pending the
// backend decision on whether a full operator snapshot can instead be treated as
// authoritative, which would close this gap without a startup sweep.)
func (r *ComponentHealthReconciler) Start(ctx context.Context) error {
	ticker := time.NewTicker(r.snapshotInterval)
	defer ticker.Stop()

	r.log.Info("Starting component health snapshot loop", "interval", r.snapshotInterval)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.emitSnapshot(ctx)
		}
	}
}

// NeedLeaderElection implements manager.LeaderElectionRunnable so the snapshot
// loop only runs on the leader, preventing multiple operator replicas from
// double-reporting the same cluster's issues.
func (r *ComponentHealthReconciler) NeedLeaderElection() bool { return true }

// issueFor builds the component-level issue payload from the current state.
func (r *ComponentHealthReconciler) issueFor(key componentIssueKey, state *componentIssueState) componenthealth.ComponentIssue {
	pods := make([]string, 0, len(state.pods))
	for pod := range state.pods {
		pods = append(pods, pod)
	}
	sort.Strings(pods)
	return componenthealth.ComponentIssue{
		Component:    key.component,
		IssueType:    key.issueType,
		Severity:     componenthealth.SeverityForIssueType(key.issueType),
		Namespace:    key.namespace,
		AffectedPods: pods,
		FirstSeen:    state.firstSeen,
		LastSeen:     state.lastSeen,
	}
}

// setActiveGauge recomputes the active-pod gauge for key.component/key.issueType
// by summing affected pods across every namespace, since the gauge itself is not
// labeled by namespace (see ComponentHealthIssuesActive). Caller holds mu.
func (r *ComponentHealthReconciler) setActiveGauge(key componentIssueKey) {
	total := 0
	for k, state := range r.reported {
		if k.component == key.component && k.issueType == key.issueType {
			total += len(state.pods)
		}
	}
	metrics.ComponentHealthIssuesActive.WithLabelValues(key.component, key.issueType).Set(float64(total))
}

// SetupWithManager wires the controller to watch only the managed cluster-level
// component pods (cluster-agent, cluster-checks-runner). Node Agent pods are
// intentionally excluded.
func (r *ComponentHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	pred, err := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      common.AgentDeploymentComponentLabelKey,
				Operator: metav1.LabelSelectorOpIn,
				Values: []string{
					constants.DefaultClusterAgentResourceSuffix,
					constants.DefaultClusterChecksRunnerResourceSuffix,
				},
			},
		},
	})
	if err != nil {
		return err
	}

	if err := ctrl.NewControllerManagedBy(mgr).
		Named(componentHealthControllerName).
		For(&corev1.Pod{}, builder.WithPredicates(pred)).
		Complete(r); err != nil {
		return err
	}

	// Register the snapshot loop as a leader-elected runnable so it ticks
	// independently of pod reconciles and only on the leader.
	return mgr.Add(r)
}

// logEmitter is the default componentHealthEmitter: it logs the current
// active-issue snapshot. It is used when the agenthealth intake emitter is not
// enabled.
type logEmitter struct {
	log logr.Logger
}

func (e *logEmitter) Snapshot(_ context.Context, active, resolved []componenthealth.ComponentIssue) error {
	if len(active) == 0 && len(resolved) == 0 {
		e.log.V(1).Info("component health snapshot: no active or resolved issues")
		return nil
	}
	e.log.V(1).Info("component health snapshot",
		"active", len(active), "resolved", len(resolved),
		"active_issues", active, "resolved_issues", resolved)
	return nil
}
