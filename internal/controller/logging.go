// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// reconcileLogConstructor names both controller-runtime and reconciler logs and
// identifies the reconciled resource once, using flat namespace/name fields.
// Controller metadata is retained for consumers using a different log encoder.
// Controller-runtime adds reconcileID to the returned per-request logger.
func reconcileLogConstructor(base logr.Logger, kind string) func(*reconcile.Request) logr.Logger {
	logger := base.WithName("controllers").WithName(kind).WithValues(
		"controller", strings.ToLower(kind),
		"controllerGroup", v1alpha1.GroupVersion.Group,
		"controllerKind", kind,
	)
	return func(req *reconcile.Request) logr.Logger {
		if req == nil {
			return logger
		}
		return logger.WithValues("namespace", req.Namespace, "name", req.Name)
	}
}
