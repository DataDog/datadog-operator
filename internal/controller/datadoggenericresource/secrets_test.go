// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/secrets/resolver"
)

const (
	secretRef     = "ENC[ns/client;tls.key]"
	secretValue   = "DUMMYSECRETVALUE"
	specWithRef   = `{"key": "` + secretRef + `"}`
	specResolved  = `{"key": "` + secretValue + `"}`
	unresolvedMsg = "unable to resolve secret references: ns/client;tls.key"
)

// fakeSecretResolver replaces secretRef with value, or fails with err.
type fakeSecretResolver struct {
	err       error
	noBackend bool
	value     string
	calls     int
	onResolve func()
}

func (f *fakeSecretResolver) ResolveJSON(spec, _, _ string) (resolver.Result, error) {
	f.calls++
	if f.onResolve != nil {
		f.onResolve()
	}
	if !strings.Contains(spec, "ENC[") {
		return resolver.Result{JSON: spec}, nil
	}
	if f.noBackend {
		return resolver.Result{JSON: spec, NoBackend: true}, nil
	}
	if f.err != nil {
		return resolver.Result{}, f.err
	}
	value := f.value
	if value == "" {
		value = secretValue
	}
	escaped, _ := json.Marshal(value)
	return resolver.Result{
		JSON:   strings.ReplaceAll(spec, `"`+secretRef+`"`, string(escaped)),
		Values: []string{value},
	}, nil
}

type secretsTestEnv struct {
	r        *Reconciler
	resolver *fakeSecretResolver
	recorder *record.FakeRecorder
	logs     *bytes.Buffer
	ctx      context.Context
}

func newSecretsTestEnv(t *testing.T, jsonSpec string) *secretsTestEnv {
	t.Helper()
	resetMockHandlerState()
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})
	logs := &bytes.Buffer{}
	logger := zap.New(zap.WriteTo(logs))
	env := &secretsTestEnv{
		resolver: &fakeSecretResolver{},
		recorder: record.NewFakeRecorder(100),
		logs:     logs,
		ctx:      ctrl.LoggerInto(context.Background(), logger),
	}
	env.r = NewReconciler(
		fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logger,
		env.recorder,
		ReconcilerOptions{SecretResolver: env.resolver},
	)
	env.r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
		mockSubresource: &MockHandler{},
	}

	instance := mockGenericResource()
	instance.Spec.JsonSpec = jsonSpec
	instance.Finalizers = []string{datadogGenericResourceFinalizer}
	require.NoError(t, env.r.client.Create(env.ctx, instance))
	return env
}

func (e *secretsTestEnv) reconcile(t *testing.T) ctrl.Result {
	t.Helper()
	result, err := reconcileRequest(e.r, e.ctx, newRequest(resourcesNamespace, resourcesName))
	require.NoError(t, err)
	return result
}

func (e *secretsTestEnv) get(t *testing.T) *datadoghqv1alpha1.DatadogGenericResource {
	t.Helper()
	obj := &datadoghqv1alpha1.DatadogGenericResource{}
	require.NoError(t, e.r.client.Get(e.ctx, client.ObjectKey{Namespace: resourcesNamespace, Name: resourcesName}, obj))
	return obj
}

// assertNoSecretValue checks the stored resource, events and logs.
func (e *secretsTestEnv) assertNoSecretValue(t *testing.T) {
	t.Helper()
	obj, err := json.Marshal(e.get(t))
	require.NoError(t, err)
	assert.NotContains(t, string(obj), secretValue, "resource or status")
	close(e.recorder.Events)
	for event := range e.recorder.Events {
		assert.NotContains(t, event, secretValue, "event")
	}
	assert.NotContains(t, e.logs.String(), secretValue, "logs")
}

func errorMessage(obj *datadoghqv1alpha1.DatadogGenericResource) string {
	for _, c := range obj.Status.Conditions {
		if c.Type == "Error" {
			return c.Message
		}
	}
	return ""
}

