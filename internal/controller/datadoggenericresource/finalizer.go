// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

const (
	datadogGenericResourceFinalizer = "finalizer.datadoghq.com/genericresource"
)

func (r *Reconciler) deleteResource(logger logr.Logger, auth context.Context, handler ResourceHandler) finalizer.ResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		instance, ok := k8sObj.(*datadoghqv1alpha1.DatadogGenericResource)
		if !ok {
			return fmt.Errorf("unexpected object type %T", k8sObj)
		}
		if datadogID == "" {
			logger.Info("No Datadog resource ID found; skipping remote deletion")
			event := buildEventInfo(k8sObj.GetName(), k8sObj.GetNamespace(), datadog.DeletionEvent)
			r.recordEvent(k8sObj, event)
			return nil
		}
		err := handler.deleteResource(auth, instance)
		if err != nil {
			logger.Error(err, "failed to finalize", "custom resource Id", fmt.Sprint(datadogID))
			return err
		}
		logger.Info("Successfully finalized DatadogGenericResource", "custom resource Id", fmt.Sprint(datadogID))
		event := buildEventInfo(k8sObj.GetName(), k8sObj.GetNamespace(), datadog.DeletionEvent)
		r.recordEvent(k8sObj, event)
		return nil
	}
}
