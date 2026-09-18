// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
)

const (
	datadogSyntheticsPrivateLocationFinalizerName = "finalizer.datadoghq.com/syntheticsprivatelocation"
)

// deleteResource returns the ResourceDeleteFunc for the generic finalizer: it
// deletes the remote private location when the CR is removed. Owned K8s
// resources are garbage-collected through owner references.
func deleteResource(logger logr.Logger, auth context.Context, ddClientSynthetics *datadogV1.SyntheticsApi) finalizer.ResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		if datadogID == "" {
			// The private location was never created remotely (or creation
			// never succeeded); owned resources are GC'd via owner refs.
			return nil
		}

		err := deletePrivateLocation(auth, ddClientSynthetics, datadogID)
		if err != nil {
			logger.Error(err, "failed to finalize private location", "Private Location ID", datadogID)
			return err
		}
		logger.Info("Successfully finalized DatadogSyntheticsPrivateLocation", "Private Location ID", datadogID)

		return nil
	}
}
