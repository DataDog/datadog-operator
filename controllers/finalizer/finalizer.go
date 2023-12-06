// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package finalizer

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResourceDeleteFunc func(ctx context.Context, k8sObj client.Object, datadogID string) error

type Finalizer struct {
	logger     logr.Logger
	client     client.Client
	deleteFunc ResourceDeleteFunc

	defaultRequeuePeriod    time.Duration
	defaultErrRequeuePeriod time.Duration
}

func NewFinalizer(
	logger logr.Logger,
	client client.Client,
	deleteFunc ResourceDeleteFunc,
	defaultRequeuePeriod time.Duration,
	defaultErrRequeuePeriod time.Duration,
) *Finalizer {
	return &Finalizer{
		logger:                  logger,
		client:                  client,
		deleteFunc:              deleteFunc,
		defaultRequeuePeriod:    defaultRequeuePeriod,
		defaultErrRequeuePeriod: defaultErrRequeuePeriod,
	}
}

// HandleFinalizer doc https://book.kubebuilder.io/reference/using-finalizers.html
func (f *Finalizer) HandleFinalizer(ctx context.Context, clientObj client.Object, datadogID string, finalizerName string) (ctrl.Result, error) {
	// examine DeletionTimestamp to determine if object is under deletion
	if clientObj.GetDeletionTimestamp().IsZero() {
		// The object is not being deleted. If it does not have a finalizer, add it and update the object.
		if !controllerutil.ContainsFinalizer(clientObj, finalizerName) {
			f.logger.Info("Object does not have a finalizer; adding finalizer",
				"kind", clientObj.GetObjectKind(),
				"datadogID", datadogID,
				"finalizername", finalizerName,
			)
			controllerutil.AddFinalizer(clientObj, finalizerName)
			err := f.client.Update(ctx, clientObj)
			if err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: f.defaultErrRequeuePeriod}, err
			}
		}
	} else {
		f.logger.Info("Object being deleted", "kind", clientObj.GetObjectKind(), "finalizername", finalizerName)
		// The object is being deleted
		if controllerutil.ContainsFinalizer(clientObj, finalizerName) {
			// Delete resource
			err := f.deleteFunc(ctx, clientObj, datadogID)
			if err != nil {
				// If deletion has failed, retry
				return ctrl.Result{Requeue: true, RequeueAfter: f.defaultErrRequeuePeriod}, err
			}
			controllerutil.RemoveFinalizer(clientObj, finalizerName)
			if err := f.client.Update(ctx, clientObj); err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: f.defaultErrRequeuePeriod}, err
			}
		}
	}
	return ctrl.Result{}, nil
}
