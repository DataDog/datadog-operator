// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
)

// newLegacyTestReconciler returns a reconciler whose cluster has the legacy
// CRDs installed and the given objects.
func newLegacyTestReconciler(t *testing.T, funcs interceptor.Funcs, objs ...client.Object) *Reconciler {
	t.Helper()
	resetMockHandlerState()
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(s))
	c := fake.NewClientBuilder().WithScheme(s).
		WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).
		WithIndex(&datadoghqv1alpha1.DatadogGenericResource{}, IDIndexField, IDIndexValues).
		WithObjects(objs...).
		WithInterceptorFuncs(funcs).
		Build()
	r := NewReconciler(c, config.NewCredentialManager(fake.NewClientBuilder().Build()), s, logf.Log.WithName(t.Name()), record.NewFakeRecorder(100))
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{}
	for _, rt := range []datadoghqv1alpha1.SupportedResourcesType{datadoghqv1alpha1.Monitor, datadoghqv1alpha1.Dashboard, datadoghqv1alpha1.SLO, datadoghqv1alpha1.Notebook} {
		r.handlers[rt] = &MockHandler{}
	}
	return r
}

func legacyMonitor(name string, id int) *datadoghqv1alpha1.DatadogMonitor {
	return &datadoghqv1alpha1.DatadogMonitor{
		ObjectMeta: metav1.ObjectMeta{Namespace: resourcesNamespace, Name: name},
		Status:     datadoghqv1alpha1.DatadogMonitorStatus{ID: id},
	}
}

func adoptingTyped(name string, typ datadoghqv1alpha1.SupportedResourcesType, id string) *datadoghqv1alpha1.DatadogGenericResource {
	obj := adoptingResource(name, id)
	obj.Spec.Type = typ
	return obj
}

func TestAdoption_LegacyOwnerBlocks(t *testing.T) {
	for _, tt := range []struct {
		name   string
		typ    datadoghqv1alpha1.SupportedResourcesType
		id     string
		legacy client.Object
	}{
		{name: "DatadogMonitor", typ: datadoghqv1alpha1.Monitor, id: "123", legacy: legacyMonitor("legacy", 123)},
		{
			name: "DatadogDashboard", typ: datadoghqv1alpha1.Dashboard, id: "abc-def-ghi",
			legacy: &datadoghqv1alpha1.DatadogDashboard{
				ObjectMeta: metav1.ObjectMeta{Namespace: resourcesNamespace, Name: "legacy"},
				Status:     datadoghqv1alpha1.DatadogDashboardStatus{ID: "abc-def-ghi"},
			},
		},
		{
			name: "DatadogSLO", typ: datadoghqv1alpha1.SLO, id: "c0a1b2c3d4e5f60718293a4b5c6d7e8f",
			legacy: &datadoghqv1alpha1.DatadogSLO{
				ObjectMeta: metav1.ObjectMeta{Namespace: "other-namespace", Name: "legacy"},
				Status:     datadoghqv1alpha1.DatadogSLOStatus{ID: "c0a1b2c3d4e5f60718293a4b5c6d7e8f"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := newLegacyTestReconciler(t, interceptor.Funcs{}, tt.legacy)
			name := "adopt"
			require.NoError(t, r.client.Create(context.TODO(), adoptingTyped(name, tt.typ, tt.id)))
			reconcileNamed(t, r, name)

			result := reconcileNamed(t, r, name)
			assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter, "retried soon: the legacy CR is expected to go away")
			obj := getNamed(t, r, name)
			assert.Equal(t, reasonAdoptionLegacyOwner, errorReason(obj))
			msg := meta.FindStatusCondition(obj.Status.Conditions, "Error").Message
			assert.Contains(t, msg, tt.name+" "+tt.legacy.GetNamespace()+"/legacy manages it")
			assert.Contains(t, msg, datadoghqv1alpha1.ManagementPolicyAnnotationKey)
			assert.Empty(t, obj.Status.Id)
			assert.Equal(t, 0, remoteCalls(), "no remote action")

			// Once the legacy CR is gone, the next retry adopts.
			require.NoError(t, r.client.Delete(context.TODO(), tt.legacy))
			reconcileNamed(t, r, name)
			obj = getNamed(t, r, name)
			assert.Equal(t, tt.id, obj.Status.Id)
			assert.Empty(t, errorReason(obj))
		})
	}
}

func TestAdoption_LegacyOwnerTerminatingStillBlocks(t *testing.T) {
	legacy := legacyMonitor("legacy", 123)
	legacy.Finalizers = []string{"finalizer.monitor.datadoghq.com"}
	r := newLegacyTestReconciler(t, interceptor.Funcs{}, legacy)
	require.NoError(t, r.client.Delete(context.TODO(), legacy))

	name := "adopt"
	require.NoError(t, r.client.Create(context.TODO(), adoptingTyped(name, datadoghqv1alpha1.Monitor, "123")))
	reconcileNamed(t, r, name)
	reconcileNamed(t, r, name)
	obj := getNamed(t, r, name)
	assert.Equal(t, reasonAdoptionLegacyOwner, errorReason(obj))
	assert.Contains(t, meta.FindStatusCondition(obj.Status.Conditions, "Error").Message, "is being deleted")
}

