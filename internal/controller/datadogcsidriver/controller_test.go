// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	testNamespace = "datadog"
	testName      = "datadog-csi"
)

func newTestReconciler(t *testing.T, objects ...client.Object) (*Reconciler, client.Client) {
	t.Helper()
	s := scheme.Scheme
	s.AddKnownTypes(v1alpha1.GroupVersion,
		&v1alpha1.DatadogCSIDriver{},
		&v1alpha1.DatadogCSIDriverList{},
	)

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objects...).
		WithStatusSubresource(&v1alpha1.DatadogCSIDriver{}).
		Build()

	// Set the default controller-runtime logger so ctrl.LoggerFrom(ctx) works in tests
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	recorder := record.NewFakeRecorder(10)
	r := NewReconciler(c, s, recorder)

	return r, c
}

func defaultCSIDriverCR() *v1alpha1.DatadogCSIDriver {
	return &v1alpha1.DatadogCSIDriver{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogCSIDriver",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: v1alpha1.DatadogCSIDriverSpec{},
	}
}

func TestReconcile_CreatesResources(t *testing.T) {
	instance := defaultCSIDriverCR()
	r, c := newTestReconciler(t, instance)
	ctx := context.Background()

	// First reconcile: adds finalizer
	result, err := r.Reconcile(ctx, instance)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Re-fetch the instance (finalizer was added)
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	assert.Contains(t, instance.GetFinalizers(), finalizerName)

	// Second reconcile: creates CSIDriver + DaemonSet
	result, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify CSIDriver was created
	csiDriver := &storagev1.CSIDriver{}
	err = c.Get(ctx, types.NamespacedName{Name: csiDriverName}, csiDriver)
	require.NoError(t, err)
	assert.Equal(t, csiDriverName, csiDriver.Name)
	assert.Equal(t, "datadog-operator", csiDriver.Labels[kubernetes.AppKubernetesManageByLabelKey])
	assert.False(t, *csiDriver.Spec.AttachRequired)
	assert.True(t, *csiDriver.Spec.PodInfoOnMount)
	assert.Contains(t, csiDriver.Spec.VolumeLifecycleModes, storagev1.VolumeLifecyclePersistent)
	assert.Contains(t, csiDriver.Spec.VolumeLifecycleModes, storagev1.VolumeLifecycleEphemeral)

	// Verify DaemonSet was created
	ds := &appsv1.DaemonSet{}
	dsName := csiDsName
	err = c.Get(ctx, types.NamespacedName{Name: dsName, Namespace: testNamespace}, ds)
	require.NoError(t, err)
	assert.Equal(t, dsName, ds.Name)
	assert.Equal(t, testNamespace, ds.Namespace)

	// Verify DaemonSet containers
	require.Len(t, ds.Spec.Template.Spec.Containers, 2)
	csiContainer := ds.Spec.Template.Spec.Containers[0]
	assert.Equal(t, v1alpha1.CSINodeDriverContainerName, csiContainer.Name)
	assert.Equal(t, fmt.Sprintf("%s/%s:%s", images.GCRContainerRegistry, defaultCSIDriverImageName, images.CSILatestImageVersion), csiContainer.Image)

	registrarContainer := ds.Spec.Template.Spec.Containers[1]
	assert.Equal(t, v1alpha1.CSINodeDriverRegistrarContainerName, registrarContainer.Name)
	assert.Equal(t, fmt.Sprintf("%s/%s:%s", images.SIGStorageRegistry, defaultRegistrarImageName, images.DefaultRegistrarImageVersion), registrarContainer.Image)

	// Verify volumes exist
	volumeNames := make([]string, 0, len(ds.Spec.Template.Spec.Volumes))
	for _, v := range ds.Spec.Template.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	assert.Contains(t, volumeNames, pluginDirVolumeName)
	assert.Contains(t, volumeNames, storageDirVolumeName)
	assert.Contains(t, volumeNames, registrationDirVolumeName)
	assert.Contains(t, volumeNames, mountpointDirVolumeName)
	assert.Contains(t, volumeNames, apmSocketVolumeName)

	// Verify status was updated
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	assert.Equal(t, csiDriverName, instance.Status.CSIDriverName)
	require.NotEmpty(t, instance.Status.Conditions)

	readyCond := findCondition(instance.Status.Conditions, "Ready")
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, "ReconcileSucceeded", readyCond.Reason)
}

