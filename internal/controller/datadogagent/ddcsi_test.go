// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func newTestReconcilerForDDCSI(scheme *runtime.Scheme, platformInfo kubernetes.PlatformInfo) *Reconciler {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	return &Reconciler{
		client:       fakeClient,
		scheme:       scheme,
		log:          logf.Log.WithName("test"),
		recorder:     record.NewFakeRecorder(100),
		platformInfo: platformInfo,
	}
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = v2alpha1.AddToScheme(s)
	return s
}

func platformInfoWithDDCSI() kubernetes.PlatformInfo {
	return kubernetes.NewPlatformInfoFromVersionMaps(
		&version.Info{},
		map[string]string{datadogCSIDriverKind: "datadoghq.com/v1alpha1"},
		nil,
	)
}

func platformInfoWithoutDDCSI() kubernetes.PlatformInfo {
	return kubernetes.NewPlatformInfoFromVersionMaps(
		&version.Info{},
		map[string]string{},
		nil,
	)
}

func newDDAForDDCSI(name, namespace string, csiEnabled, createDDCSI bool) *v2alpha1.DatadogAgent {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("test-uid"),
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				CSI: &v2alpha1.CSIConfig{
					Enabled:                apiutils.NewBoolPointer(csiEnabled),
					CreateDatadogCSIDriver: apiutils.NewBoolPointer(createDDCSI),
				},
			},
		},
	}
	return dda
}

func TestReconcileDatadogCSIDriver_Disabled(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", false, false)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	assert.NoError(t, err)

	// No DatadogCSIDriver should exist
	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err)
}

func TestReconcileDatadogCSIDriver_EnabledAndCreated(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true, true)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// DatadogCSIDriver should exist with correct owner reference
	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	require.NoError(t, err)
	assert.Equal(t, "test-dda", ddcsi.Name)
	assert.Equal(t, "default", ddcsi.Namespace)
	require.Len(t, ddcsi.OwnerReferences, 1)
	assert.Equal(t, "test-dda", ddcsi.OwnerReferences[0].Name)
	assert.True(t, *ddcsi.OwnerReferences[0].Controller)
}

func TestReconcileDatadogCSIDriver_CRDNotAvailable(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithoutDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true, true)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CRD is not installed")
}

func TestReconcileDatadogCSIDriver_CSIEnabledButCreateFalse(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true, false)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	assert.NoError(t, err)

	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err) // Not found
}

func TestReconcileDatadogCSIDriver_Idempotent(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true, true)

	// First call creates
	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// Second call should not error
	err = r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	assert.NoError(t, err)
}

func TestReconcileDatadogCSIDriver_CleanupOnDisable(t *testing.T) {
	scheme := testScheme()
	r := newTestReconcilerForDDCSI(scheme, platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true, true)

	// Create the DatadogCSIDriver first
	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// Now disable
	dda.Spec.Global.CSI.CreateDatadogCSIDriver = apiutils.NewBoolPointer(false)
	err = r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// Should be deleted
	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err) // Not found
}

func TestReconcileDatadogCSIDriver_CleanupSkipsNotOwned(t *testing.T) {
	scheme := testScheme()
	r := newTestReconcilerForDDCSI(scheme, platformInfoWithDDCSI())

	// Pre-create a DatadogCSIDriver that is NOT owned by our DDA
	ddcsi := &v1alpha1.DatadogCSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dda",
			Namespace: "default",
		},
	}
	err := r.client.Create(context.Background(), ddcsi)
	require.NoError(t, err)

	// Reconcile with CSI disabled — should NOT delete the unowned resource
	dda := newDDAForDDCSI("test-dda", "default", false, false)
	err = r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// Should still exist
	existing := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, existing)
	assert.NoError(t, err)
}

func TestReconcileDatadogCSIDriver_CleanupCRDNotAvailable(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithoutDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", false, false)

	// Should not error when CRD is not available and cleanup is a no-op
	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	assert.NoError(t, err)
}
