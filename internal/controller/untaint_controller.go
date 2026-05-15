// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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
)

const (
	untaintControllerName = "Untaint"

	// Fixed taint to remove
	agentNotReadyTaintKey    = "agent.datadoghq.com/not-ready"
	agentNotReadyTaintValue  = "presence"
	agentNotReadyTaintEffect = corev1.TaintEffectNoSchedule
)

// UntaintReconciler watches agent pods and removes the taint
// agent.datadoghq.com/not-ready=presence:NoSchedule from their nodes once Ready.
type UntaintReconciler struct {
	client        client.Client
	log           logr.Logger
	recorder      record.EventRecorder
	eventsEnabled bool
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is called with the name of a Node and removes the agent-not-ready taint
// if a Ready agent pod exists on that node.
func (r *UntaintReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.WithValues("node", req.Name)

	// 1. Get the Node from cache
	node := &corev1.Node{}
	if err := r.client.Get(ctx, req.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get node %s: %w", req.Name, err)
	}

	// 2. Check if the taint we care about is present
	if !hasTaint(node) {
		return ctrl.Result{}, nil
	}

	// 3. List agent pods on this node
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
	})
	if err := r.client.List(ctx, podList,
		client.MatchingLabelsSelector{Selector: labelSelector},
		client.MatchingFields{"spec.nodeName": req.Name},
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list pods on node %s: %w", req.Name, err)
	}

	// 4. Check if any agent pod is Ready; record its Ready transition time for the latency metric
	var readyTransitionTime *time.Time
	anyReady := false
	for i := range podList.Items {
		pod := &podList.Items[i]
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				anyReady = true
				if !c.LastTransitionTime.IsZero() {
					t := c.LastTransitionTime.Time
					readyTransitionTime = &t
				}
				break
			}
		}
		if anyReady {
			break
		}
	}

	if !anyReady {
		log.V(1).Info("No ready agent pod found, skipping taint removal")
		return ctrl.Result{}, nil
	}

	// 5. Remove the taint via JSON patch (test-and-set for optimistic concurrency)
	if err := r.removeTaint(ctx, node); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove taint from node %s: %w", req.Name, err)
	}

	log.Info("Removed agent-not-ready taint from node")

	// 6. Record metrics
	metrics.TaintRemovalsTotal.Inc()
	if readyTransitionTime != nil {
		metrics.TaintRemovalLatency.Observe(time.Since(*readyTransitionTime).Seconds())
	}

	// 7. Optionally emit Kubernetes event
	if r.eventsEnabled {
		r.recorder.Eventf(node, corev1.EventTypeNormal, "TaintRemoved",
			"Removed taint %s from node %s after agent became ready", agentNotReadyTaintKey, node.Name)
	}

	return ctrl.Result{}, nil
}

// hasTaint returns true if the node has the agent-not-ready taint.
func hasTaint(node *corev1.Node) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == agentNotReadyTaintKey && t.Value == agentNotReadyTaintValue && t.Effect == agentNotReadyTaintEffect {
			return true
		}
	}
	return false
}

type jsonPatchOp struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

// removeTaint patches the node to remove the agent-not-ready taint using a JSON test-and-set patch.
func (r *UntaintReconciler) removeTaint(ctx context.Context, node *corev1.Node) error {
	newTaints := make([]corev1.Taint, 0, len(node.Spec.Taints))
	for _, t := range node.Spec.Taints {
		if t.Key == agentNotReadyTaintKey && t.Value == agentNotReadyTaintValue && t.Effect == agentNotReadyTaintEffect {
			continue
		}
		newTaints = append(newTaints, t)
	}

	if len(newTaints) == len(node.Spec.Taints) {
		return nil // taint not present
	}

	// Use JSON patch with test precondition for optimistic concurrency.
	patch := []jsonPatchOp{
		{Op: "test", Path: "/spec/taints", Value: node.Spec.Taints},
		{Op: "replace", Path: "/spec/taints", Value: newTaints},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	// TODO: If this fails due to conflict with optimistic concurrency, we should requeue
	// the reconciliation instead of returning an error.
	return r.client.Patch(ctx, node, client.RawPatch(types.JSONPatchType, patchBytes))
}

// SetupWithManager sets up the controller to watch agent pods and map them to their nodes.
func (r *UntaintReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index pods by spec.nodeName so we can list pods on a specific node efficiently.
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
		pod, ok := obj.(*corev1.Pod)
		if !ok || pod.Spec.NodeName == "" {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return fmt.Errorf("failed to index pods by spec.nodeName: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(untaintControllerName).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.podToNodeRequest),
			builder.WithPredicates(r.agentPodReadinessPredicate()),
		).
		Complete(r)
}

// podToNodeRequest maps a Pod event to a reconcile.Request for the pod's node.
func (r *UntaintReconciler) podToNodeRequest(ctx context.Context, obj client.Object) []reconcile.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok || pod.Spec.NodeName == "" {
		return nil
	}
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: pod.Spec.NodeName}},
	}
}

// agentPodReadinessPredicate returns a predicate that only processes agent pods
// when they transition to Ready.
func (r *UntaintReconciler) agentPodReadinessPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return r.isAgentPod(e.Object) && isPodReady(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !r.isAgentPod(e.ObjectNew) {
				return false
			}
			wasReady := isPodReady(e.ObjectOld)
			isReady := isPodReady(e.ObjectNew)
			return !wasReady && isReady
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

// isAgentPod returns true if the object has the agent component label.
func (r *UntaintReconciler) isAgentPod(obj client.Object) bool {
	lbls := obj.GetLabels()
	return lbls[common.AgentDeploymentComponentLabelKey] == constants.DefaultAgentResourceSuffix
}

// isPodReady returns true if the pod has the Ready condition set to True.
func isPodReady(obj client.Object) bool {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return false
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
