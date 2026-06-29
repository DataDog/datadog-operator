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
	"github.com/DataDog/datadog-operator/internal/controller/datadogcsidriver"
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
// agent.datadoghq.com/not-ready=presence:NoSchedule once readiness criteria
// are met, or after a configurable timeout depending on the policy.
// When waitForCSIDriver is true (--untaintControllerWaitForCSIDriver), the node
// agent and CSI node-server pods must both be Ready before untainting.
type UntaintReconciler struct {
	client   client.Client
	log      logr.Logger
	recorder record.EventRecorder
	clock    clock.PassiveClock

	waitForCSIDriver  bool
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
//
// waitForCSIDriver should match SetupOptions.UntaintControllerWaitForCSIDriver.
// It is only consulted when the untaint controller is running (--untaintControllerEnabled).
func NewUntaintReconciler(c client.Client, log logr.Logger, rec record.EventRecorder, waitForCSIDriver bool) (*UntaintReconciler, error) {
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
		waitForCSIDriver:  waitForCSIDriver,
		eventsEnabled:     os.Getenv(EnvEventsEnabled) == "true",
		readinessTimeout:  readiness,
		schedulingTimeout: scheduling,
		timeoutPolicy:     policy,
	}

	log.Info("untaint controller configured",
		"waitForCSIDriver", r.waitForCSIDriver,
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
//   - by default: if any agent pod on the node is Ready, untaint
//   - with --untaintControllerWaitForCSIDriver: agent and CSI node-server pods
//     must both be Ready before untaint
//   - if pods exist but readiness criteria are not met and the readiness timeout
//     has elapsed, apply the timeout policy (remove or keep)
//   - if no agent pod is scheduled and the scheduling timeout has elapsed,
//     apply the timeout policy
//   - with --untaintControllerWaitForCSIDriver: readiness timeout uses the
//     later of max(agent StartTime) and max(CSI StartTime) when both workloads
//     have a pod on the node with Status.StartTime set; if either workload is
//     missing on the node use the scheduling timeout (node creation); if both
//     are on the node but either side still lacks StartTime, requeue with
//     readinessTimeout (same coarse poll as agent-only) until StartTime appears
//   - otherwise, requeue after the remaining timeout window so we re-evaluate.
func (r *UntaintReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.WithValues("node", req.Name)

	node := &corev1.Node{}
	if err := r.client.Get(ctx, req.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			// Node is gone (e.g. autoscaler scale-down). Drop its per-node
			// metric series so node names don't accumulate in the exporter for
			// the operator's lifetime. No-op if the node never had series.
			metrics.DeleteNodeSeries(req.Name)
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

	if r.waitForCSIDriver {
		csiPodList := &corev1.PodList{}
		csiLabel := labels.SelectorFromSet(map[string]string{
			datadogcsidriver.AppLabelKey: datadogcsidriver.NodeServerDaemonSetAppValue,
		})
		if err := r.client.List(ctx, csiPodList,
			client.MatchingLabelsSelector{Selector: csiLabel},
			client.MatchingFields{untaintPodNodeIndex: req.Name},
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to list CSI driver pods on node %s: %w", req.Name, err)
		}

		agentReady := slices.ContainsFunc(podList.Items, func(p corev1.Pod) bool {
			_, ok := podReadyTransition(&p)
			return ok
		})
		csiReady := len(csiPodList.Items) > 0 && slices.ContainsFunc(csiPodList.Items, func(p corev1.Pod) bool {
			_, ok := podReadyTransition(&p)
			return ok
		})

		// Both Ready → untaint. Otherwise keep the taint and evaluate timeouts.
		if agentReady && csiReady {
			combined := append(append([]corev1.Pod{}, podList.Items...), csiPodList.Items...)
			readyAt, _ := maxReadyTransitionTime(combined)
			return r.completeUntaintFromReadiness(ctx, node, log, readyAt,
				fmt.Sprintf("Removed agent-not-ready taint from node %s after node agent and CSI driver pods became ready", node.Name),
				"Removed taint %s from node %s after node agent and CSI node-server pods became ready",
			)
		}
		return r.reconcileTaintedNodeTimeouts(ctx, node, log, podList, csiPodList)
	} else {
		// Agent-only mode: untaint when any node-agent pod is Ready; otherwise timeouts below.
		var readyAt time.Time
		var anyReady bool
		for i := range podList.Items {
			if ts, ok := podReadyTransition(&podList.Items[i]); ok {
				readyAt, anyReady = ts, true
				break
			}
		}
		if anyReady {
			return r.completeUntaintFromReadiness(ctx, node, log, readyAt,
				fmt.Sprintf("Removed agent-not-ready taint from node %s", node.Name),
				"Removed taint %s from node %s after agent became ready",
			)
		}
	}

	return r.reconcileTaintedNodeTimeouts(ctx, node, log, podList, nil)
}

// completeUntaintFromReadiness runs removeTaint after readiness gates passed, then
// records removal metrics, optional Node event, and latency from readyAt.
func (r *UntaintReconciler) completeUntaintFromReadiness(ctx context.Context, node *corev1.Node, log logr.Logger, readyAt time.Time, infoLog, eventFmt string) (ctrl.Result, error) {
	result, err := r.removeTaint(ctx, node)
	if err != nil {
		return result, err
	}
	if result.RequeueAfter > 0 {
		return result, nil
	}
	log.Info(infoLog)
	metrics.TaintRemovalsTotal.WithLabelValues(node.Name, metrics.UntaintRemovalReasonAgentReady).Inc()
	if !readyAt.IsZero() {
		metrics.TaintRemovalLatency.WithLabelValues(node.Name).Observe(r.clock.Since(readyAt).Seconds())
	}
	if r.eventsEnabled {
		r.recorder.Eventf(node, corev1.EventTypeNormal, "TaintRemoved", eventFmt, untaint.AgentNotReadyTaintKey, node.Name)
	}
	return ctrl.Result{}, nil
}

// reconcileTaintedNodeTimeouts evaluates scheduling vs readiness timeouts.
// When waitForCSIDriver is false, only agent pods are considered (csiPods is ignored).
// When waitForCSIDriver is true, both the node agent and CSI node-server must
// have at least one pod on the node with Status.StartTime before the readiness
// clock runs; the clock is the later of the two sides' max(StartTime). If a
// required workload is missing on the node (no agent pod or no CSI pod), the
// scheduling timeout anchored on node creation applies. If both workloads have
// pods on the node but either side still lacks StartTime, requeue after
// readinessTimeout (same coarse behavior as agent-only when StartTime is not set
// yet) so an old node CreationTimestamp does not instantly trip scheduling.
func (r *UntaintReconciler) reconcileTaintedNodeTimeouts(ctx context.Context, node *corev1.Node, log logr.Logger, agentPods *corev1.PodList, csiPods *corev1.PodList) (ctrl.Result, error) {
	now := r.clock.Now()
	agents := agentPods.Items

	if !r.waitForCSIDriver {
		if len(agents) == 0 {
			return r.schedulingTimeoutResult(ctx, node, log, now)
		}
		latestStart, ok := latestPodStartTime(agents)
		if !ok {
			return ctrl.Result{RequeueAfter: r.readinessTimeout}, nil
		}
		return r.readinessTimeoutResult(ctx, node, now, latestStart)
	}

	csis := []corev1.Pod(nil)
	if csiPods != nil {
		csis = csiPods.Items
	}
	if len(agents) == 0 || len(csis) == 0 {
		return r.schedulingTimeoutResult(ctx, node, log, now)
	}
	agentLatest, agentOK := latestPodStartTime(agents)
	csiLatest, csiOK := latestPodStartTime(csis)
	if !agentOK || !csiOK {
		// Match agent-only: pods exist but StartTime not populated yet — coarse
		// requeue, not node-age scheduling (avoids instant timeout on old nodes).
		return ctrl.Result{RequeueAfter: r.readinessTimeout}, nil
	}
	latestStart := agentLatest
	if csiLatest.After(latestStart) {
		latestStart = csiLatest
	}
	return r.readinessTimeoutResult(ctx, node, now, latestStart)
}

func (r *UntaintReconciler) schedulingTimeoutResult(ctx context.Context, node *corev1.Node, log logr.Logger, now time.Time) (ctrl.Result, error) {
	created := node.CreationTimestamp.Time
	if created.IsZero() {
		log.V(1).Info("Node has no CreationTimestamp; requeueing", "node", node.Name)
		return ctrl.Result{RequeueAfter: r.schedulingTimeout}, nil
	}
	elapsed := now.Sub(created)
	if elapsed >= r.schedulingTimeout {
		return r.applyTimeoutPolicy(ctx, node, metrics.UntaintTimeoutReasonScheduling, elapsed)
	}
	return ctrl.Result{RequeueAfter: r.schedulingTimeout - elapsed}, nil
}

func (r *UntaintReconciler) readinessTimeoutResult(ctx context.Context, node *corev1.Node, now, latestStart time.Time) (ctrl.Result, error) {
	elapsed := now.Sub(latestStart)
	if elapsed >= r.readinessTimeout {
		return r.applyTimeoutPolicy(ctx, node, metrics.UntaintTimeoutReasonReadiness, elapsed)
	}
	return ctrl.Result{RequeueAfter: r.readinessTimeout - elapsed}, nil
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
	metrics.TaintRemovalsTotal.WithLabelValues(node.Name, metrics.UntaintRemovalReasonTimeout).Inc()
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

// maxReadyTransitionTime returns the latest PodReady LastTransitionTime among
// pods that are currently Ready (used for removal latency when multiple pods
// must be ready).
func maxReadyTransitionTime(pods []corev1.Pod) (time.Time, bool) {
	var latest time.Time
	found := false
	for i := range pods {
		if ts, ok := podReadyTransition(&pods[i]); ok {
			if !found || ts.After(latest) {
				latest, found = ts, true
			}
		}
	}
	return latest, found
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
			builder.WithPredicates(r.podWatchPredicate()),
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

// podWatchPredicate enqueues a node's reconcile for pod Create/Update/Delete
// events on agent pods, and on CSI node-server pods when waitForCSIDriver is set.
// We deliberately do NOT filter to "ready transition" here because:
//   - a NotReady pod creation starts the readiness clock for that node, so the
//     reconciler needs to schedule a RequeueAfter against pod.Status.StartTime
//     (filtering out Create-as-NotReady would silently miss this clock until
//     some other event fires);
//   - a Delete must enqueue so a stuck-in-keep node can re-start its readiness
//     clock for the replacement pod;
//   - Reconcile itself is idempotent and quick (cache list + early-return on
//     !hasTaint), and bounds re-evaluation via RequeueAfter, so the extra
//     reconciles are cheap.
func (r *UntaintReconciler) podWatchPredicate() predicate.Predicate {
	isRelevant := func(o client.Object) bool {
		p, ok := o.(*corev1.Pod)
		return ok && (isAgentPod(p) || (r.waitForCSIDriver && isCSINodeServerPod(p)))
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isRelevant(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isRelevant(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isRelevant(e.Object) },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// isCSINodeServerPod reports whether the pod is a Datadog CSI node-server workload.
func isCSINodeServerPod(pod *corev1.Pod) bool {
	return pod.Labels[datadogcsidriver.AppLabelKey] == datadogcsidriver.NodeServerDaemonSetAppValue
}

// taintedNodePredicate enqueues a node when it carries the target taint on
// create, or when the target taint *appears* on update. The reconciler reruns
// itself via RequeueAfter while a timeout window is pending, so we do not need
// to fire on every unrelated node update.
//
// Node deletions enqueue unconditionally so Reconcile can drop the node's
// per-node metric series (see DeleteNodeSeries). We deliberately do NOT gate
// deletes on hasTaint: by the time a node is removed it has usually already had
// the taint stripped (that is precisely the node whose series must be cleaned
// up), so a hasTaint check would skip exactly the cases we care about.
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
		DeleteFunc: func(e event.DeleteEvent) bool {
			_, ok := e.Object.(*corev1.Node)
			return ok
		},
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// isAgentPod returns true if the pod has the agent component label.
func isAgentPod(pod *corev1.Pod) bool {
	return pod.Labels[common.AgentDeploymentComponentLabelKey] == constants.DefaultAgentResourceSuffix
}
