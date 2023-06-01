// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2023 Datadog, Inc.

package finalizer

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ExternalResourceDeleteFunc func(ctx context.Context, k8sObj client.Object, datadogID string) error

type Finalizer struct {
	logger      logr.Logger
	client      client.Client
	deleterFunc ExternalResourceDeleteFunc

	defaultRequeuePeriod    time.Duration
	defaultErrRequeuePeriod time.Duration
}

func NewFinalizer(
	logger logr.Logger,
	client client.Client,
	deleteFunc ExternalResourceDeleteFunc,
	defaultRequeuePeriod time.Duration,
	defaultErrRequeuePeriod time.Duration,
) *Finalizer {
	return &Finalizer{
		logger:                  logger,
		client:                  client,
		deleterFunc:             deleteFunc,
		defaultRequeuePeriod:    defaultRequeuePeriod,
		defaultErrRequeuePeriod: defaultErrRequeuePeriod,
	}
}

// HandleFinalizer doc https://book.kubebuilder.io/reference/using-finalizers.html
func (f *Finalizer) HandleFinalizer(ctx context.Context, clientObj client.Object, datadogID string, finalizerName string) (ctrl.Result, error) {
	// examine DeletionTimestamp to determine if object is under deletion
	if clientObj.GetDeletionTimestamp().IsZero() {
		f.logger.Info("Object not being deleted", "kind", clientObj.GetObjectKind(), "finalizername", finalizerName)
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(clientObj, finalizerName) {
			f.logger.Info("Object does not have finalizer adding finalizer",
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
		f.logger.Info("Object under the deletion", "kind", clientObj.GetObjectKind(), "finalizername", finalizerName)
		// The object is being deleted
		if controllerutil.ContainsFinalizer(clientObj, finalizerName) {
			// our finalizer is present, so lets handle any external dependency
			err := f.deleterFunc(ctx, clientObj, datadogID)
			if err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{Requeue: true, RequeueAfter: f.defaultErrRequeuePeriod}, err
			}
			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(clientObj, finalizerName)
			if err := f.client.Update(ctx, clientObj); err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: f.defaultErrRequeuePeriod}, err
			}
		}
	}
	return ctrl.Result{}, nil
}
