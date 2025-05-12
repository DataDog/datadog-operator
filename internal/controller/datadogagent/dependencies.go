// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
)

func (r *Reconciler) manageDDADependenciesWithDDAI(ctx context.Context, logger logr.Logger, instance *v2alpha1.DatadogAgent) error {
	depsStore, resourceManagers := r.setupDependencies(instance, logger)

	// Credentials
	if err := global.AddCredentialDependencies(logger, instance, resourceManagers); err != nil {
		return err
	}
	// DCA token
	if err := global.AddDCATokenDependencies(logger, instance, resourceManagers); err != nil {
		return err
	}

	// Apply and cleanup dependencies
	if err := r.applyAndCleanupDependencies(ctx, logger, depsStore); err != nil {
		return err
	}
	return nil
}
