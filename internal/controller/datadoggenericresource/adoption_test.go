// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

const adoptedID = "existing-id"

func newAdoptionTestReconciler(t *testing.T) (*Reconciler, *record.FakeRecorder) {
	t.Helper()
	resetMockHandlerState()
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{}, &datadoghqv1alpha1.DatadogGenericResourceList{})
	recorder := record.NewFakeRecorder(100)
	r := NewReconciler(
		fake.NewClientBuilder().WithScheme(s).
			WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).
			WithIndex(&datadoghqv1alpha1.DatadogGenericResource{}, IDIndexField, IDIndexValues).
			Build(),
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logf.Log.WithName(t.Name()),
		recorder,
	)
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{}
	for _, rt := range []datadoghqv1alpha1.SupportedResourcesType{mockSubresource, datadoghqv1alpha1.Monitor, datadoghqv1alpha1.SoftwareCatalogEntity} {
		r.handlers[rt] = &MockHandler{}
	}
	return r, recorder
}

func adoptingResource(name, id string) *datadoghqv1alpha1.DatadogGenericResource {
	obj := mockGenericResource()
	obj.Name = name
	obj.Spec.ID = id
	return obj
}

func reconcileNamed(t *testing.T, r *Reconciler, name string) ctrl.Result {
	t.Helper()
	result, err := reconcileRequest(r, context.TODO(), newRequest(resourcesNamespace, name))
	require.NoError(t, err)
	return result
}

func getNamed(t *testing.T, r *Reconciler, name string) *datadoghqv1alpha1.DatadogGenericResource {
	t.Helper()
	obj := &datadoghqv1alpha1.DatadogGenericResource{}
	require.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: name}, obj))
	return obj
}

func errorReason(obj *datadoghqv1alpha1.DatadogGenericResource) string {
	if c := meta.FindStatusCondition(obj.Status.Conditions, string(condition.DatadogConditionTypeError)); c != nil {
		return c.Reason
	}
	return ""
}

// remoteCalls counts the API calls that read or write the resource, leaving
// out live state refreshes.
func remoteCalls() int {
	return mockCreateCalls + mockGetCalls + mockUpdateCalls + mockDeleteCalls
}

func apiErr(code int) error {
	return ctrutils.NewAPIError(fmt.Errorf("%d %s", code, http.StatusText(code)), &http.Response{StatusCode: code})
}

func updateSpec(t *testing.T, r *Reconciler, name string, edit func(*datadoghqv1alpha1.DatadogGenericResource)) {
	t.Helper()
	obj := getNamed(t, r, name)
	edit(obj)
	require.NoError(t, r.client.Update(context.TODO(), obj))
}

// makeForceSyncDue makes the next reconcile run a force sync.
func makeForceSyncDue(t *testing.T, r *Reconciler, name string) {
	t.Helper()
	obj := getNamed(t, r, name)
	stale := metav1.NewTime(time.Now().Add(-2 * defaultForceSyncPeriod))
	obj.Status.LastForceSyncTime = &stale
	require.NoError(t, r.client.Status().Update(context.TODO(), obj))
}

// adopted creates a resource with spec.id and runs the reconciles that adopt
// it and write jsonSpec to it.
func adopted(t *testing.T, r *Reconciler, name string) {
	t.Helper()
	require.NoError(t, r.client.Create(context.TODO(), adoptingResource(name, adoptedID)))
	reconcileNamed(t, r, name) // finalizer
	reconcileNamed(t, r, name) // adopt
	reconcileNamed(t, r, name) // update
	obj := getNamed(t, r, name)
	require.Equal(t, adoptedID, obj.Status.Id)
	require.NotEmpty(t, obj.Status.CurrentHash)
}

