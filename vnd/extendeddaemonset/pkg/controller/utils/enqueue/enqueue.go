// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package enqueue contains Object enqueue helpers.
package enqueue

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

var _ handler.EventHandler = &RequestForExtendedDaemonSetLabel{}

// RequestForExtendedDaemonSetLabel enqueues Requests for the ExtendedDaemonSet corresponding to the label value.
type RequestForExtendedDaemonSetLabel struct{}

// Create implements EventHandler.
func (e *RequestForExtendedDaemonSetLabel) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

// Update implements EventHandler.
func (e *RequestForExtendedDaemonSetLabel) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.ObjectOld, q)
	e.add(evt.ObjectNew, q)
}

// Delete implements EventHandler.
func (e *RequestForExtendedDaemonSetLabel) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

// Generic implements EventHandler.
func (e *RequestForExtendedDaemonSetLabel) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

func (e RequestForExtendedDaemonSetLabel) add(meta metav1.Object, q workqueue.RateLimitingInterface) {
	value, ok := meta.GetLabels()[v1alpha1.ExtendedDaemonSetNameLabelKey]
	if ok {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: meta.GetNamespace(),
				Name:      value,
			},
		}
		q.Add(req)
	}
}

var _ handler.EventHandler = &RequestForExtendedDaemonSetLabel{}

// RequestForExtendedDaemonSetReplicaSetLabel enqueues Requests for the ExtendedDaemonSet corresponding to the label value.
type RequestForExtendedDaemonSetReplicaSetLabel struct{}

// Create implements EventHandler.
func (e *RequestForExtendedDaemonSetReplicaSetLabel) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

// Update implements EventHandler.
func (e *RequestForExtendedDaemonSetReplicaSetLabel) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.ObjectOld, q)
	e.add(evt.ObjectNew, q)
}

// Delete implements EventHandler.
func (e *RequestForExtendedDaemonSetReplicaSetLabel) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

// Generic implements EventHandler.
func (e RequestForExtendedDaemonSetReplicaSetLabel) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

func (e *RequestForExtendedDaemonSetReplicaSetLabel) add(meta metav1.Object, q workqueue.RateLimitingInterface) {
	value, ok := meta.GetLabels()[v1alpha1.ExtendedDaemonSetReplicaSetNameLabelKey]
	if ok {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: meta.GetNamespace(),
				Name:      value,
			},
		}
		q.Add(req)
	}
}

var _ handler.EventHandler = &RequestForExtendedDaemonSetStatus{}

// RequestForExtendedDaemonSetStatus enqueues Requests for the ExtendedDaemonSet corresponding to the label value.
type RequestForExtendedDaemonSetStatus struct{}

// Create implements EventHandler.
func (e *RequestForExtendedDaemonSetStatus) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

// Update implements EventHandler.
func (e *RequestForExtendedDaemonSetStatus) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.ObjectOld, q)
	e.add(evt.ObjectNew, q)
}

// Delete implements EventHandler.
func (e *RequestForExtendedDaemonSetStatus) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

// Generic implements EventHandler.
func (e *RequestForExtendedDaemonSetStatus) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

func (e RequestForExtendedDaemonSetStatus) add(obj runtime.Object, q workqueue.RateLimitingInterface) {
	// convert unstructured.Unstructured to a ExtendedDaemonSet.
	eds, ok := obj.(*v1alpha1.ExtendedDaemonSet)
	if !ok {
		return
	}

	if eds.Status.ActiveReplicaSet != "" {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: eds.Namespace,
				Name:      eds.Status.ActiveReplicaSet,
			},
		}
		q.Add(req)
	}
}

// NewRequestForAllReplicaSetFromNodeEvent returns new instance of RequestForAllReplicaSetFromNodeEvent.
func NewRequestForAllReplicaSetFromNodeEvent(c client.Client) handler.EventHandler {
	return &RequestForAllReplicaSetFromNodeEvent{
		client: c,
	}
}

var _ handler.EventHandler = &RequestForAllReplicaSetFromNodeEvent{}

// RequestForAllReplicaSetFromNodeEvent enqueues Requests for the ExtendedDaemonSet corresponding to the label value.
type RequestForAllReplicaSetFromNodeEvent struct {
	client client.Client
}

// Create implements EventHandler.
func (e *RequestForAllReplicaSetFromNodeEvent) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.add(q)
}

// Update implements EventHandler.
func (e *RequestForAllReplicaSetFromNodeEvent) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	// e.add(q)
}

// Delete implements EventHandler.
func (e *RequestForAllReplicaSetFromNodeEvent) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.add(q)
}

// Generic implements EventHandler.
func (e *RequestForAllReplicaSetFromNodeEvent) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	// e.add(q)
}

func (e RequestForAllReplicaSetFromNodeEvent) add(q workqueue.RateLimitingInterface) {
	rsList := &v1alpha1.ExtendedDaemonSetReplicaSetList{}
	err := e.client.List(context.TODO(), rsList)
	if err != nil {
		return
	}

	for _, rs := range rsList.Items {
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: rs.Namespace,
				Name:      rs.Name,
			},
		}

		q.Add(req)
	}
}
