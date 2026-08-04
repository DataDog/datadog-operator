// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogdashboard

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

const (
	datadogDashboardFinalizer = "finalizer.datadoghq.com/dashboard"
)

func (r *Reconciler) deleteResource(logger logr.Logger, auth context.Context) finalizer.ResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		if datadogID != "" {
			err := deleteDashboard(auth, r.datadogClient, datadogID)
			if err != nil {
				logger.Error(err, "failed to finalize dashboard", "dashboard ID", fmt.Sprint(datadogID))
				return nil
			}
			logger.Info("Successfully finalized DatadogDashboard", "dashboard ID", fmt.Sprint(datadogID))
		}
		event := buildEventInfo(k8sObj.GetName(), k8sObj.GetNamespace(), datadog.DeletionEvent)
		r.recordEvent(k8sObj, event)
		return nil
	}
}
