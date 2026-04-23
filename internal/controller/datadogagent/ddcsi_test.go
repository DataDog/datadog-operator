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
	corev1 "k8s.io/api/core/v1"
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
		options:      ReconcilerOptions{DatadogCSIDriverEnabled: true},
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

func externalCSIDriver() *storagev1.CSIDriver {
	return &storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: datadogCSIDriverObjectName,
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

func TestReconcileDatadogCSIDriver_SpecFromDDA(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

	apmPath := "/custom/apm.socket"
	dsdPath := "/custom/dsd.socket"
	dda.Spec.Features = &v2alpha1.DatadogFeatures{
		APM: &v2alpha1.APMFeatureConfig{
			UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{Path: ptr.To(apmPath)},
		},
		Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
			UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{Path: ptr.To(dsdPath)},
		},
	}
	nodeAgentTolerations := []corev1.Toleration{
		{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "datadog", Effect: corev1.TaintEffectNoSchedule},
	}
	dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.NodeAgentComponentName: {Tolerations: nodeAgentTolerations},
	}

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	require.NoError(t, err)

	require.NotNil(t, ddcsi.Spec.APMSocketPath)
	assert.Equal(t, apmPath, *ddcsi.Spec.APMSocketPath)
	require.NotNil(t, ddcsi.Spec.DSDSocketPath)
	assert.Equal(t, dsdPath, *ddcsi.Spec.DSDSocketPath)

	require.NotNil(t, ddcsi.Spec.Override)
	assert.Equal(t, nodeAgentTolerations, ddcsi.Spec.Override.Tolerations)
}

func TestReconcileDatadogCSIDriver_NoOverrideWhenNoTolerations(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	require.NoError(t, err)
	assert.Nil(t, ddcsi.Spec.Override)
}

func TestReconcileDatadogCSIDriver_ManageDisabledCleansUpAndDoesNotRecreate(t *testing.T) {
	// Migration path: user opts out via manageDatadogCSIDriver=false while keeping csi.enabled=true.
	// The operator must clean up the DDA-owned CR it created previously and not recreate it,
	// letting the user take over with a DatadogCSIDriver CR they maintain themselves.
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

	// Initial reconcile: default management (field unset) creates the CR.
	require.NoError(t, r.reconcileDatadogCSIDriver(context.Background(), r.log, dda))
	ddcsi := &v1alpha1.DatadogCSIDriver{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi))

	// User opts out of management.
	dda.Spec.Global.CSI.ManageDatadogCSIDriver = ptr.To(false)
	require.NoError(t, r.reconcileDatadogCSIDriver(context.Background(), r.log, dda))

	// DDA-owned CR is gone.
	err := r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err)

	// Subsequent reconciles with manage=false must not recreate it.
	require.NoError(t, r.reconcileDatadogCSIDriver(context.Background(), r.log, dda))
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err)
}

func TestReconcileDatadogCSIDriver_ManageDisabledSkipsForeignCR(t *testing.T) {
	// When manage=false and a non-DDA-owned DatadogCSIDriver CR happens to exist at the same
	// namespace/name, the cleanup must not touch it (cleanup's IsControlledBy guard).
	scheme := testScheme()
	r := newTestReconcilerForDDCSI(scheme, platformInfoWithDDCSI())

	foreign := &v1alpha1.DatadogCSIDriver{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dda", Namespace: "default"},
	}
	require.NoError(t, r.client.Create(context.Background(), foreign))

	dda := newDDAForDDCSI("test-dda", "default", true)
	dda.Spec.Global.CSI.ManageDatadogCSIDriver = ptr.To(false)

	require.NoError(t, r.reconcileDatadogCSIDriver(context.Background(), r.log, dda))

	// Foreign CR still exists.
	existing := &v1alpha1.DatadogCSIDriver{}
	assert.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, existing))
}

