// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"sort"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
)

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// componentHealthEmitter reports state transitions of component-level issues:
// Emit when an issue type first appears on a component (its first affected pod),
// Resolve when the last affected pod recovers. The controller only calls these
// on transitions, so callers receive edges (appeared/cleared), not the full
// state on every reconcile.
//
// The default implementation logs and increments a Prometheus counter. A backend
// implementation that ships issues to the health-platform intake can be added
// behind this interface without changing the controller.
type componentHealthEmitter interface {
	Emit(ctx context.Context, issue componenthealth.ComponentIssue)
	Resolve(ctx context.Context, issue componenthealth.ComponentIssue)
}

// componentIssueKey identifies a component-level issue instance: the same issue
// type on the same component in the same namespace, independent of which pods
// currently exhibit it. Detection happens per pod, but this is the granularity
// at which issues are reported.
type componentIssueKey struct {
	namespace string
	component string
	issueType string
}

// componentIssueState is the live state of one component-level issue: the pods
// currently exhibiting it (keyed by pod name) and the severity of the most
// recent detection. The issue is active while pods is non-empty.
type componentIssueState struct {
	severity componenthealth.Severity
	pods     map[string]struct{}
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

	// mu guards reported. reported holds every currently-active component-level
	// issue and the set of pods exhibiting it, so the controller emits only on
	// presence transitions (first pod affected / last pod recovered) rather than
	// on every reconcile.
	mu       sync.Mutex
	reported map[componentIssueKey]*componentIssueState
}

// NewComponentHealthReconciler builds a ComponentHealthReconciler with the
// default logging/metric emitter.
func NewComponentHealthReconciler(c client.Client, log logr.Logger) *ComponentHealthReconciler {
	return &ComponentHealthReconciler{
		client:           c,
		log:              log,
		restartThreshold: defaultRestartThreshold,
		emitter:          &logEmitter{log: log},
		reported:         map[componentIssueKey]*componentIssueState{},
	}
}

// Reconcile evaluates a single managed component pod and folds its detected
// issues into the component-level state, emitting only the resulting transitions.
func (r *ComponentHealthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pod := &corev1.Pod{}
	if err := r.client.Get(ctx, req.NamespacedName, pod); err != nil {
		if apierrors.IsNotFound(err) {
			// Pod is gone: drop it from every component issue it contributed to.
			r.forgetPod(ctx, req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// A pod being deleted is treated like a gone pod.
	if !pod.DeletionTimestamp.IsZero() {
		r.forgetPod(ctx, req.NamespacedName)
		return ctrl.Result{}, nil
	}

	component := componenthealth.ManagedComponent(pod)
	issues := componenthealth.DetectPodIssues(pod, r.restartThreshold)
	r.reconcilePod(ctx, req.NamespacedName, component, issues)
	return ctrl.Result{}, nil
}

// reconcilePod folds one pod's currently-detected issues into the component-level
// state: it registers the pod on the issues it now exhibits and removes it from
// the component issues it no longer does, emitting Emit/Resolve on the component's
// presence transitions.
func (r *ComponentHealthReconciler) reconcilePod(ctx context.Context, pod types.NamespacedName, component string, issues []componenthealth.DetectedIssue) {
	current := make(map[string]componenthealth.DetectedIssue, len(issues))
	for _, issue := range issues {
		current[issue.IssueType] = issue
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
		r.dropPod(ctx, key, state, pod.Name)
	}

	// Register the pod on every issue it currently exhibits.
	for issueType, issue := range current {
		key := componentIssueKey{namespace: pod.Namespace, component: component, issueType: issueType}
		r.addPod(ctx, key, issue.Severity, pod.Name)
	}
}

// addPod records that a pod exhibits a component issue, emitting Emit if this is
// the first affected pod (the issue transitioning to active). Caller holds mu.
func (r *ComponentHealthReconciler) addPod(ctx context.Context, key componentIssueKey, severity componenthealth.Severity, pod string) {
	state := r.reported[key]
	if state == nil {
		state = &componentIssueState{pods: map[string]struct{}{}}
		r.reported[key] = state
	}
	firstPod := len(state.pods) == 0
	state.pods[pod] = struct{}{}
	state.severity = severity

	if firstPod {
		r.emitter.Emit(ctx, r.issueFor(key, state))
	}
	r.setActiveGauge(key, len(state.pods))
}

// dropPod removes a pod from a component issue, emitting Resolve and clearing the
// state when the last affected pod is gone. Caller holds mu.
func (r *ComponentHealthReconciler) dropPod(ctx context.Context, key componentIssueKey, state *componentIssueState, pod string) {
	if _, ok := state.pods[pod]; !ok {
		return
	}
	delete(state.pods, pod)
	if len(state.pods) == 0 {
		r.emitter.Resolve(ctx, r.issueFor(key, state))
		r.setActiveGauge(key, 0)
		delete(r.reported, key)
		return
	}
	r.setActiveGauge(key, len(state.pods))
}

// forgetPod removes a pod from every component issue it contributed to (used when
// the pod disappears), resolving any issue whose last pod it was.
func (r *ComponentHealthReconciler) forgetPod(ctx context.Context, pod types.NamespacedName) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, state := range r.reported {
		if key.namespace != pod.Namespace {
			continue
		}
		r.dropPod(ctx, key, state, pod.Name)
	}
}

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
		Severity:     state.severity,
		Namespace:    key.namespace,
		AffectedPods: pods,
	}
}

func (r *ComponentHealthReconciler) setActiveGauge(key componentIssueKey, affectedPods int) {
	metrics.ComponentHealthIssuesActive.WithLabelValues(key.component, key.issueType).Set(float64(affectedPods))
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

	return ctrl.NewControllerManagedBy(mgr).
		Named(componentHealthControllerName).
		For(&corev1.Pod{}, builder.WithPredicates(pred)).
		Complete(r)
}

// logEmitter is the default componentHealthEmitter: it logs each component-level
// transition and increments the issues-detected counter on Emit.
type logEmitter struct {
	log logr.Logger
}

func (e *logEmitter) Emit(_ context.Context, issue componenthealth.ComponentIssue) {
	e.log.Info("component health issue detected",
		"component", issue.Component,
		"issue_type", issue.IssueType,
		"severity", issue.Severity,
		"namespace", issue.Namespace,
		"affected_pods", issue.AffectedPods,
	)
	metrics.ComponentHealthIssuesDetected.WithLabelValues(issue.Component, issue.IssueType).Inc()
}

func (e *logEmitter) Resolve(_ context.Context, issue componenthealth.ComponentIssue) {
	e.log.Info("component health issue resolved",
		"component", issue.Component,
		"issue_type", issue.IssueType,
		"namespace", issue.Namespace,
	)
}
