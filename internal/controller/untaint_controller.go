// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/untaint"
)

// Environment variables consumed by the untaint controller. Reading them here
// (rather than in cmd/main.go) keeps the controller's configuration surface
// self-contained so callers do not need to know about them.
const (
	EnvEventsEnabled     = "DD_UNTAINT_CONTROLLER_EVENTS_ENABLED"
	EnvReadinessTimeout  = "DD_UNTAINT_CONTROLLER_TIMEOUT"
	EnvSchedulingTimeout = "DD_UNTAINT_CONTROLLER_SCHEDULING_TIMEOUT"
	EnvTimeoutPolicy     = "DD_UNTAINT_CONTROLLER_TIMEOUT_POLICY"
)

const (
	untaintControllerName = "Untaint"

	// untaintPodNodeIndex is a controller-scoped field-indexer key for caching
	// pods by their spec.nodeName. Using a namespaced key (rather than the
	// bare "spec.nodeName") avoids collisions with any other controller that
	// may register the same field on the shared manager indexer.
	untaintPodNodeIndex = "untaint.spec.nodeName"

	// conflictRequeueDelay is how long we wait before retrying after a benign
	// optimistic-concurrency conflict on the taint patch. Matches the convention
	// used elsewhere in this codebase for IsConflict on status updates.
	conflictRequeueDelay = time.Second

	// DefaultReadinessTimeout is applied when an agent pod exists on a tainted
	// node but never reaches Ready. Clock: pod.Status.StartTime.
	DefaultReadinessTimeout = 10 * time.Minute
	// DefaultSchedulingTimeout is applied when no agent pod is ever scheduled
	// on a tainted node. Clock: node.CreationTimestamp.
	DefaultSchedulingTimeout = 5 * time.Minute
)

// TimeoutPolicy controls what the controller does when a timeout fires.
type TimeoutPolicy string

const (
	// PolicyRemove untaints the node despite the agent never becoming ready.
	PolicyRemove TimeoutPolicy = metrics.UntaintTimeoutPolicyRemove
	// PolicyKeep leaves the taint in place; the controller only emits
	// observability signals (metric, event, log).
	PolicyKeep TimeoutPolicy = metrics.UntaintTimeoutPolicyKeep
)

// ParseTimeoutPolicy returns PolicyRemove for the empty string or "remove",
// PolicyKeep for "keep", and an error for any other value.
func ParseTimeoutPolicy(s string) (TimeoutPolicy, error) {
	switch s {
	case "", string(PolicyRemove):
		return PolicyRemove, nil
	case string(PolicyKeep):
		return PolicyKeep, nil
	default:
		return "", fmt.Errorf("invalid untaint timeout policy %q (want %q or %q)", s, PolicyRemove, PolicyKeep)
	}
}

// UntaintReconciler watches agent pods and nodes and removes the taint
// agent.datadoghq.com/not-ready=presence:NoSchedule once the agent pod is
// Ready, or after a configurable timeout depending on the policy.
type UntaintReconciler struct {
	client   client.Client
	log      logr.Logger
	recorder record.EventRecorder
	clock    clock.PassiveClock

	eventsEnabled     bool
	readinessTimeout  time.Duration
	schedulingTimeout time.Duration
	timeoutPolicy     TimeoutPolicy
}

// NewUntaintReconciler builds an UntaintReconciler. All tuning knobs are
// sourced from environment variables (DD_UNTAINT_CONTROLLER_*) with the
// constants on this package as fallback defaults. Any invalid env value
// (unparseable duration, unknown policy) returns an error and aborts
// startup — we fail loud rather than silently substituting defaults so
// operator misconfiguration is caught at boot, not discovered later when
// timeouts fire at the wrong time.
//
// The effective configuration is logged once at INFO so the operator can
// confirm what was actually applied.
func NewUntaintReconciler(c client.Client, log logr.Logger, rec record.EventRecorder) (*UntaintReconciler, error) {
	policy, err := ParseTimeoutPolicy(os.Getenv(EnvTimeoutPolicy))
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", EnvTimeoutPolicy, err)
	}
	readiness, err := durationFromEnv(EnvReadinessTimeout, DefaultReadinessTimeout)
	if err != nil {
		return nil, err
	}
	scheduling, err := durationFromEnv(EnvSchedulingTimeout, DefaultSchedulingTimeout)
	if err != nil {
		return nil, err
	}

	r := &UntaintReconciler{
		client:            c,
		log:               log,
		recorder:          rec,
		clock:             clock.RealClock{},
		eventsEnabled:     os.Getenv(EnvEventsEnabled) == "true",
		readinessTimeout:  readiness,
		schedulingTimeout: scheduling,
		timeoutPolicy:     policy,
	}

	log.Info("untaint controller configured",
		"eventsEnabled", r.eventsEnabled,
		"readinessTimeout", r.readinessTimeout,
		"schedulingTimeout", r.schedulingTimeout,
		"timeoutPolicy", r.timeoutPolicy,
	)
	return r, nil
}