func TestAdoption_AdoptsAndWritesJSONSpec(t *testing.T) {
	r, recorder := newAdoptionTestReconciler(t)
	name := "adopt"
	require.NoError(t, r.client.Create(context.TODO(), adoptingResource(name, adoptedID)))
	reconcileNamed(t, r, name) // finalizer
	drainEvents(recorder)

	result := reconcileNamed(t, r, name)
	assert.True(t, result.Requeue, "the update follows right away")
	obj := getNamed(t, r, name)
	assert.Equal(t, adoptedID, obj.Status.Id)
	assert.Empty(t, obj.Status.CurrentHash, "jsonSpec is not written yet")
	assert.True(t, meta.IsStatusConditionTrue(obj.Status.Conditions, string(condition.DatadogConditionTypeAdopted)))
	assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusOK, obj.Status.SyncStatus)
	assert.Nil(t, obj.Status.Created, "an adopted resource has no creation time")
	assert.Equal(t, []string{adoptedID}, mockGetIDs, "the GET uses spec.id")
	assert.Equal(t, 0, mockCreateCalls)
	assert.Equal(t, 0, mockUpdateCalls)
	events := drainEvents(recorder)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], adoptedEventReason)

	// The next reconcile writes jsonSpec to the adopted resource, once.
	reconcileNamed(t, r, name)
	obj = getNamed(t, r, name)
	assert.Equal(t, 1, mockUpdateCalls)
	assert.Equal(t, 0, mockCreateCalls)
	wantHash, err := specHash(&obj.Spec)
	require.NoError(t, err)
	assert.Equal(t, wantHash, obj.Status.CurrentHash)
	assert.True(t, meta.IsStatusConditionTrue(obj.Status.Conditions, string(condition.DatadogConditionTypeUpdated)))

	reconcileNamed(t, r, name)
	assert.Equal(t, 1, mockUpdateCalls, "no further update while nothing changed")
}

func TestAdoption_GetErrors(t *testing.T) {
	for _, tt := range []struct {
		name        string
		getErr      error
		wantReason  string
		wantRequeue time.Duration
	}{
		{name: "not found", getErr: apiErr(http.StatusNotFound), wantReason: reasonAdoptionTargetNotFound, wantRequeue: defaultForceSyncPeriod},
		{name: "not found without status code", getErr: fmt.Errorf("error getting resource: %s", ctrutils.NotFoundString), wantReason: reasonAdoptionTargetNotFound, wantRequeue: defaultErrRequeuePeriod},
		{name: "forbidden", getErr: apiErr(http.StatusForbidden), wantReason: reasonAdoptionForbidden, wantRequeue: defaultForceSyncPeriod},
		{name: "unauthorized", getErr: apiErr(http.StatusUnauthorized), wantReason: reasonAdoptionForbidden, wantRequeue: defaultForceSyncPeriod},
		{name: "server error is transient", getErr: apiErr(http.StatusInternalServerError), wantReason: reasonAdoptionGetError, wantRequeue: defaultErrRequeuePeriod},
		{name: "rate limit is transient", getErr: apiErr(http.StatusTooManyRequests), wantReason: reasonAdoptionGetError, wantRequeue: defaultErrRequeuePeriod},
		{name: "network error is transient", getErr: ctrutils.NewAPIError(fmt.Errorf("connection refused"), nil), wantReason: reasonAdoptionGetError, wantRequeue: defaultErrRequeuePeriod},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newAdoptionTestReconciler(t)
			name := "adopt"
			require.NoError(t, r.client.Create(context.TODO(), adoptingResource(name, adoptedID)))
			reconcileNamed(t, r, name)
			mockGetErr = tt.getErr

			result := reconcileNamed(t, r, name)
			assert.Equal(t, tt.wantRequeue, result.RequeueAfter)
			obj := getNamed(t, r, name)
			assert.Empty(t, obj.Status.Id)
			assert.Equal(t, tt.wantReason, errorReason(obj))
			assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusGetError, obj.Status.SyncStatus)
			assert.Equal(t, 0, mockCreateCalls, "never creates a replacement")
			assert.Equal(t, 0, mockUpdateCalls)

			// Once the resource is reachable, the adoption goes ahead.
			mockGetErr = nil
			reconcileNamed(t, r, name)
			obj = getNamed(t, r, name)
			assert.Equal(t, adoptedID, obj.Status.Id)
			assert.Empty(t, errorReason(obj))
		})
	}
}

