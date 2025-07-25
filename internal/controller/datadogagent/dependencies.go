// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

func (r *Reconciler) manageDDADependenciesWithDDAI(ctx context.Context, logger logr.Logger, instance *v2alpha1.DatadogAgent, newDDAStatus *v2alpha1.DatadogAgentStatus) error {
	depsStore, resourceManagers := r.setupDependencies(instance, logger)

	// Credentials
	if err := global.AddCredentialDependencies(logger, instance.GetObjectMeta(), &instance.Spec, resourceManagers); err != nil {
		return err
	}
	// DCA token
	if err := global.AddDCATokenDependencies(logger, instance.GetObjectMeta(), &instance.Spec, &instance.Status, resourceManagers); err != nil {
		return err
	}
	ensureAutoGeneratedTokenInStatus(instance, newDDAStatus, resourceManagers, logger)

	// APM Telemetry
	// ConfigMap stores DatadogAgent resource UID and creation time whether or not DatadogAgentInternal is enabled.
	if err := global.AddAPMTelemetryDependencies(logger, instance, resourceManagers); err != nil {
		return err
	}

	// DCA service
	service := clusteragent.GetClusterAgentService(instance)
	if err := resourceManagers.ServiceManager().AddService(service.Name, service.Namespace, service.Spec.Selector, service.Spec.Ports, service.Spec.InternalTrafficPolicy); err != nil {
		return err
	}

	// Apply dependencies
	if err := depsStore.Apply(ctx, r.client); err != nil {
		return errors.NewAggregate(err)
	}
	// TODO: clean up dependencies

	return nil
}

func ensureAutoGeneratedTokenInStatus(instance *v2alpha1.DatadogAgent, newStatus *v2alpha1.DatadogAgentStatus, resourceManagers feature.ResourceManagers, logger logr.Logger) {
	if instance.Status.ClusterAgent != nil && instance.Status.ClusterAgent.GeneratedToken != "" {
		// Already there; nothing to do.
		return
	}

	tokenSecret, exists := resourceManagers.Store().Get(
		kubernetes.SecretsKind, instance.Namespace, secrets.GetDefaultDCATokenSecretName(instance),
	)
	if !exists {
		logger.V(1).Info("expected autogenerated token was not created by global dependencies")
		return
	}

	generatedToken := tokenSecret.(*corev1.Secret).Data[common.DefaultTokenKey]
	if newStatus == nil {
		newStatus = &v2alpha1.DatadogAgentStatus{}
	}
	if newStatus.ClusterAgent == nil {
		newStatus.ClusterAgent = &v2alpha1.DeploymentStatus{}
	}
	// Persist generated token for subsequent reconcile loops
	newStatus.ClusterAgent.GeneratedToken = string(generatedToken)
}
