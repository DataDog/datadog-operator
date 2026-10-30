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

	// defaultResolveGracePeriod is how long a component-level issue must stay clear
	// before its RESOLVED signal is emitted. It debounces a component that flaps
	// between detected and clear (e.g. an OOM/crash-loop cycle) so a brief recovery
	// window does not emit a false resolve. The backend TTL remains the backstop.
	defaultResolveGracePeriod = 5 * time.Minute
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
	// clearedAt is when the issue last went clear. Its RESOLVED signal is withheld
	// until it has stayed clear for resolveGracePeriod past this time (see
	// collectSnapshot); a re-activation drops the tombstone (see addPod). A zero
	// clearedAt bypasses the grace period (startup sweep resolves).
	clearedAt time.Time
}

// componentScope identifies a managed component within a namespace. It is the
// unit over which the startup resolve sweep enumerates the issue catalog.
type componentScope struct {
	namespace string
	component string
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
	// resolveGracePeriod is how long an issue must stay clear before its RESOLVED
	// signal is emitted. Overridable for tests; 0 disables the debounce.
	resolveGracePeriod time.Duration
	clock              func() time.Time // overridable for tests; defaults to time.Now

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

	// seeded is closed after startup initializes issue state from existing pods.
	// Reconcile waits for it so startup cannot overwrite a newer pod observation.
	seeded chan struct{}
}

// NewComponentHealthReconciler builds a ComponentHealthReconciler with the
// default logging/metric emitter.
func NewComponentHealthReconciler(c client.Client, log logr.Logger) *ComponentHealthReconciler {
	return &ComponentHealthReconciler{
		client:             c,
		log:                log,
		restartThreshold:   defaultRestartThreshold,
		emitter:            &logEmitter{log: log},
		snapshotInterval:   defaultSnapshotInterval,
		resolveGracePeriod: defaultResolveGracePeriod,
		clock:              time.Now,
		reported:           map[componentIssueKey]*componentIssueState{},
		resolved:           map[componentIssueKey]resolvedIssue{},
		seeded:             make(chan struct{}),
	}
}