func TestAdoption_InvalidSpecID(t *testing.T) {
	for _, tt := range []struct {
		name       string
		typ        datadoghqv1alpha1.SupportedResourcesType
		id         string
		wantReason string
		fixedID    string
	}{
		{name: "monitor ID is not an integer", typ: datadoghqv1alpha1.Monitor, id: "abc", wantReason: reasonAdoptionInvalidID, fixedID: "123"},
		{name: "ID with a path separator", typ: mockSubresource, id: "123/../456", wantReason: reasonAdoptionInvalidID, fixedID: "456"},
		{name: "software catalog entity", typ: datadoghqv1alpha1.SoftwareCatalogEntity, id: "abc", wantReason: reasonAdoptionUnsupportedType},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newAdoptionTestReconciler(t)
			name := "adopt"
			obj := adoptingResource(name, tt.id)
			obj.Spec.Type = tt.typ
			require.NoError(t, r.client.Create(context.TODO(), obj))
			reconcileNamed(t, r, name)

			result := reconcileNamed(t, r, name)
			assert.Equal(t, defaultForceSyncPeriod, result.RequeueAfter)
			obj = getNamed(t, r, name)
			assert.Equal(t, tt.wantReason, errorReason(obj))
			assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusValidateError, obj.Status.SyncStatus)
			assert.Equal(t, 0, mockCalls(), "no Datadog API call")

			if tt.fixedID == "" {
				return
			}
			// Fixing spec.id clears the error and adopts.
			updateSpec(t, r, name, func(o *datadoghqv1alpha1.DatadogGenericResource) { o.Spec.ID = tt.fixedID })
			reconcileNamed(t, r, name)
			obj = getNamed(t, r, name)
			assert.Equal(t, tt.fixedID, obj.Status.Id)
			assert.Empty(t, errorReason(obj))
			assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusOK, obj.Status.SyncStatus)
		})
	}
}

func TestValidateSpecID(t *testing.T) {
	for _, tt := range []struct {
		typ   datadoghqv1alpha1.SupportedResourcesType
		id    string
		valid bool
	}{
		{datadoghqv1alpha1.Monitor, "123456789", true},
		{datadoghqv1alpha1.Monitor, "0", false},
		{datadoghqv1alpha1.Monitor, "-1", false},
		{datadoghqv1alpha1.Monitor, "12a", false},
		{datadoghqv1alpha1.Notebook, "42", true},
		{datadoghqv1alpha1.Dashboard, "abc-def-ghi", true},
		{datadoghqv1alpha1.Dashboard, "ABC-def-ghi", false},
		{datadoghqv1alpha1.Dashboard, "123", false},
		{datadoghqv1alpha1.SyntheticsAPITest, "a1b-2c3-d4e", true},
		{datadoghqv1alpha1.SyntheticsBrowserTest, "a1b-2c3", false},
		{datadoghqv1alpha1.Downtime, "00000000-0000-1234-0000-000000000000", true},
		{datadoghqv1alpha1.Downtime, "123", false},
		{datadoghqv1alpha1.MonitorNotificationRule, "00000000-0000-1234-0000-00000000000g", false},
		{datadoghqv1alpha1.SLO, "c0a1b2c3d4e5f60718293a4b5c6d7e8f", true},
		{datadoghqv1alpha1.SLO, "has space", false},
		{datadoghqv1alpha1.SLOCorrection, "00000000-0000-1234-0000-000000000000", true},
		{datadoghqv1alpha1.SoftwareCatalogEntity, "anything", false},
	} {
		t.Run(fmt.Sprintf("%s %s", tt.typ, tt.id), func(t *testing.T) {
			_, err := validateSpecID(tt.typ, tt.id)
			assert.Equal(t, tt.valid, err == nil, "error: %v", err)
		})
	}
}

