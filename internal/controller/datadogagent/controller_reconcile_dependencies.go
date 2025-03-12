// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/errors"
)

func (r *Reconciler) manageDependencies(ctx context.Context, logger logr.Logger, resourceManagers feature.ResourceManagers, instance *v2alpha1.DatadogAgent, requiredComponents feature.RequiredComponents, features []feature.Feature, depsStore *store.Store) error {
	if err := r.manageGlobalDependencies(logger, resourceManagers, instance, requiredComponents); err != nil {
		return err
	}

	if err := r.manageFeatureDependencies(logger, features, requiredComponents, resourceManagers); err != nil {
		return err
	}

	if err := r.overrideDependencies(logger, resourceManagers, instance); err != nil {
		return err
	}

	if err := r.applyAndCleanupDependencies(ctx, logger, depsStore); err != nil {
		return err
	}

	return nil
}

// setupDependencies initializes the store and resource managers.
func (r *Reconciler) setupDependencies(instance *v2alpha1.DatadogAgent, logger logr.Logger) (*store.Store, feature.ResourceManagers) {
	storeOptions := &store.StoreOptions{
		SupportCilium: r.options.SupportCilium,
		PlatformInfo:  r.platformInfo,
		Logger:        logger,
		Scheme:        r.scheme,
	}
	depsStore := store.NewStore(instance, storeOptions)
	resourceManagers := feature.NewResourceManagers(depsStore)
	return depsStore, resourceManagers
}

// manageGlobalDependencies wraps the global dependency logic.
func (r *Reconciler) manageGlobalDependencies(logger logr.Logger, resourceManagers feature.ResourceManagers, instance *datadoghqv2alpha1.DatadogAgent, components feature.RequiredComponents) error {
	errs := global.Dependencies(logger, instance, resourceManagers, components)
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// manageFeatureDependencies iterates over features to set up dependencies.
func (r *Reconciler) manageFeatureDependencies(logger logr.Logger, features []feature.Feature, requiredComponents feature.RequiredComponents, resourceManagers feature.ResourceManagers) error {
	var errs []error
	for _, feat := range features {
		logger.V(1).Info("Managing dependencies", "featureID", feat.ID())
		if err := feat.ManageDependencies(resourceManagers, requiredComponents); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// overrideDependencies wraps the dependency override logic.
func (r *Reconciler) overrideDependencies(logger logr.Logger, resourceManagers feature.ResourceManagers, instance *datadoghqv2alpha1.DatadogAgent) error {
	errs := override.Dependencies(logger, resourceManagers, instance)
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// applyAndCleanupDependencies applies pending changes and cleans up unused dependencies.
func (r *Reconciler) applyAndCleanupDependencies(ctx context.Context, logger logr.Logger, depsStore *store.Store) error {
	var errs []error
	errs = append(errs, depsStore.Apply(ctx, r.client)...)
	if len(errs) > 0 {
		logger.V(2).Info("Dependencies apply error", "errs", errs)
		return errors.NewAggregate(errs)
	}
	if errs = depsStore.Cleanup(ctx, r.client); len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}
