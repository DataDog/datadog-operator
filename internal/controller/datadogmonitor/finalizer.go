// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

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
	datadogMonitorFinalizer = "finalizer.monitor.datadoghq.com"
)

func (r *Reconciler) deleteResource(logger logr.Logger, auth context.Context) finalizer.ResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		dm, ok := k8sObj.(*datadoghqv1alpha1.DatadogMonitor)
		if !ok {
			return fmt.Errorf("unexpected object type %T", k8sObj)
		}

		if r.forwarders != nil {
			r.forwarders.Unregister(dm)
		}

		if dm.Status.Primary {
			err := deleteMonitor(auth, r.datadogClient, dm.Status.ID)
			if err != nil {
				logger.Error(err, "failed to finalize monitor", "Monitor ID", fmt.Sprint(dm.Status.ID))
				return err
			}
			logger.Info("Successfully finalized DatadogMonitor", "Monitor ID", fmt.Sprint(dm.Status.ID))
			event := buildEventInfo(dm.Name, dm.Namespace, datadog.DeletionEvent)
			r.recordEvent(dm, event)
		}

		return nil
	}
}