func TestAdoption_SpecIDChangedAfterAdoption(t *testing.T) {
	r, _ := newAdoptionTestReconciler(t)
	name := "adopt"
	adopted(t, r, name)
	callsBefore := remoteCalls()

	updateSpec(t, r, name, func(o *datadoghqv1alpha1.DatadogGenericResource) { o.Spec.ID = "other-id" })
	makeForceSyncDue(t, r, name)
	result := reconcileNamed(t, r, name)
	assert.Equal(t, defaultForceSyncPeriod, result.RequeueAfter)
	obj := getNamed(t, r, name)
	assert.Equal(t, reasonIDMismatch, errorReason(obj))
	assert.Equal(t, adoptedID, obj.Status.Id, "status.id is kept")
	assert.Equal(t, callsBefore, remoteCalls(), "no remote action, not even a force sync")

	// Reverting spec.id clears the error without calling the API: the spec
	// hash doesn't include spec.id.
	updateSpec(t, r, name, func(o *datadoghqv1alpha1.DatadogGenericResource) { o.Spec.ID = adoptedID })
	obj = getNamed(t, r, name)
	obj.Status.LastForceSyncTime = ptrNow()
	require.NoError(t, r.client.Status().Update(context.TODO(), obj))
	reconcileNamed(t, r, name)
	obj = getNamed(t, r, name)
	assert.Empty(t, errorReason(obj))
	assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusOK, obj.Status.SyncStatus)
	assert.Equal(t, callsBefore, remoteCalls())
}

func ptrNow() *metav1.Time {
	now := metav1.Now()
	return &now
}

func TestAdoption_AdoptedResourceDeletedOutOfBand(t *testing.T) {
	t.Run("force sync", func(t *testing.T) {
		r, _ := newAdoptionTestReconciler(t)
		name := "adopt"
		adopted(t, r, name)

		mockGetErr = apiErr(http.StatusNotFound)
		makeForceSyncDue(t, r, name)
		result := reconcileNamed(t, r, name)
		assert.Equal(t, defaultForceSyncPeriod, result.RequeueAfter)
		obj := getNamed(t, r, name)
		assert.Equal(t, reasonAdoptedResourceNotFound, errorReason(obj))
		assert.Equal(t, adoptedID, obj.Status.Id)
		assert.Equal(t, 0, mockCreateCalls, "never recreated while spec.id is set")
	})
	t.Run("update", func(t *testing.T) {
		r, _ := newAdoptionTestReconciler(t)
		name := "adopt"
		adopted(t, r, name)

		mockUpdateErr = apiErr(http.StatusNotFound)
		updateSpec(t, r, name, func(o *datadoghqv1alpha1.DatadogGenericResource) { o.Spec.JsonSpec = `{"foo": "changed"}` })
		result := reconcileNamed(t, r, name)
		assert.Equal(t, defaultForceSyncPeriod, result.RequeueAfter)
		obj := getNamed(t, r, name)
		assert.Equal(t, reasonAdoptedResourceNotFound, errorReason(obj))
		assert.Equal(t, adoptedID, obj.Status.Id)
		assert.Equal(t, 0, mockCreateCalls, "never recreated while spec.id is set")
	})
	t.Run("spec.id removed", func(t *testing.T) {
		r, _ := newAdoptionTestReconciler(t)
		name := "adopt"
		adopted(t, r, name)

		// Removing spec.id keeps managing the same object, with no update.
		updateSpec(t, r, name, func(o *datadoghqv1alpha1.DatadogGenericResource) { o.Spec.ID = "" })
		updatesBefore := mockUpdateCalls
		reconcileNamed(t, r, name)
		assert.Equal(t, updatesBefore, mockUpdateCalls)
		assert.Equal(t, adoptedID, getNamed(t, r, name).Status.Id)

		// It gives up the no-replacement guarantee: a 404 recreates the object.
		mockGetErr = apiErr(http.StatusNotFound)
		makeForceSyncDue(t, r, name)
		reconcileNamed(t, r, name)
		assert.Equal(t, 1, mockCreateCalls)
		assert.Equal(t, mockResourceID, getNamed(t, r, name).Status.Id)
	})
}

