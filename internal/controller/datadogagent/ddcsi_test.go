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
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func newTestReconcilerForDDCSI(scheme *runtime.Scheme, platformInfo kubernetes.PlatformInfo, initObjs ...runtime.Object) *Reconciler {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initObjs...).Build()
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
	_ = storagev1.AddToScheme(s)
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

func newDDAForDDCSI(name, namespace string, csiEnabled bool) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("test-uid"),
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				CSI: &v2alpha1.CSIConfig{
					Enabled: ptr.To(csiEnabled),
				},
			},
		},
	}
}

func helmManagedCSIDriver() *storagev1.CSIDriver {
	return &storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: helmManagedCSIDriverName,
			Labels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: helmManagedByLabelValue,
			},
		},
	}
}

func TestReconcileDatadogCSIDriver_Disabled(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", false)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	assert.NoError(t, err)

	// No DatadogCSIDriver should exist
	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err)
}

func TestReconcileDatadogCSIDriver_EnabledAndCreated(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

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

	// Default operator-managed labels should be present.
	assert.Equal(t, "datadog-operator", ddcsi.Labels[kubernetes.AppKubernetesManageByLabelKey])
	assert.Equal(t, "test-dda", ddcsi.Labels[kubernetes.AppKubernetesInstanceLabelKey])
	assert.Equal(t, "default-test--dda", ddcsi.Labels[kubernetes.AppKubernetesPartOfLabelKey])
	assert.Equal(t, "datadog-agent-deployment", ddcsi.Labels[kubernetes.AppKubernetesNameLabelKey])
}

func TestReconcileDatadogCSIDriver_CRDNotAvailable(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithoutDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CRD is not installed")
}

func TestReconcileDatadogCSIDriver_HelmManagedPresent(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI(), helmManagedCSIDriver())
	dda := newDDAForDDCSI("test-dda", "default", true)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// DatadogCSIDriver must NOT be created when a Helm-managed CSIDriver exists.
	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err)
}

func TestReconcileDatadogCSIDriver_NonHelmManagedCSIDriverPresent(t *testing.T) {
	// A CSIDriver exists with the Datadog name but without the Helm managed-by label.
	// The operator should still create the DatadogCSIDriver — only Helm-managed installs are deferred to.
	existing := &storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: helmManagedCSIDriverName,
			Labels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
			},
		},
	}
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI(), existing)
	dda := newDDAForDDCSI("test-dda", "default", true)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	require.NoError(t, err)
}

func TestReconcileDatadogCSIDriver_HelmManagedAppearsLater(t *testing.T) {
	// Operator first creates the DatadogCSIDriver (no Helm install), then a Helm CSIDriver shows up
	// on a later reconcile — the operator must clean up its own DatadogCSIDriver.
	scheme := testScheme()
	r := newTestReconcilerForDDCSI(scheme, platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	require.NoError(t, err)

	require.NoError(t, r.client.Create(context.Background(), helmManagedCSIDriver()))

	err = r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err)
}

func TestReconcileDatadogCSIDriver_Idempotent(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

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
	dda := newDDAForDDCSI("test-dda", "default", true)

	// Create the DatadogCSIDriver first
	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// Now disable
	dda.Spec.Global.CSI.Enabled = ptr.To(false)
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
	dda := newDDAForDDCSI("test-dda", "default", false)
	err = r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// Should still exist
	existing := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, existing)
	assert.NoError(t, err)
}

func TestReconcileDatadogCSIDriver_UpdateOnSpecDrift(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

	// Create the DatadogCSIDriver via reconcile
	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// Simulate external modification: someone sets a custom APM socket path
	existing := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, existing)
	require.NoError(t, err)
	customPath := "/custom/apm.socket"
	existing.Spec.APMSocketPath = &customPath
	err = r.client.Update(context.Background(), existing)
	require.NoError(t, err)

	// Reconcile again — should update back to the desired state (empty spec)
	err = r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// Verify the spec was reconciled back to desired state
	updated := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, updated)
	require.NoError(t, err)
	assert.Nil(t, updated.Spec.APMSocketPath)
}

func TestReconcileDatadogCSIDriver_CleanupCRDNotAvailable(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithoutDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", false)

	// Should not error when CRD is not available and cleanup is a no-op
	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	assert.NoError(t, err)
}