func TestReconcileDatadogCSIDriver_ControllerDisabled(t *testing.T) {
	// Backward compat: spec.global.csi.enabled=true with --datadogCSIDriverEnabled=false (the
	// default) must be a no-op. Existing users who set csi.enabled=true for an externally
	// installed driver must not suddenly get a DDA-owned CR nor a reconcile error.
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	r.options.DatadogCSIDriverEnabled = false
	dda := newDDAForDDCSI("test-dda", "default", true)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// No DatadogCSIDriver CR created.
	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err)
}

func TestReconcileDatadogCSIDriver_DefersToExternalCSIDriver(t *testing.T) {
	// On the first-time create path, if a cluster-scoped `k8s.csi.datadoghq.com` CSIDriver is
	// already installed (by any tool), the operator must defer and not create its own CR.
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI(), externalCSIDriver())
	dda := newDDAForDDCSI("test-dda", "default", true)

	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	ddcsi := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi)
	assert.Error(t, err)
}

func TestReconcileDatadogCSIDriver_ExternalAppearsAfterCRCreated(t *testing.T) {
	// Once our CR exists, we keep managing it. A later-appearing external CSIDriver must not
	// cause us to abandon the CR: the external check only gates the first-time create.
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

	require.NoError(t, r.reconcileDatadogCSIDriver(context.Background(), r.log, dda))

	ddcsi := &v1alpha1.DatadogCSIDriver{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi))

	require.NoError(t, r.client.Create(context.Background(), externalCSIDriver()))

	require.NoError(t, r.reconcileDatadogCSIDriver(context.Background(), r.log, dda))

	// Our CR must still be there.
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, ddcsi))
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

	// Reconcile with CSI disabled, should NOT delete the unowned resource
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

	// Reconcile again, should update back to the desired state (empty spec)
	err = r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	require.NoError(t, err)

	// Verify the spec was reconciled back to desired state
	updated := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, updated)
	require.NoError(t, err)
	assert.Nil(t, updated.Spec.APMSocketPath)
}

func TestReconcileDatadogCSIDriver_UpdatePreservesFinalizers(t *testing.T) {
	// The DatadogCSIDriver controller adds its cleanup finalizer to this object. An operator-driven
	// spec-drift update must preserve it. Otherwise a subsequent delete/disable would bypass
	// cleanup and leak the cluster-scoped CSIDriver.
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", true)

	require.NoError(t, r.reconcileDatadogCSIDriver(context.Background(), r.log, dda))

	existing := &v1alpha1.DatadogCSIDriver{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, existing))
	existing.Finalizers = append(existing.Finalizers, "finalizer.datadoghq.com/csi-driver")
	require.NoError(t, r.client.Update(context.Background(), existing))

	// Cause spec drift via the DDA (APM socket path).
	dda.Spec.Features = &v2alpha1.DatadogFeatures{
		APM: &v2alpha1.APMFeatureConfig{
			UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{Path: ptr.To("/custom/apm.socket")},
		},
	}
	require.NoError(t, r.reconcileDatadogCSIDriver(context.Background(), r.log, dda))

	updated := &v1alpha1.DatadogCSIDriver{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "test-dda", Namespace: "default"}, updated))

	require.NotNil(t, updated.Spec.APMSocketPath)
	assert.Equal(t, "/custom/apm.socket", *updated.Spec.APMSocketPath)
	assert.Contains(t, updated.Finalizers, "finalizer.datadoghq.com/csi-driver")
}

func TestReconcileDatadogCSIDriver_CleanupCRDNotAvailable(t *testing.T) {
	r := newTestReconcilerForDDCSI(testScheme(), platformInfoWithoutDDCSI())
	dda := newDDAForDDCSI("test-dda", "default", false)

	// Should not error when CRD is not available and cleanup is a no-op
	err := r.reconcileDatadogCSIDriver(context.Background(), r.log, dda)
	assert.NoError(t, err)
}