func TestAdoption_SpecIDAddedToManagedResource(t *testing.T) {
	newManaged := func(t *testing.T) (*Reconciler, string) {
		r, _ := newAdoptionTestReconciler(t)
		name := "managed"
		require.NoError(t, r.client.Create(context.TODO(), adoptingResource(name, "")))
		reconcileNamed(t, r, name)
		reconcileNamed(t, r, name)
		require.Equal(t, 1, mockCreateCalls)
		require.Equal(t, mockResourceID, getNamed(t, r, name).Status.Id)
		return r, name
	}
	t.Run("equal to status.id", func(t *testing.T) {
		r, name := newManaged(t)
		callsBefore := remoteCalls()
		updateSpec(t, r, name, func(o *datadoghqv1alpha1.DatadogGenericResource) { o.Spec.ID = mockResourceID })
		reconcileNamed(t, r, name)
		obj := getNamed(t, r, name)
		assert.Empty(t, errorReason(obj))
		assert.Equal(t, callsBefore, remoteCalls(), "writing the ID back into the manifest is a no-op")
	})
	t.Run("different from status.id", func(t *testing.T) {
		r, name := newManaged(t)
		callsBefore := remoteCalls()
		updateSpec(t, r, name, func(o *datadoghqv1alpha1.DatadogGenericResource) { o.Spec.ID = "other-id" })
		reconcileNamed(t, r, name)
		obj := getNamed(t, r, name)
		assert.Equal(t, reasonIDMismatch, errorReason(obj))
		assert.Equal(t, mockResourceID, obj.Status.Id)
		assert.Equal(t, callsBefore, remoteCalls())
	})
}

func TestSpecHash_UnchangedWithoutSpecID(t *testing.T) {
	spec := mockGenericResource().Spec
	// The hash existing resources have in status.currentHash.
	before, err := comparison.GenerateMD5ForSpec(&spec)
	require.NoError(t, err)
	after, err := specHash(&spec)
	require.NoError(t, err)
	assert.Equal(t, before, after, "upgrading doesn't trigger updates")

	spec.ID = "123"
	withID, err := specHash(&spec)
	require.NoError(t, err)
	assert.Equal(t, before, withID, "spec.id is not part of the hash")
	assert.Equal(t, "123", spec.ID, "the spec is not modified")
}

func TestAdoption_StaleCache(t *testing.T) {
	r, _ := newAdoptionTestReconciler(t)
	name := "adopt"
	require.NoError(t, r.client.Create(context.TODO(), adoptingResource(name, adoptedID)))
	reconcileNamed(t, r, name)
	reconcileNamed(t, r, name)
	require.Equal(t, adoptedID, getNamed(t, r, name).Status.Id)
	getsBefore := mockGetCalls

	// A reconcile working from a cache entry older than the adoption.
	stale := getNamed(t, r, name)
	stale.Status = datadoghqv1alpha1.DatadogGenericResourceStatus{}
	result, err := r.Reconcile(context.TODO(), stale)
	require.NoError(t, err)
	assert.Equal(t, defaultErrRequeuePeriod, result.RequeueAfter)
	assert.Equal(t, getsBefore, mockGetCalls, "no second adoption")
	assert.Equal(t, 0, mockUpdateCalls)
	assert.Equal(t, adoptedID, getNamed(t, r, name).Status.Id)
}