// Reconcile evaluates a single managed component pod and folds its detected
// issues into the component-level state, emitting only the resulting transitions.
func (r *ComponentHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Retry after startup has initialized issue state.
	select {
	case <-r.seeded:
	default:
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

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
		// A flap (re-activating while its resolve is still pending within the grace
		// window) is not a new occurrence: keep the original first_seen and skip the
		// detected log/counter. Otherwise drop the pending resolve and count it.
		ri, pending := r.resolved[key]
		flap := pending && !ri.clearedAt.IsZero() && now.Sub(ri.clearedAt) < r.resolveGracePeriod
		if flap && !ri.firstSeen.IsZero() {
			state.firstSeen = ri.firstSeen
		}
		delete(r.resolved, key)

		issue := r.issueFor(key, state)
		if flap {
			r.log.V(1).Info("component health issue re-detected within resolve window",
				"component", issue.Component,
				"issue_type", issue.IssueType,
				"namespace", issue.Namespace,
			)
		} else {
			metrics.ComponentHealthIssuesDetected.WithLabelValues(key.component, key.issueType).Inc()
			r.log.Info("component health issue detected",
				"component", issue.Component,
				"issue_type", issue.IssueType,
				"severity", issue.Severity,
				"namespace", issue.Namespace,
				"affected_pods", issue.AffectedPods,
			)
		}
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
		// Debug only: the raw clear flaps for a crash-looping component. The
		// default-level resolve is logged once, debounced, in emitSnapshot.
		r.log.V(1).Info("component health issue clear observed",
			"component", key.component,
			"issue_type", key.issueType,
			"namespace", key.namespace,
		)
		delete(r.reported, key)
		// Remember the clear so a later snapshot carries an explicit RESOLVED signal
		// for it, carrying the lifecycle timestamps. The signal is withheld until the
		// issue has stayed clear for resolveGracePeriod (see collectSnapshot), and the
		// tombstone is dropped once the snapshot carrying it is acknowledged.
		now := r.clock()
		r.resolved[key] = resolvedIssue{
			firstSeen:  state.firstSeen,
			lastSeen:   state.lastSeen,
			resolvedAt: now,
			clearedAt:  now,
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
	now := r.clock()
	r.mu.Lock()
	defer r.mu.Unlock()

	active = make([]componenthealth.ComponentIssue, 0, len(r.reported))
	for key, state := range r.reported {
		active = append(active, r.issueFor(key, state))
	}
	sortIssues(active)

	resolved = make([]componenthealth.ComponentIssue, 0, len(r.resolved))
	for key, ri := range r.resolved {
		// Withhold the RESOLVED signal until the issue has stayed clear for the grace
		// period, so a flapping component does not emit a false resolve. A zero
		// clearedAt (startup sweep) is emitted immediately; otherwise the tombstone
		// stays pending until it ages past the window or a re-activation drops it.
		if !ri.clearedAt.IsZero() && now.Sub(ri.clearedAt) < r.resolveGracePeriod {
			continue
		}
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
// carrying them has been delivered. It only drops a tombstone whose resolvedAt
// still matches the one that was sent: while a slow send is in flight, an issue
// can reappear and clear again, replacing the tombstone with a newer one that was
// never sent. Deleting by key alone would ack that newer tombstone and drop its
// RESOLVED signal, so it is retained (and retried next tick) when its resolvedAt
// differs. Issues that became active again and stayed active were already removed
// by addPod, so they are simply absent here.
func (r *ComponentHealthReconciler) ackResolved(resolved []componenthealth.ComponentIssue) {
	if len(resolved) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, issue := range resolved {
		key := componentIssueKey{namespace: issue.Namespace, component: issue.Component, issueType: issue.IssueType}
		if ri, ok := r.resolved[key]; ok && ri.resolvedAt.Equal(issue.ResolvedAt) {
			delete(r.resolved, key)
		}
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
	// Debounced resolve log: fires once per delivered, grace-promoted resolve, so it
	// does not flap. Startup-sweep catalog clears (no first_seen) stay at debug.
	for _, issue := range resolved {
		if issue.FirstSeen.IsZero() {
			continue
		}
		r.log.Info("component health issue resolved",
			"component", issue.Component,
			"issue_type", issue.IssueType,
			"namespace", issue.Namespace,
		)
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
// Before the loop it runs seedAndSweep once, then emits an initial snapshot so
// recovered issues clear promptly instead of waiting a full interval.
func (r *ComponentHealthReconciler) Start(ctx context.Context) error {
	r.log.Info("Starting component health snapshot loop", "interval", r.snapshotInterval)

	if err := r.seedAndSweep(ctx); err != nil {
		// A failed sweep is not fatal: steady-state reporting still works, and any
		// pre-restart issue that cleared during downtime falls back to backend TTL.
		r.log.Error(err, "component health startup resolve sweep failed; pre-restart issues that cleared during downtime will clear via backend TTL instead")
	}
	// Release deferred reconciles. Closed even on sweep error so reconciliation
	// never stays blocked.
	close(r.seeded)
	r.emitSnapshot(ctx)

	ticker := time.NewTicker(r.snapshotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.emitSnapshot(ctx)
		}
	}
}

// seedAndSweep loads active issues from existing component pods, then queues
// RESOLVED signals for the remaining issue types. This clears issues that
// recovered while the operator was stopped.
//
// Components with no remaining pods are skipped; their previous issues expire
// through the backend TTL.
func (r *ComponentHealthReconciler) seedAndSweep(ctx context.Context) error {
	ls := managedComponentLabelSelector()
	sel, err := metav1.LabelSelectorAsSelector(&ls)
	if err != nil {
		return err
	}

	pods := &corev1.PodList{}
	if err := r.client.List(ctx, pods, &client.ListOptions{LabelSelector: sel}); err != nil {
		return err
	}

	observed := map[componentScope]struct{}{}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !pod.DeletionTimestamp.IsZero() {
			continue
		}
		component := componenthealth.ManagedComponent(pod)
		if component == "" {
			continue
		}
		observed[componentScope{namespace: pod.Namespace, component: component}] = struct{}{}
		issues := componenthealth.DetectPodIssues(pod, r.restartThreshold)
		r.reconcilePod(types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, component, issues)
	}

	r.sweepResolved(observed)
	return nil
}

// sweepResolved queues a RESOLVED tombstone for every catalog issue type that is
// not currently active on an observed component, skipping issues that are active
// or already pending resolution. Caller must not hold mu.
func (r *ComponentHealthReconciler) sweepResolved(observed map[componentScope]struct{}) {
	now := r.clock()
	r.mu.Lock()
	defer r.mu.Unlock()
	for scope := range observed {
		for _, issueType := range componenthealth.AllIssueTypes {
			key := componentIssueKey{namespace: scope.namespace, component: scope.component, issueType: issueType}
			if _, active := r.reported[key]; active {
				continue
			}
			if _, pending := r.resolved[key]; pending {
				continue
			}
			// Only resolvedAt is known here; the backend matches on issue identity.
			r.resolved[key] = resolvedIssue{resolvedAt: now}
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

// managedComponentLabelSelector selects the managed cluster-level component pods
// (cluster-agent, cluster-checks-runner). It is the single source of truth for
// both the watch predicate and the startup resolve sweep's pod list, so the two
// cannot drift.
func managedComponentLabelSelector() metav1.LabelSelector {
	return metav1.LabelSelector{
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
	}
}

// SetupWithManager wires the controller to watch only the managed cluster-level
// component pods (cluster-agent, cluster-checks-runner). Node Agent pods are
// intentionally excluded.
func (r *ComponentHealthReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ls := managedComponentLabelSelector()
	pred, err := predicate.LabelSelectorPredicate(ls)
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