func TestSecretsCreateResolvesCopy(t *testing.T) {
	env := newSecretsTestEnv(t, specWithRef)

	env.reconcile(t)
	assert.Equal(t, 1, mockCreateCalls)
	assert.JSONEq(t, specResolved, mockLastJSONSpec)

	obj := env.get(t)
	assert.Equal(t, specWithRef, obj.Spec.JsonSpec, "the resource must keep the reference")
	assert.Equal(t, mockResourceID, obj.Status.Id)
	env.assertNoSecretValue(t)
}

func TestSecretsCreateResolutionFailure(t *testing.T) {
	env := newSecretsTestEnv(t, specWithRef)
	env.resolver.err = errors.New(unresolvedMsg)

	result := env.reconcile(t)
	assert.Zero(t, mockCreateCalls, "nothing must be created")
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)

	obj := env.get(t)
	assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusCreateError, obj.Status.SyncStatus)
	assert.Contains(t, errorMessage(obj), unresolvedMsg)
	assert.Empty(t, obj.Status.Id)

	// Once the secret resolves, the next reconcile creates the resource.
	env.resolver.err = nil
	env.reconcile(t)
	assert.Equal(t, 1, mockCreateCalls)
	assert.Equal(t, mockResourceID, env.get(t).Status.Id)
	env.assertNoSecretValue(t)
}

func TestSecretsUpdateResolutionFailure(t *testing.T) {
	env := newSecretsTestEnv(t, `{"foo": "bar"}`)
	env.reconcile(t)
	createdHash := env.get(t).Status.CurrentHash

	obj := env.get(t)
	obj.Spec.JsonSpec = specWithRef
	require.NoError(t, env.r.client.Update(env.ctx, obj))
	env.resolver.err = errors.New(unresolvedMsg)

	result := env.reconcile(t)
	assert.Zero(t, mockUpdateCalls, "the remote resource must be left unchanged")
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)
	obj = env.get(t)
	assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusUpdateError, obj.Status.SyncStatus)
	assert.Equal(t, createdHash, obj.Status.CurrentHash, "the hash must not advance, so the update is retried")

	env.resolver.err = nil
	env.reconcile(t)
	assert.Equal(t, 1, mockUpdateCalls)
	assert.JSONEq(t, specResolved, mockLastJSONSpec)
	assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusOK, env.get(t).Status.SyncStatus)
	env.assertNoSecretValue(t)
}

func TestSecretsDeleteDoesNotResolve(t *testing.T) {
	env := newSecretsTestEnv(t, specWithRef)
	env.reconcile(t)
	calls := env.resolver.calls

	env.resolver.err = errors.New(unresolvedMsg)
	require.NoError(t, env.r.client.Delete(env.ctx, env.get(t)))
	env.reconcile(t)

	assert.Equal(t, 1, mockDeleteCalls)
	assert.Equal(t, calls, env.resolver.calls, "deletion must not resolve secrets")
}

func TestSecretsNoBackendSendsAsIs(t *testing.T) {
	env := newSecretsTestEnv(t, specWithRef)
	env.resolver.noBackend = true

	env.reconcile(t)
	assert.Equal(t, 1, mockCreateCalls)
	assert.Equal(t, specWithRef, mockLastJSONSpec, "references are sent as is, as before")
	assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusOK, env.get(t).Status.SyncStatus)
	assert.Contains(t, <-env.recorder.Events, "Warning SecretBackendNotConfigured")
}

func TestSecretsFreshnessCheckedAfterResolution(t *testing.T) {
	env := newSecretsTestEnv(t, specWithRef)
	env.resolver.onResolve = func() {
		obj := env.get(t)
		obj.Status.Id = "already-created"
		require.NoError(t, env.r.client.Status().Update(env.ctx, obj))
	}
	result := env.reconcile(t)
	assert.Zero(t, mockCreateCalls)
	assert.Equal(t, defaultErrRequeuePeriod, result.RequeueAfter)
	assert.Equal(t, "already-created", env.get(t).Status.Id)
}