func TestAdoption_Conflicts(t *testing.T) {
	t.Run("another resource already manages the ID", func(t *testing.T) {
		r, _ := newAdoptionTestReconciler(t)
		adopted(t, r, "first")

		require.NoError(t, r.client.Create(context.TODO(), adoptingResource("second", adoptedID)))
		reconcileNamed(t, r, "second")
		getsBefore := mockGetCalls
		result := reconcileNamed(t, r, "second")
		assert.Equal(t, defaultForceSyncPeriod, result.RequeueAfter)
		obj := getNamed(t, r, "second")
		assert.Equal(t, reasonAdoptionConflict, errorReason(obj))
		assert.Contains(t, meta.FindStatusCondition(obj.Status.Conditions, string(condition.DatadogConditionTypeError)).Message, "first already manages it")
		assert.Empty(t, obj.Status.Id)
		assert.Equal(t, getsBefore, mockGetCalls, "no remote action")
	})
	t.Run("a resource created without spec.id manages the ID", func(t *testing.T) {
		r, _ := newAdoptionTestReconciler(t)
		require.NoError(t, r.client.Create(context.TODO(), adoptingResource("first", "")))
		reconcileNamed(t, r, "first")
		reconcileNamed(t, r, "first")

		require.NoError(t, r.client.Create(context.TODO(), adoptingResource("second", mockResourceID)))
		reconcileNamed(t, r, "second")
		reconcileNamed(t, r, "second")
		assert.Equal(t, reasonAdoptionConflict, errorReason(getNamed(t, r, "second")))
	})
	t.Run("two pending resources: the oldest adopts", func(t *testing.T) {
		r, _ := newAdoptionTestReconciler(t)
		older := adoptingResource("zzz-older", adoptedID)
		older.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
		newer := adoptingResource("aaa-newer", adoptedID)
		newer.CreationTimestamp = metav1.NewTime(time.Now().Truncate(time.Second))
		require.NoError(t, r.client.Create(context.TODO(), older))
		require.NoError(t, r.client.Create(context.TODO(), newer))
		for _, name := range []string{"zzz-older", "aaa-newer"} {
			reconcileNamed(t, r, name) // finalizers
		}

		// The newer one reconciles first but yields.
		reconcileNamed(t, r, "aaa-newer")
		obj := getNamed(t, r, "aaa-newer")
		assert.Equal(t, reasonAdoptionConflict, errorReason(obj))
		assert.Contains(t, meta.FindStatusCondition(obj.Status.Conditions, string(condition.DatadogConditionTypeError)).Message, "takes precedence")

		reconcileNamed(t, r, "zzz-older")
		assert.Equal(t, adoptedID, getNamed(t, r, "zzz-older").Status.Id)
	})
	t.Run("same ID for another type is not a conflict", func(t *testing.T) {
		r, _ := newAdoptionTestReconciler(t)
		adopted(t, r, "first")

		other := adoptingResource("second", "123")
		other.Spec.Type = datadoghqv1alpha1.Monitor
		require.NoError(t, r.client.Create(context.TODO(), other))
		updateSpec(t, r, "first", func(o *datadoghqv1alpha1.DatadogGenericResource) {})
		reconcileNamed(t, r, "second")
		reconcileNamed(t, r, "second")
		assert.Equal(t, "123", getNamed(t, r, "second").Status.Id)
	})
}

func TestIDIndexValues(t *testing.T) {
	obj := adoptingResource("x", "spec-id")
	assert.Equal(t, []string{"mock_resource/spec-id"}, IDIndexValues(obj))
	obj.Status.Id = "spec-id"
	assert.Equal(t, []string{"mock_resource/spec-id"}, IDIndexValues(obj))
	obj.Status.Id = "status-id"
	assert.Equal(t, []string{"mock_resource/spec-id", "mock_resource/status-id"}, IDIndexValues(obj))
	obj.Spec.ID = ""
	assert.Equal(t, []string{"mock_resource/status-id"}, IDIndexValues(obj))
	assert.True(t, strings.HasPrefix(IDIndexValues(obj)[0], string(mockSubresource)))
}

func TestSyntheticsGetResourceToAdopt_ChecksType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"public_id":"abc-def-ghi","type":"browser","name":"t"}`))
	}))
	defer server.Close()
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = server.Client()
	client := datadogV1.NewSyntheticsApi(datadogapi.NewAPIClient(cfg))
	auth := setupTestAuth(server.URL)
	instance := &datadoghqv1alpha1.DatadogGenericResource{Status: datadoghqv1alpha1.DatadogGenericResourceStatus{Id: "abc-def-ghi"}}

	apiHandler := &SyntheticsAPITestHandler{client: client}
	err := apiHandler.getResourceToAdopt(auth, instance)
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, apiStatusCode(err))
	assert.Contains(t, err.Error(), "is a browser test")
	assert.NoError(t, apiHandler.getResource(auth, instance), "force syncs don't check the type")

	browserHandler := &SyntheticsBrowserTestHandler{client: client}
	assert.NoError(t, browserHandler.getResourceToAdopt(auth, instance))
}
