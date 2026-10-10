// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils provides the object management shared by the DatadogBYOCCluster and
// DatadogObservabilityPipelinesWorker controllers.
package utils

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ErrObjectConflict reports an existing object with a managed name that the owner does not control.
var ErrObjectConflict = errors.New("exists and is not controlled by the owner")

// GetFresh reads the object from the cache, and from the API server when the cache does not have it.
func GetFresh(ctx context.Context, cache, apiReader client.Reader, object client.Object) error {
	key := client.ObjectKeyFromObject(object)
	err := cache.Get(ctx, key, object)
	if !apierrors.IsNotFound(err) {
		return err
	}
	// The cache may not have observed an object that was just created.
	return apiReader.Get(ctx, key, object)
}

// ApplyControlled sets owner as the controller of desired and applies desired with a forced server-side apply.
// It returns ErrObjectConflict without writing when an object with the same name exists and owner does not control it.
func ApplyControlled(ctx context.Context, c client.Client, apiReader client.Reader, scheme *runtime.Scheme, owner, desired client.Object, fieldOwner string) error {
	current := desired.DeepCopyObject().(client.Object)
	switch err := GetFresh(ctx, c, apiReader, current); {
	case apierrors.IsNotFound(err):
	case err != nil:
		return err
	case !metav1.IsControlledBy(current, owner):
		// The forced apply would otherwise overwrite the object and adopt it.
		return ErrObjectConflict
	}
	if err := controllerutil.SetControllerReference(owner, desired, scheme); err != nil {
		return err
	}
	gvk, err := apiutil.GVKForObject(desired, scheme)
	if err != nil {
		return err
	}
	desired.GetObjectKind().SetGroupVersionKind(gvk)
	return c.Patch(ctx, desired, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner))
}

// DeleteIfControlled deletes the object when it is controlled by owner and reports whether a deletion was requested.
// The ownership check reads the object through reader, and the deletion fails if the object changed since that read.
func DeleteIfControlled(ctx context.Context, c client.Client, reader client.Reader, owner, object client.Object, opts ...client.DeleteOption) (bool, error) {
	key := client.ObjectKeyFromObject(object)
	if err := reader.Get(ctx, key, object); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get %T %s: %w", object, key, err)
	}
	if !metav1.IsControlledBy(object, owner) {
		return false, nil
	}
	uid, resourceVersion := object.GetUID(), object.GetResourceVersion()
	opts = append(opts, client.Preconditions{UID: &uid, ResourceVersion: &resourceVersion})
	if err := c.Delete(ctx, object, opts...); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("delete %T %s: %w", object, key, err)
	}
	return true, nil
}