// durationFromEnv reads envVar as a Go duration. Returns def if unset or
// empty. Returns an error (with envVar context) on any parse failure or
// non-positive value — we refuse to start with bad config rather than
// silently fall back to the default.
func durationFromEnv(envVar string, def time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(envVar)
	if !ok || raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: %w", envVar, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid %s=%q: must be positive", envVar, raw)
	}
	return d, nil
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile decides what to do with a tainted node:
//   - if any agent pod on the node is Ready, untaint
//   - if an agent pod exists but is not Ready and the readiness timeout has
//     elapsed, apply the timeout policy (remove or keep)
//   - if no agent pod is scheduled and the scheduling timeout has elapsed,
//     apply the timeout policy
//   - otherwise, requeue after the remaining timeout window so we re-evaluate.
func (r *UntaintReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.WithValues("node", req.Name)

	node := &corev1.Node{}
	if err := r.client.Get(ctx, req.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get node %s: %w", req.Name, err)
	}

	if !hasTaint(node) {
		return ctrl.Result{}, nil
	}

	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
	})
	if err := r.client.List(ctx, podList,
		client.MatchingLabelsSelector{Selector: labelSelector},
		client.MatchingFields{untaintPodNodeIndex: req.Name},
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list pods on node %s: %w", req.Name, err)
	}

	// Happy path: any ready agent pod → untaint immediately and record latency.
	var readyAt time.Time
	var anyReady bool
	for i := range podList.Items {
		if ts, ok := podReadyTransition(&podList.Items[i]); ok {
			readyAt, anyReady = ts, true
			break
		}
	}
	if anyReady {
		result, err := r.removeTaint(ctx, node)
		if err != nil {
			return result, err
		}
		if result.RequeueAfter > 0 {
			return result, nil
		}
		log.Info(fmt.Sprintf("Removed agent-not-ready taint from node %s", node.Name))
		metrics.TaintRemovalsTotal.Inc()
		if !readyAt.IsZero() {
			metrics.TaintRemovalLatency.Observe(r.clock.Since(readyAt).Seconds())
		}
		if r.eventsEnabled {
			r.recorder.Eventf(node, corev1.EventTypeNormal, "TaintRemoved",
				"Removed taint %s from node %s after agent became ready", untaint.AgentNotReadyTaintKey, node.Name)
		}
		return ctrl.Result{}, nil
	}

	// Timeout-evaluation path.
	//   1. Agent pod present - readiness timeout. Clock: max(pod.Status.StartTime),
	//      so a fresh restart resets the window. If StartTime is unset (very
	//      early lifecycle), requeue without falling through to scheduling timeout check
	//      since pod is scheduled.
	//   2. Agent pod not present - scheduling timeout. Clock: node.CreationTimestamp.
	now := r.clock.Now()

	if len(podList.Items) > 0 {
		latestStart, ok := latestPodStartTime(podList.Items)
		if !ok {
			// Pods scheduled but no StartTime yet. Re-check shortly.
			return ctrl.Result{RequeueAfter: r.readinessTimeout}, nil
		}
		elapsed := now.Sub(latestStart)
		if elapsed >= r.readinessTimeout {
			return r.applyTimeoutPolicy(ctx, node, metrics.UntaintTimeoutReasonReadiness, elapsed)
		}
		return ctrl.Result{RequeueAfter: r.readinessTimeout - elapsed}, nil
	}

	created := node.CreationTimestamp.Time
	if created.IsZero() {
		// Defensive: without a CreationTimestamp we cannot compute scheduling
		// elapsed. Requeue after the scheduling timeout so we re-evaluate once
		// the cache catches up.
		log.V(1).Info("Node has no CreationTimestamp; requeueing")
		return ctrl.Result{RequeueAfter: r.schedulingTimeout}, nil
	}
	elapsed := now.Sub(created)
	if elapsed >= r.schedulingTimeout {
		return r.applyTimeoutPolicy(ctx, node, metrics.UntaintTimeoutReasonScheduling, elapsed)
	}
	return ctrl.Result{RequeueAfter: r.schedulingTimeout - elapsed}, nil
}