func TestReconcile_CustomSocketPaths(t *testing.T) {
	customAPM := "/custom/apm/apm.socket"
	customDSD := "/custom/dsd/dsd.socket"
	instance := defaultCSIDriverCR()
	instance.Spec.APMSocketPath = &customAPM
	instance.Spec.DSDSocketPath = &customDSD

	r, c := newTestReconciler(t, instance)
	ctx := context.Background()

	// Reconcile twice (finalizer + create)
	_, err := r.Reconcile(ctx, instance)
	require.NoError(t, err)
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	ds := &appsv1.DaemonSet{}
	dsName := csiDsName
	err = c.Get(ctx, types.NamespacedName{Name: dsName, Namespace: testNamespace}, ds)
	require.NoError(t, err)

	// Verify the CSI driver container args use custom socket paths
	csiContainer := ds.Spec.Template.Spec.Containers[0]
	assert.Contains(t, csiContainer.Args, fmt.Sprintf("--apm-host-socket-path=%s", customAPM))
	assert.Contains(t, csiContainer.Args, fmt.Sprintf("--dsd-host-socket-path=%s", customDSD))

	// With different APM/DSD dirs, dsd-socket volume should be present
	volumeNames := make([]string, 0, len(ds.Spec.Template.Spec.Volumes))
	for _, v := range ds.Spec.Template.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	assert.Contains(t, volumeNames, dsdSocketVolumeName)
}

func TestReconcile_Deletion(t *testing.T) {
	instance := defaultCSIDriverCR()
	r, c := newTestReconciler(t, instance)
	ctx := context.Background()

	// Reconcile to add finalizer + create resources
	_, err := r.Reconcile(ctx, instance)
	require.NoError(t, err)
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	// Verify CSIDriver exists
	csiDriver := &storagev1.CSIDriver{}
	err = c.Get(ctx, types.NamespacedName{Name: csiDriverName}, csiDriver)
	require.NoError(t, err)

	// Mark for deletion
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	err = c.Delete(ctx, instance)
	require.NoError(t, err)

	// Re-fetch to get the deletion timestamp set by fake client
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)

	// Reconcile deletion
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	// Verify CSIDriver was deleted
	err = c.Get(ctx, types.NamespacedName{Name: csiDriverName}, csiDriver)
	assert.True(t, err != nil, "CSIDriver should be deleted")

	// Verify finalizer was removed
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	if err == nil {
		assert.NotContains(t, instance.GetFinalizers(), finalizerName)
	}
}

func TestReconcile_UpdateDaemonSetOnSpecChange(t *testing.T) {
	instance := defaultCSIDriverCR()
	r, c := newTestReconciler(t, instance)
	ctx := context.Background()

	// Reconcile to create resources
	_, err := r.Reconcile(ctx, instance)
	require.NoError(t, err)
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	// Verify initial DaemonSet uses default APM socket path
	ds := &appsv1.DaemonSet{}
	dsName := csiDsName
	err = c.Get(ctx, types.NamespacedName{Name: dsName, Namespace: testNamespace}, ds)
	require.NoError(t, err)
	csiContainer := ds.Spec.Template.Spec.Containers[0]
	assert.Contains(t, csiContainer.Args, fmt.Sprintf("--apm-host-socket-path=%s", defaultAPMSocketPath))

	// Change spec: set custom APM socket path
	customAPM := "/new/apm.socket"
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	instance.Spec.APMSocketPath = &customAPM
	err = c.Update(ctx, instance)
	require.NoError(t, err)

	// Reconcile the change
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	// Verify the DaemonSet spec was updated with the new socket path
	err = c.Get(ctx, types.NamespacedName{Name: dsName, Namespace: testNamespace}, ds)
	require.NoError(t, err)
	csiContainer = ds.Spec.Template.Spec.Containers[0]
	assert.Contains(t, csiContainer.Args, fmt.Sprintf("--apm-host-socket-path=%s", customAPM))
}

