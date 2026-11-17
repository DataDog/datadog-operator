// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/secrets/resolver"
)

const noSecretBackendMsg = "spec.jsonSpec contains ENC[] secret references but no secret backend is configured; sending them as is"

// SecretResolver resolves ENC[] secret references in a resource's jsonSpec.
type SecretResolver interface {
	ResolveJSON(jsonSpec, origin, namespace string) (resolver.Result, error)
}

// secretResolutionError marks a failure to resolve references, before any API call.
type secretResolutionError struct{ err error }

func (e *secretResolutionError) Error() string { return e.err.Error() }
func (e *secretResolutionError) Unwrap() error { return e.err }

func isSecretResolutionError(err error) bool {
	_, ok := errors.AsType[*secretResolutionError](err)
	return ok
}

func secretOrigin(instance *v1alpha1.DatadogGenericResource) string {
	return fmt.Sprintf("datadoggenericresource/%s/%s", instance.Namespace, instance.Name)
}

// resolveSecrets returns a copy of instance with resolved references, and the resolved values.
func (r *Reconciler) resolveSecrets(ctx context.Context, instance *v1alpha1.DatadogGenericResource) (*v1alpha1.DatadogGenericResource, []string, error) {
	if r.secretResolver == nil {
		return instance, nil, nil
	}
	res, err := r.secretResolver.ResolveJSON(instance.Spec.JsonSpec, secretOrigin(instance), instance.Namespace)
	if err != nil {
		return nil, nil, &secretResolutionError{err: err}
	}
	if res.NoBackend {
		ctrl.LoggerFrom(ctx).Info(noSecretBackendMsg)
		r.recorder.Event(instance, corev1.EventTypeWarning, "SecretBackendNotConfigured", noSecretBackendMsg)
	}
	if len(res.Values) == 0 {
		return instance, nil, nil
	}
	resolved := instance.DeepCopy()
	resolved.Spec.JsonSpec = res.JSON
	return resolved, res.Values, nil
}