// applyTimeoutPolicy is invoked when a readiness or scheduling timeout has
// elapsed. It records observability signals and, if policy=remove, removes the
// taint.
func (r *UntaintReconciler) applyTimeoutPolicy(ctx context.Context, node *corev1.Node, reason string, elapsed time.Duration) (ctrl.Result, error) {
	policy := r.timeoutPolicy
	log := r.log.WithValues("node", node.Name, "reason", reason, "policy", policy, "elapsed", elapsed)

	metrics.TaintTimeoutsTotal.WithLabelValues(reason, string(policy)).Inc()
	if policy == PolicyKeep {
		// logr has no Warn level; the codebase convention is log.Error(nil, ...)
		// to surface a warning-level event without an associated Go error.
		log.Error(nil, fmt.Sprintf("Untaint controller timeout fired on node %s; keeping taint per policy=keep — operator action may be required", node.Name))
	} else {
		log.Info(fmt.Sprintf("Untaint controller timeout fired on node %s", node.Name))
	}
	if r.eventsEnabled {
		evType := corev1.EventTypeNormal
		evReason := "UntaintTimeout"
		if policy == PolicyKeep {
			evType = corev1.EventTypeWarning
		}
		r.recorder.Eventf(node, evType, evReason,
			"Untaint controller %s timeout reached on node %s after %s (policy=%s)", reason, node.Name, elapsed.Round(time.Second), policy)
	}

	if policy == PolicyKeep {
		// No requeue: nothing will change until a pod event or node-taint event
		// fires; the watches handle that.
		return ctrl.Result{}, nil
	}

	result, err := r.removeTaint(ctx, node)
	if err != nil {
		return result, err
	}
	if result.RequeueAfter > 0 {
		return result, nil
	}
	log.Info(fmt.Sprintf("Removed agent-not-ready taint from node %s by timeout policy", node.Name))
	metrics.TaintRemovalsTotal.Inc()
	return ctrl.Result{}, nil
}

// podReadyTransition returns the Ready condition's LastTransitionTime and
// true if the pod is currently Ready, otherwise (zero, false).
func podReadyTransition(pod *corev1.Pod) (time.Time, bool) {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			// This controller only gates the initial node join. Once an agent
			// reached Ready on the node, a later terminating/restarting pod is
			// still valid evidence that the bootstrap condition was satisfied.
			return c.LastTransitionTime.Time, true
		}
	}
	return time.Time{}, false
}

// latestPodStartTime returns the most recent non-nil Status.StartTime across
// pods plus an ok flag. When a pod has just restarted, its StartTime resets,
// so the readiness clock effectively restarts as well.
func latestPodStartTime(pods []corev1.Pod) (time.Time, bool) {
	var latest time.Time
	found := false
	for i := range pods {
		st := pods[i].Status.StartTime
		if st == nil {
			continue
		}
		if !found || st.Time.After(latest) {
			latest = st.Time
			found = true
		}
	}
	return latest, found
}

// hasTaint returns true if the node has the agent-not-ready taint.
func hasTaint(node *corev1.Node) bool {
	return slices.ContainsFunc(node.Spec.Taints, untaint.IsAgentNotReadyTaint)
}

type jsonPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// removeTaint patches the node to remove the agent-not-ready taint using a
// JSON test-and-set patch. Returns RequeueAfter on a benign optimistic-
// concurrency conflict so the caller does not trigger exponential backoff.
func (r *UntaintReconciler) removeTaint(ctx context.Context, node *corev1.Node) (ctrl.Result, error) {
	// Defensive: ensure we always marshal [] not null, even for a nil slice.
	currentTaints := node.Spec.Taints
	if currentTaints == nil {
		currentTaints = []corev1.Taint{}
	}
	newTaints := make([]corev1.Taint, 0, len(currentTaints))
	for _, t := range currentTaints {
		if untaint.IsAgentNotReadyTaint(t) {
			continue
		}
		newTaints = append(newTaints, t)
	}
	if len(newTaints) == len(currentTaints) {
		return ctrl.Result{}, nil // taint not present (defensive; caller already checked)
	}

	patch := []jsonPatchOp{
		// test precondition catches concurrent modifications to the taints slice
		{Op: "test", Path: "/spec/taints", Value: currentTaints},
		{Op: "replace", Path: "/spec/taints", Value: newTaints},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		metrics.TaintRemovalErrorsTotal.Inc()
		return ctrl.Result{}, fmt.Errorf("failed to marshal patch: %w", err)
	}

	if err := r.client.Patch(ctx, node, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		// Both "resource conflict" (etcd CAS) and "test op failed" (different
		// k8s versions surface as Conflict or Invalid/UnprocessableEntity) are
		// benign races: another writer modified the node taints concurrently.
		// Requeue with a short delay instead of returning the error, which
		// would otherwise trigger exponential backoff.
		if apierrors.IsConflict(err) || apierrors.IsInvalid(err) {
			r.log.V(1).Info("Taint patch race; requeueing",
				"node", node.Name, "err", err.Error())
			return ctrl.Result{RequeueAfter: conflictRequeueDelay}, nil
		}
		metrics.TaintRemovalErrorsTotal.Inc()
		return ctrl.Result{}, fmt.Errorf("failed to remove taint from node %s: %w", node.Name, err)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager wires the Pod and Node watches and registers the cache
// field index used to list pods by node name.
func (r *UntaintReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, untaintPodNodeIndex, func(obj client.Object) []string {
		pod, ok := obj.(*corev1.Pod)
		if !ok || pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return fmt.Errorf("failed to index pods by %s: %w", untaintPodNodeIndex, err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(untaintControllerName).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.podToNodeRequest),
			builder.WithPredicates(agentPodPredicate()),
		).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(nodeToRequest),
			builder.WithPredicates(taintedNodePredicate()),
		).
		Complete(r)
}

// podToNodeRequest maps a Pod event to a reconcile.Request for the pod's node.
func (r *UntaintReconciler) podToNodeRequest(_ context.Context, obj client.Object) []reconcile.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok || pod.Spec.NodeName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: pod.Spec.NodeName}}}
}

// nodeToRequest maps a Node event to a reconcile.Request for that node.
func nodeToRequest(_ context.Context, obj client.Object) []reconcile.Request {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: node.Name}}}
}

// agentPodPredicate enqueues an agent pod's node for any Create/Update/Delete
// event on an agent pod. We deliberately do NOT filter to "ready transition"
// here because:
//   - a NotReady pod creation starts the readiness clock for that node, so the
//     reconciler needs to schedule a RequeueAfter against pod.Status.StartTime
//     (filtering out Create-as-NotReady would silently miss this clock until
//     some other event fires);
//   - a Delete must enqueue so a stuck-in-keep node can re-start its readiness
//     clock for the replacement pod;
//   - Reconcile itself is idempotent and quick (cache list + early-return on
//     !hasTaint), and bounds re-evaluation via RequeueAfter, so the extra
//     reconciles are cheap.
func agentPodPredicate() predicate.Predicate {
	isAgentPodEvent := func(o client.Object) bool {
		p, ok := o.(*corev1.Pod)
		return ok && isAgentPod(p)
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isAgentPodEvent(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isAgentPodEvent(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isAgentPodEvent(e.Object) },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// taintedNodePredicate enqueues a node when it carries the target taint on
// create, or when the target taint *appears* on update. The reconciler reruns
// itself via RequeueAfter while a timeout window is pending, so we do not need
// to fire on every unrelated node update.
func taintedNodePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			node, ok := e.Object.(*corev1.Node)
			return ok && hasTaint(node)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, okOld := e.ObjectOld.(*corev1.Node)
			newNode, okNew := e.ObjectNew.(*corev1.Node)
			if !okOld || !okNew {
				return false
			}
			return hasTaint(newNode) && !hasTaint(oldNode)
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// isAgentPod returns true if the pod has the agent component label.
func isAgentPod(pod *corev1.Pod) bool {
	return pod.Labels[common.AgentDeploymentComponentLabelKey] == constants.DefaultAgentResourceSuffix
}