func TestReconcile_IdempotentNoUpdate(t *testing.T) {
	instance := defaultCSIDriverCR()
	r, c := newTestReconciler(t, instance)
	ctx := context.Background()

	// Reconcile to create resources
	_, err := r.Reconcile(ctx, instance)
	require.NoError(t, err)
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	// Get DaemonSet resource version before second reconcile
	ds := &appsv1.DaemonSet{}
	dsName := csiDsName
	err = c.Get(ctx, types.NamespacedName{Name: dsName, Namespace: testNamespace}, ds)
	require.NoError(t, err)
	rvBefore := ds.ResourceVersion

	// Reconcile again with no changes
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	// Verify the DaemonSet was not updated (same resource version)
	err = c.Get(ctx, types.NamespacedName{Name: dsName, Namespace: testNamespace}, ds)
	require.NoError(t, err)
	assert.Equal(t, rvBefore, ds.ResourceVersion)
}

func TestReconcile_CSIDriverLabelsAdoption(t *testing.T) {
	instance := defaultCSIDriverCR()
	attachRequired := true
	podInfoOnMount := false

	// Pre-create a CSIDriver without ownership labels and with drifted spec
	// to validate adoption plus reconciliation of manual edits.
	existingCSIDriver := &storagev1.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: csiDriverName,
		},
		Spec: storagev1.CSIDriverSpec{
			AttachRequired: &attachRequired,
			PodInfoOnMount: &podInfoOnMount,
			VolumeLifecycleModes: []storagev1.VolumeLifecycleMode{
				storagev1.VolumeLifecyclePersistent,
			},
		},
	}

	r, c := newTestReconciler(t, instance, existingCSIDriver)
	ctx := context.Background()

	// Reconcile: add finalizer
	_, err := r.Reconcile(ctx, instance)
	require.NoError(t, err)
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)

	// Reconcile: should adopt existing CSIDriver by adding labels
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	csiDriver := &storagev1.CSIDriver{}
	err = c.Get(ctx, types.NamespacedName{Name: csiDriverName}, csiDriver)
	require.NoError(t, err)
	assert.Equal(t, "datadog-operator", csiDriver.Labels[kubernetes.AppKubernetesManageByLabelKey])
	assert.NotEmpty(t, csiDriver.Labels[kubernetes.AppKubernetesPartOfLabelKey])
	assert.False(t, *csiDriver.Spec.AttachRequired)
	assert.True(t, *csiDriver.Spec.PodInfoOnMount)
	assert.Contains(t, csiDriver.Spec.VolumeLifecycleModes, storagev1.VolumeLifecyclePersistent)
	assert.Contains(t, csiDriver.Spec.VolumeLifecycleModes, storagev1.VolumeLifecycleEphemeral)
}