func TestAdoption_LegacyOwnerIndexEntryGoesStale(t *testing.T) {
	legacy := legacyMonitor("legacy", 123)
	r := newLegacyTestReconciler(t, interceptor.Funcs{}, legacy)
	name := "adopt"
	require.NoError(t, r.client.Create(context.TODO(), adoptingTyped(name, datadoghqv1alpha1.Monitor, "123")))
	reconcileNamed(t, r, name)
	reconcileNamed(t, r, name)
	require.Equal(t, reasonAdoptionLegacyOwner, errorReason(getNamed(t, r, name)))

	// The legacy CR now manages another monitor: the indexed entry is stale.
	legacy.Status.ID = 456
	require.NoError(t, r.client.Update(context.TODO(), legacy)) // no status subresource for legacy kinds in this fake client
	reconcileNamed(t, r, name)
	assert.Equal(t, "123", getNamed(t, r, name).Status.Id)
}

func TestAdoption_LegacyOwnerOtherTypesAndIDs(t *testing.T) {
	r := newLegacyTestReconciler(t, interceptor.Funcs{}, legacyMonitor("legacy", 123))
	// Another monitor, and a notebook with the same number: no legacy owner.
	require.NoError(t, r.client.Create(context.TODO(), adoptingTyped("other-monitor", datadoghqv1alpha1.Monitor, "124")))
	require.NoError(t, r.client.Create(context.TODO(), adoptingTyped("notebook", datadoghqv1alpha1.Notebook, "123")))
	for _, name := range []string{"other-monitor", "notebook"} {
		reconcileNamed(t, r, name)
		reconcileNamed(t, r, name)
		assert.NotEmpty(t, getNamed(t, r, name).Status.Id, name)
	}
}

func TestAdoption_LegacyCheckFailsClosed(t *testing.T) {
	var forbid atomic.Bool
	forbid.Store(true)
	var lists atomic.Int32
	funcs := interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
		if _, ok := list.(*datadoghqv1alpha1.DatadogMonitorList); ok {
			lists.Add(1)
			if forbid.Load() {
				return apierrors.NewForbidden(schema.GroupResource{Group: "datadoghq.com", Resource: "datadogmonitors"}, "", fmt.Errorf("RBAC"))
			}
		}
		return c.List(ctx, list, opts...)
	}}
	r := newLegacyTestReconciler(t, funcs)
	name := "adopt"
	require.NoError(t, r.client.Create(context.TODO(), adoptingTyped(name, datadoghqv1alpha1.Monitor, "123")))
	reconcileNamed(t, r, name)

	result := reconcileNamed(t, r, name)
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)
	obj := getNamed(t, r, name)
	assert.Equal(t, reasonAdoptionLegacyCheckError, errorReason(obj))
	assert.Contains(t, meta.FindStatusCondition(obj.Status.Conditions, "Error").Message, "list permission on datadogmonitors.datadoghq.com")
	assert.Equal(t, 0, remoteCalls())

	// The list is retried on the next check, then cached.
	forbid.Store(false)
	reconcileNamed(t, r, name)
	assert.Equal(t, "123", getNamed(t, r, name).Status.Id)
	assert.Equal(t, int32(2), lists.Load())

	require.NoError(t, r.client.Create(context.TODO(), adoptingTyped("second", datadoghqv1alpha1.Monitor, "456")))
	reconcileNamed(t, r, "second")
	reconcileNamed(t, r, "second")
	assert.Equal(t, int32(2), lists.Load(), "one list per kind")
}

func TestAdoption_LegacyCRDNotInstalled(t *testing.T) {
	funcs := interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
		if _, ok := list.(*datadoghqv1alpha1.DatadogMonitorList); ok {
			return &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "datadoghq.com", Kind: "DatadogMonitor"}}
		}
		return c.List(ctx, list, opts...)
	}}
	r := newLegacyTestReconciler(t, funcs)
	name := "adopt"
	require.NoError(t, r.client.Create(context.TODO(), adoptingTyped(name, datadoghqv1alpha1.Monitor, "123")))
	reconcileNamed(t, r, name)
	reconcileNamed(t, r, name)
	assert.Equal(t, "123", getNamed(t, r, name).Status.Id)
}

// pagedReader serves DatadogMonitors in pages of two, like an API server
// honoring Limit and Continue.
type pagedReader struct {
	client.Reader
	monitors []datadoghqv1alpha1.DatadogMonitor
	calls    int
}

func (p *pagedReader) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	p.calls++
	o := &client.ListOptions{}
	o.ApplyOptions(opts)
	start := 0
	if o.Continue != "" {
		start, _ = strconv.Atoi(o.Continue)
	}
	end := min(start+2, len(p.monitors))
	out := list.(*datadoghqv1alpha1.DatadogMonitorList)
	out.Items = p.monitors[start:end]
	if end < len(p.monitors) {
		out.Continue = strconv.Itoa(end)
	}
	return nil
}

func TestLegacyOwnerIndex_Paginates(t *testing.T) {
	reader := &pagedReader{}
	for i := 1; i <= 5; i++ {
		reader.monitors = append(reader.monitors, *legacyMonitor(fmt.Sprintf("m%d", i), 100+i))
	}
	reader.monitors = append(reader.monitors, *legacyMonitor("no-id", 0))
	x := newLegacyOwnerIndex(reader)
	lk := legacyKinds[datadoghqv1alpha1.Monitor]

	idx, err := x.build(context.TODO(), datadoghqv1alpha1.Monitor, lk)
	require.NoError(t, err)
	assert.Equal(t, 3, reader.calls)
	assert.Len(t, idx.owners, 5, "CRs without an ID are left out")
	assert.Equal(t, "m5", idx.owners["105"].Name)
}
