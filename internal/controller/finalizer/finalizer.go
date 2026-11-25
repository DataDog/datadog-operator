// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package finalizer

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// OrphanedEventReason is the reason of the Event recorded when a CR with the
// orphan management policy is deleted without deleting its Datadog object.
const OrphanedEventReason = "Orphaned"

// ResourceDeleteFunc performs controller-specific cleanup when a resource is being deleted.
// If it returns an error, the finalizer is NOT removed — the object stays in a terminating
// state and the controller retries on the next reconcile. If it returns nil, the finalizer
// is removed and Kubernetes proceeds with deletion.
//
// An empty datadogID signals "no remote resource to delete" for controllers that call
// the Datadog API.
type ResourceDeleteFunc func(ctx context.Context, k8sObj client.Object, datadogID string) error

type Finalizer struct {
	logger     logr.Logger
	client     client.Client
	deleteFunc ResourceDeleteFunc

	// defaultRequeuePeriod is how often to requeue while waiting for Kubernetes
	// to garbage-collect the object after the finalizer has been removed.
	// The add-finalizer path always uses an immediate requeue to avoid
	// delaying the first real reconcile pass.
	defaultRequeuePeriod    time.Duration
	defaultErrRequeuePeriod time.Duration

	orphanPolicy *orphanPolicy
}

type orphanPolicy struct {
	recorder record.EventRecorder
	onOrphan func(client.Object)
}

// Option configures a Finalizer.
type Option func(*Finalizer)

// WithOrphanPolicy makes the finalizer honor the
// datadoghq.com/management-policy: orphan annotation: deleting an annotated
// object removes the finalizer without calling the ResourceDeleteFunc, so the
// Datadog object is kept. recorder, if not nil, records an Orphaned event.
// onOrphan, if not nil, runs before the finalizer is removed, for local
// cleanup the ResourceDeleteFunc would otherwise do (for example metrics).
func WithOrphanPolicy(recorder record.EventRecorder, onOrphan func(client.Object)) Option {
	return func(f *Finalizer) {
		f.orphanPolicy = &orphanPolicy{recorder: recorder, onOrphan: onOrphan}
	}
}

// NewOrphanFinalizer returns the finalizer of an object that has the orphan
// management policy. It never deletes the Datadog object, so controllers can
// run it without Datadog credentials: the annotation then releases an object
// even when the credentials are unavailable.
func NewOrphanFinalizer(
	logger logr.Logger,
	c client.Client,
	defaultRequeuePeriod time.Duration,
	defaultErrRequeuePeriod time.Duration,
	recorder record.EventRecorder,
	onOrphan func(client.Object),
) *Finalizer {
	refuseDelete := func(context.Context, client.Object, string) error {
		return errors.New("refusing to delete the Datadog object of a resource without the orphan management policy")
	}
	return NewFinalizer(logger, c, refuseDelete, defaultRequeuePeriod, defaultErrRequeuePeriod, WithOrphanPolicy(recorder, onOrphan))
}

func NewFinalizer(
	logger logr.Logger,
	client client.Client,
	deleteFunc ResourceDeleteFunc,
	defaultRequeuePeriod time.Duration,
	defaultErrRequeuePeriod time.Duration,
	opts ...Option,
) *Finalizer {
	f := &Finalizer{
		logger:                  logger,
		client:                  client,
		deleteFunc:              deleteFunc,
		defaultRequeuePeriod:    defaultRequeuePeriod,
		defaultErrRequeuePeriod: defaultErrRequeuePeriod,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// HandleFinalizer doc https://book.kubebuilder.io/reference/using-finalizers.html
func (f *Finalizer) HandleFinalizer(ctx context.Context, clientObj client.Object, datadogID string, finalizerName string) (ctrl.Result, error) {
	// examine DeletionTimestamp to determine if object is under deletion
	if clientObj.GetDeletionTimestamp().IsZero() {
		// The object is not being deleted. If it does not have a finalizer, add it and update the object.
		if !controllerutil.ContainsFinalizer(clientObj, finalizerName) {
			f.logger.Info("Object does not have a finalizer; adding finalizer",
				"kind", clientObj.GetObjectKind().GroupVersionKind().Kind,
				"datadogID", datadogID,
				"finalizername", finalizerName,
			)
			controllerutil.AddFinalizer(clientObj, finalizerName)
			err := f.client.Update(ctx, clientObj)
			if err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: f.defaultErrRequeuePeriod}, err
			}
			// Requeue immediately so the next reconcile works with the updated
			// object from the API server. Using a delay here would block the
			// first real reconcile pass and fail tight Eventually() timeouts.
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		f.logger.Info("Object being deleted", "kind", clientObj.GetObjectKind().GroupVersionKind().Kind, "finalizername", finalizerName)
		// The object is being deleted
		if controllerutil.ContainsFinalizer(clientObj, finalizerName) {
			// Keep the Datadog object of an orphaned object. The annotation can
			// be set on an object that is already terminating, which releases
			// one whose remote deletion keeps failing.
			orphaned := f.orphanPolicy != nil && v1alpha1.IsOrphaned(clientObj)
			if orphaned {
				f.logger.Info("Object has the orphan management policy; removing finalizer without deleting the Datadog object", "datadogID", datadogID)
			} else if err := f.deleteFunc(ctx, clientObj, datadogID); err != nil {
				// If deletion has failed, retry
				return ctrl.Result{Requeue: true, RequeueAfter: f.defaultErrRequeuePeriod}, err
			}
			controllerutil.RemoveFinalizer(clientObj, finalizerName)
			if err := f.client.Update(ctx, clientObj); err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: f.defaultErrRequeuePeriod}, err
			}
			// Only once the finalizer is gone, so that a failed update and its
			// retry don't run these twice.
			if orphaned {
				if f.orphanPolicy.onOrphan != nil {
					f.orphanPolicy.onOrphan(clientObj)
				}
				if f.orphanPolicy.recorder != nil {
					f.orphanPolicy.recorder.Event(clientObj, corev1.EventTypeNormal, OrphanedEventReason, "Finalizer removed without deleting the Datadog object (management policy: orphan)")
				}
			}
		}
		// Requeue on a slow cadence while waiting for Kubernetes to
		// garbage-collect the object. Watch events will usually wake us up
		// sooner; this is a safety net.
		return ctrl.Result{RequeueAfter: f.defaultRequeuePeriod}, nil
	}
	return ctrl.Result{}, nil
}