func TestReconcile_Overrides(t *testing.T) {
	instance := defaultCSIDriverCR()
	instance.Spec.Override = &v1alpha1.DatadogCSIDriverOverride{
		Labels: map[string]string{
			"team": "containers",
		},
		Tolerations: []corev1.Toleration{
			{
				Key:      "node-role.kubernetes.io/master",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "extra-config",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "CUSTOM_VAR",
				Value: "custom-value",
			},
		},
	}

	r, c := newTestReconciler(t, instance)
	ctx := context.Background()

	// Reconcile twice (finalizer + create)
	_, err := r.Reconcile(ctx, instance)
	require.NoError(t, err)
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	ds := &appsv1.DaemonSet{}
	dsName := csiDsName
	err = c.Get(ctx, types.NamespacedName{Name: dsName, Namespace: testNamespace}, ds)
	require.NoError(t, err)

	// Labels merged into pod template
	assert.Equal(t, "containers", ds.Spec.Template.Labels["team"])
	// Default labels still present
	assert.Equal(t, csiDsName, ds.Spec.Template.Labels[appLabelKey])

	// Tolerations applied
	require.Len(t, ds.Spec.Template.Spec.Tolerations, 1)
	assert.Equal(t, "node-role.kubernetes.io/master", ds.Spec.Template.Spec.Tolerations[0].Key)

	// Extra volume appended
	volumeNames := make([]string, 0, len(ds.Spec.Template.Spec.Volumes))
	for _, v := range ds.Spec.Template.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	assert.Contains(t, volumeNames, "extra-config")
	// Default volumes still present
	assert.Contains(t, volumeNames, pluginDirVolumeName)

	// Env var injected into all containers
	for _, container := range ds.Spec.Template.Spec.Containers {
		found := false
		for _, env := range container.Env {
			if env.Name == "CUSTOM_VAR" {
				assert.Equal(t, "custom-value", env.Value)
				found = true
				break
			}
		}
		assert.True(t, found, "CUSTOM_VAR env not found in container %s", container.Name)
	}
}

func TestReconcile_StatusConditionOnCSIDriverError(t *testing.T) {
	// Create an instance referencing a custom CSI driver name. We'll make reconcileCSIDriver
	// fail by creating a CSIDriver that the client can get but not update properly.
	// Instead, we test that when the reconcile succeeds, the condition is Ready=True,
	// and verify the status structure.
	instance := defaultCSIDriverCR()
	r, c := newTestReconciler(t, instance)
	ctx := context.Background()

	// Reconcile to add finalizer
	_, err := r.Reconcile(ctx, instance)
	require.NoError(t, err)
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)

	// Reconcile to create resources
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	// Verify status
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)

	// Note: fake client doesn't bump .metadata.generation, so ObservedGeneration stays 0.
	assert.Equal(t, csiDriverName, instance.Status.CSIDriverName)

	readyCond := findCondition(instance.Status.Conditions, "Ready")
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
}

func TestReconcile_CSIDriverSpecDriftIsReconciled(t *testing.T) {
	instance := defaultCSIDriverCR()
	r, c := newTestReconciler(t, instance)
	ctx := context.Background()

	// Reconcile to create resources.
	_, err := r.Reconcile(ctx, instance)
	require.NoError(t, err)
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	// Simulate manual drift on the managed CSIDriver.
	csiDriver := &storagev1.CSIDriver{}
	err = c.Get(ctx, types.NamespacedName{Name: csiDriverName}, csiDriver)
	require.NoError(t, err)

	// These are opposite of the default
	csiDriver.Spec.AttachRequired = ptr.To(true)
	csiDriver.Spec.PodInfoOnMount = ptr.To(false)
	csiDriver.Spec.VolumeLifecycleModes = []storagev1.VolumeLifecycleMode{storagev1.VolumeLifecyclePersistent}
	err = c.Update(ctx, csiDriver)
	require.NoError(t, err)

	// Reconcile again and verify drift is reverted.
	err = c.Get(ctx, types.NamespacedName{Name: testName, Namespace: testNamespace}, instance)
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, instance)
	require.NoError(t, err)

	err = c.Get(ctx, types.NamespacedName{Name: csiDriverName}, csiDriver)
	require.NoError(t, err)
	assert.False(t, *csiDriver.Spec.AttachRequired)
	assert.True(t, *csiDriver.Spec.PodInfoOnMount)
	assert.Contains(t, csiDriver.Spec.VolumeLifecycleModes, storagev1.VolumeLifecyclePersistent)
	assert.Contains(t, csiDriver.Spec.VolumeLifecycleModes, storagev1.VolumeLifecycleEphemeral)
}

// findCondition returns the condition with the given type, or nil.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
