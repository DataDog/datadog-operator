// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/secrets/resolver"
)

const noSecretBackendMsg = "spec.jsonSpec contains ENC[] secret references but no secret backend is configured; sending them as is"

// SecretResolver resolves ENC[] secret references in a resource's jsonSpec.
type SecretResolver interface {
	ResolveJSON(jsonSpec, origin, namespace string) (resolver.Result, error)
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
		return nil, nil, err
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

// redactedError replaces err's message, keeping err in the chain for classification.
type redactedError struct {
	err error
	msg string
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// hideResponseBody omits untrusted SDK details while preserving error classification.
func hideResponseBody(err error) error {
	bodyErr, ok := errors.AsType[*responseBodyError](err)
	if !ok {
		return err
	}
	msg := bodyErr.msg
	if apiErr, ok := errors.AsType[*ctrutils.APIError](err); ok && apiErr.StatusCode > 0 {
		msg += fmt.Sprintf(": %d %s", apiErr.StatusCode, http.StatusText(apiErr.StatusCode))
	}
	msg += " (response omitted because the spec contains secret references)"
	return &redactedError{err: err, msg: strings.Replace(err.Error(), bodyErr.Error(), msg, 1)}
}

// hideDecodeErrorBody hides a read's SDK decode error (2xx), whose body is the stored resource.
// Read requests carry no payload, so other error bodies can't echo resolved values.
func hideDecodeErrorBody(err error) error {
	if apiErr, ok := errors.AsType[*ctrutils.APIError](err); ok && apiErr.StatusCode >= 200 && apiErr.StatusCode < 300 {
		return hideResponseBody(err)
	}
	return err
}

// hideSecrets removes the API response body and resolved values from err's message.
func hideSecrets(err error, values []string) error {
	if err == nil || len(values) == 0 {
		return err
	}
	err = hideResponseBody(err)
	var forms []string
	for _, v := range values {
		forms = append(forms, secretForms(v)...)
	}
	slices.SortFunc(forms, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	var replacements []string
	for _, form := range forms {
		if form != "" {
			replacements = append(replacements, form, "[redacted]")
		}
	}
	msg := strings.NewReplacer(replacements...).Replace(err.Error())
	if msg == err.Error() {
		return err
	}
	return &redactedError{err: err, msg: msg}
}

// secretForms returns how v can appear in a message: as is, %q-escaped and JSON-escaped.
func secretForms(v string) []string {
	quoted := strconv.Quote(v)
	escaped := []string{quoted[1 : len(quoted)-1]}
	if jsonQuoted, err := json.Marshal(v); err == nil {
		escaped = append(escaped, string(jsonQuoted[1:len(jsonQuoted)-1]))
	}
	forms := []string{v}
	for _, f := range escaped {
		if !slices.Contains(forms, f) {
			forms = append(forms, f)
		}
	}
	return forms
}
