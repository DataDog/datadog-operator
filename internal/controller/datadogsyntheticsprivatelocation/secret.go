// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

// apiManagedConfigKeys are minted by Datadog when the private location is
// created and can never be recovered from the API afterwards; they are the
// write-once part of the worker configuration.
var apiManagedConfigKeys = map[string]struct{}{
	"accessKey":       {},
	"secretAccessKey": {},
	"publicKey":       {},
	"privateKey":      {},
	"id":              {},
}

// operatorManagedConfigKeys must match the worker Deployment probes, which
// the status probes annotation controls.
var operatorManagedConfigKeys = map[string]struct{}{
	"enableStatusProbes": {},
	"statusProbesPort":   {},
}

// parseConfigOverride parses the worker config override annotation and
// rejects Datadog-managed and operator-managed keys.
func parseConfigOverride(overrideJSON string) (map[string]any, error) {
	if overrideJSON == "" {
		return nil, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(overrideJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid %s annotation: %w", datadoghqv1alpha1.DatadogSPLWorkerConfigOverrideAnnotation, err)
	}
	for k := range raw {
		if _, managed := apiManagedConfigKeys[k]; managed {
			return nil, fmt.Errorf("invalid %s annotation: key %q is Datadog-managed and cannot be overridden", datadoghqv1alpha1.DatadogSPLWorkerConfigOverrideAnnotation, k)
		}
		if _, managed := operatorManagedConfigKeys[k]; managed {
			return nil, fmt.Errorf("invalid %s annotation: key %q is managed by the operator, use the %s annotation", datadoghqv1alpha1.DatadogSPLWorkerConfigOverrideAnnotation, k, datadoghqv1alpha1.DatadogSPLStatusProbesEnabledAnnotation)
		}
	}
	return raw, nil
}

// mergeWorkerConfig merges the Datadog-provided config skeleton with user
// overrides: base (the creation response config or the existing Secret data)
// is merged with spec.worker.config, then the worker config override
// annotation (last wins). The operator-managed site is always forced from the
// credentials.
func mergeWorkerConfig(baseJSON []byte, config *datadoghqv1alpha1.DatadogSPLWorkerConfig, overrideJSON, site string) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(baseJSON, &cfg); err != nil {
		return nil, ctrutils.TranslateUnmarshalError(fmt.Errorf("invalid config returned by Datadog API: %w", err), "invalid config")
	}

	if config != nil && config.Concurrency != nil {
		cfg["concurrency"] = *config.Concurrency
	}

	raw, err := parseConfigOverride(overrideJSON)
	if err != nil {
		return nil, ctrutils.TranslateUnmarshalError(err, "invalid worker config override")
	}
	maps.Copy(cfg, raw)

	cfg["site"] = site

	return json.Marshal(cfg)
}

func buildConfigSecret(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, configJSON []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configSecretName(instance),
			Namespace: instance.Namespace,
			Labels:    splLabels(instance),
		},
		Data: map[string][]byte{
			datadoghqv1alpha1.DatadogSPLConfigSecretDataKey: configJSON,
		},
	}
}

// reconcileConfigSecret creates the config Secret on initial creation (the
// config skeleton can only be obtained from the creation response) or merges
// user overrides over the existing Secret data on updates. It returns the
// Secret's data key content that the worker should consume.
func reconcileConfigSecret(ctx context.Context, kubeClient client.Client, scheme *runtime.Scheme, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, newBaseConfig []byte, site string) error {
	logger := ctrl.LoggerFrom(ctx)

	var baseConfig []byte
	current := &corev1.Secret{}
	secretExists := false
	err := kubeClient.Get(ctx, client.ObjectKey{Name: configSecretName(instance), Namespace: instance.Namespace}, current)
	switch {
	case err == nil:
		secretExists = true
		baseConfig = current.Data[datadoghqv1alpha1.DatadogSPLConfigSecretDataKey]
	case apierrors.IsNotFound(err):
		baseConfig = newBaseConfig
	default:
		return fmt.Errorf("getting config secret: %w", err)
	}
	if len(baseConfig) == 0 {
		// The write-once config is lost: the Datadog API never returns it
		// again, so it cannot be rebuilt. Surface the error instead of
		// writing an unusable config.
		return fmt.Errorf("config secret %s has no %s data: the Datadog-provided config cannot be recovered, recreate the DatadogSyntheticsPrivateLocation", configSecretName(instance), datadoghqv1alpha1.DatadogSPLConfigSecretDataKey)
	}

	var config *datadoghqv1alpha1.DatadogSPLWorkerConfig
	if instance.Spec.Worker != nil {
		config = instance.Spec.Worker.Config
	}
	overrideJSON := instance.GetAnnotations()[datadoghqv1alpha1.DatadogSPLWorkerConfigOverrideAnnotation]
	merged, err := mergeWorkerConfig(baseConfig, config, overrideJSON, site)
	if err != nil {
		return err
	}

	desired := buildConfigSecret(instance, merged)
	if refErr := controllerutil.SetControllerReference(instance, desired, scheme); refErr != nil {
		return fmt.Errorf("setting owner reference on config secret: %w", refErr)
	}

	if !secretExists {
		logger.Info("Creating worker config secret", "secret", desired.Name)
		return kubeClient.Create(ctx, desired)
	}

	if !apiequality.Semantic.DeepEqual(current.Data, desired.Data) ||
		!apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) {
		logger.Info("Updating worker config secret", "secret", desired.Name)
		current.Data = desired.Data
		current.Labels = desired.Labels
		return kubeClient.Update(ctx, current)
	}

	return nil
}
