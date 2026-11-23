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
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/secrets/resolver"
)

const (
	secretRef     = "ENC[ns/client;tls.key]"
	secretValue   = "DUMMYSECRETVALUE"
	specWithRef   = `{"key": "` + secretRef + `"}`
	specResolved  = `{"key": "` + secretValue + `"}`
	unresolvedMsg = "unable to resolve secret references: ns/client;tls.key"
)

var errUnresolved = fmt.Errorf("%w: ns/client;tls.key", resolver.ErrUnresolved)

// apiError builds an error like a handler does for a Datadog API error response.
func apiError(code int, body string) error {
	return translateClientError(
		datadogapi.GenericOpenAPIError{ErrorMessage: fmt.Sprintf("%d %s", code, http.StatusText(code)), ErrorBody: []byte(body)},
		&http.Response{StatusCode: code},
		"error calling API",
	)
}

// fakeSecretResolver replaces secretRef with value, or fails with err.
type fakeSecretResolver struct {
	err       error
	noBackend bool
	value     string
	calls     int
	onResolve func()
}

func (f *fakeSecretResolver) ResolveJSON(spec string) (resolver.Result, error) {
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
	env.resolver.err = errUnresolved

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
	env.resolver.err = errUnresolved

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

func TestSecretsAPIErrorOmitsBody(t *testing.T) {
	env := newSecretsTestEnv(t, specWithRef)
	mockCreateErr = apiError(http.StatusBadRequest, `{"errors": ["invalid key `+secretValue+`"]}`)

	result := env.reconcile(t)
	assert.Equal(t, 1, mockCreateCalls)
	assert.Equal(t, env.r.forceSyncPeriod, result.RequeueAfter, "permanent errors keep their classification")
	assert.Equal(t, "error calling API: 400 Bad Request (response omitted because the spec contains secret references)", errorMessage(env.get(t)))
	env.assertNoSecretValue(t)
}

func TestSecretsErrorDetailsKept(t *testing.T) {
	for name, tc := range map[string]struct {
		spec string
		err  error
		want string
	}{
		"no references": {`{"foo": "bar"}`, apiError(http.StatusBadRequest, `{"errors": ["invalid foo"]}`), "invalid foo"},
		"local error":   {specWithRef, translateUnmarshalError(errors.New("cannot unmarshal string into field port"), "error unmarshalling API test spec"), "cannot unmarshal string into field port"},
	} {
		t.Run(name, func(t *testing.T) {
			env := newSecretsTestEnv(t, tc.spec)
			mockCreateErr = tc.err
			env.reconcile(t)
			assert.Contains(t, errorMessage(env.get(t)), tc.want)
		})
	}
}

func TestSecretsUpdateNotFoundRecreates(t *testing.T) {
	env := newSecretsTestEnv(t, specWithRef)
	env.reconcile(t)

	obj := env.get(t)
	obj.Spec.JsonSpec = `{"key": "` + secretRef + `", "other": 1}`
	require.NoError(t, env.r.client.Update(env.ctx, obj))
	env.resolver.value = ctrutils.NotFoundString
	mockUpdateErr = apiError(http.StatusNotFound, secretValue)
	calls := env.resolver.calls

	env.reconcile(t)
	assert.Equal(t, 1, mockUpdateCalls)
	assert.Equal(t, 2, mockCreateCalls, "a 404 on update must still recreate the resource")
	assert.Equal(t, calls+1, env.resolver.calls, "update and the recreate must share one resolution")
	env.assertNoSecretValue(t)
}

func TestSecretsDeleteDoesNotResolve(t *testing.T) {
	env := newSecretsTestEnv(t, specWithRef)
	env.reconcile(t)
	calls := env.resolver.calls

	env.resolver.err = errUnresolved
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

func TestSecretsLocalErrorRedacted(t *testing.T) {
	env := newSecretsTestEnv(t, specWithRef)
	env.resolver.value = "-----BEGIN KEY-----\nDUMMYKEY\n-----END KEY-----\n"
	mockCreateErr = fmt.Errorf("slo_id cannot be changed from %q to %q", "old", env.resolver.value)

	env.reconcile(t)
	assert.Equal(t, `slo_id cannot be changed from "old" to "[redacted]"`, errorMessage(env.get(t)))
	assert.NotContains(t, env.logs.String(), "DUMMYKEY")
}

func TestHideSecrets(t *testing.T) {
	assert.NoError(t, hideSecrets(nil, []string{secretValue}))
	omitted := " (response omitted because the spec contains secret references)"
	for name, tc := range map[string]struct {
		err    error
		values []string
		want   string // empty: err is returned unchanged
	}{
		"no values in message": {err: errors.New("dial tcp: connection refused"), values: []string{secretValue}},
		"no references":        {err: apiError(http.StatusBadRequest, "invalid foo")},
		"transient API error":  {apiError(http.StatusServiceUnavailable, "upstream"), []string{secretValue}, "error calling API: 503 Service Unavailable" + omitted},
		"wrapped API error":    {fmt.Errorf("step X: %w", apiError(http.StatusBadRequest, "echo "+secretValue)), []string{secretValue}, "step X: error calling API: 400 Bad Request" + omitted},
		"raw and quoted":       {fmt.Errorf("value %s and %s", secretValue, "a\"b"), []string{secretValue, "a\"b"}, "value [redacted] and [redacted]"},
		"short value first":    {fmt.Errorf("%s %q", "abc", "abcdef"), []string{"abc", "abcdef"}, `[redacted] "[redacted]"`},
		"long value first":     {fmt.Errorf("%s %q", "abcdef", "abc"), []string{"abcdef", "abc"}, `[redacted] "[redacted]"`},
		"multiline overlap":    {fmt.Errorf("%s %q", "a", "a\nb"), []string{"a", "a\nb"}, `[redacted] "[redacted]"`},
	} {
		t.Run(name, func(t *testing.T) {
			got := hideSecrets(tc.err, tc.values)
			if tc.want == "" {
				assert.Equal(t, tc.err, got)
				return
			}
			assert.Equal(t, tc.want, got.Error())
			assert.Equal(t, ctrutils.IsPermanentAPIError(tc.err), ctrutils.IsPermanentAPIError(got), "classification is kept")
		})
	}
}

func TestSecretsReadErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		refresh bool
		status  int
		hidden  bool
	}{
		"get decode error":     {status: http.StatusOK, hidden: true},
		"refresh decode error": {refresh: true, status: http.StatusOK, hidden: true},
		"get server error":     {status: http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			env := newSecretsTestEnv(t, specWithRef)
			env.reconcile(t)
			obj := env.get(t)
			stale := metav1.NewTime(time.Now().Add(-2 * defaultForceSyncPeriod))
			if tc.refresh {
				obj.Status.StateLastUpdateTime = &stale
			} else {
				obj.Status.LastForceSyncTime = &stale
			}
			require.NoError(t, env.r.client.Status().Update(env.ctx, obj))
			// A 2xx SDK decode error includes the stored resource; reads send no payload otherwise.
			body := "upstream unavailable"
			if tc.hidden {
				body = `{"value":"` + secretValue + `"}`
			}
			err := translateClientError(datadogapi.GenericOpenAPIError{ErrorMessage: "read failed", ErrorBody: []byte(body)},
				&http.Response{StatusCode: tc.status}, "error reading resource")
			mockGetErr, mockRefreshStateErr = err, err
			calls := env.resolver.calls
			env.reconcile(t)
			assert.Equal(t, calls, env.resolver.calls, "read errors must not need secret resolution")
			conditions := fmt.Sprint(env.get(t).Status.Conditions)
			if tc.hidden {
				assert.Contains(t, conditions, "response omitted")
				env.assertNoSecretValue(t)
			} else {
				assert.Contains(t, conditions, body)
			}
		})
	}
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
