// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

// newRolloutTestReconciler builds a Reconciler with a fake client seeded with objs,
// suitable for driving createOrUpdateDaemonset/annotateWithReferencedConfigMapsChecksum directly.
func newRolloutTestReconciler(flagEnabled bool, objs ...client.Object) *Reconciler {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "Test_configmapRollout"})
	return &Reconciler{
		client:   fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build(),
		scheme:   sch,
		recorder: recorder,
		options:  ReconcilerOptions{RolloutOnConfigMapChangeEnabled: flagEnabled},
	}
}

func newRolloutTestDDAI() *v1alpha1.DatadogAgentInternal {
	return &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{Name: "dda-foo", Namespace: "ns-1"},
	}
}

func newRolloutTestDaemonSet() *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "dda-foo-agent", Namespace: "ns-1"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "agent"}},
				Spec: corev1.PodSpec{
					Volumes:    []corev1.Volume{configMapVolume("agent-custom-config")},
					Containers: []corev1.Container{{Name: "agent", Image: "agent:latest"}},
				},
			},
		},
	}
}

// reconcileOnce drives one createOrUpdate pass: applies the checksum annotation (if enabled)
// then calls createOrUpdateDaemonset, mirroring the real call site in reconcileV2Agent.
func reconcileOnce(t *testing.T, r *Reconciler, ddai *v1alpha1.DatadogAgentInternal, ds *appsv1.DaemonSet) {
	t.Helper()
	ctx := context.Background()
	if r.options.RolloutOnConfigMapChangeEnabled {
		require.NoError(t, r.annotateWithReferencedConfigMapsChecksum(ctx, ddai.Namespace, &ds.Spec.Template))
	}
	_, err := r.createOrUpdateDaemonset(ctx, ddai, ds, &v1alpha1.DatadogAgentInternalStatus{}, updateDSStatusV2WithAgent)
	require.NoError(t, err)
}

func getDaemonSet(t *testing.T, r *Reconciler, ds *appsv1.DaemonSet) *appsv1.DaemonSet {
	t.Helper()
	got := &appsv1.DaemonSet{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: ds.Name, Namespace: ds.Namespace}, got))
	return got
}

func Test_ConfigMapRollout_FlagDisabled(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "agent-custom-config", Namespace: "ns-1"}, Data: map[string]string{"foo": "bar"}}
	r := newRolloutTestReconciler(false, cm)
	ddai := newRolloutTestDDAI()
	ds := newRolloutTestDaemonSet()

	reconcileOnce(t, r, ddai, ds)

	got := getDaemonSet(t, r, ds)
	_, ok := got.Spec.Template.Annotations[constants.MD5ReferencedConfigMapsAnnotationKey]
	assert.False(t, ok, "checksum annotation should never be added when the flag is disabled")
}

func Test_ConfigMapRollout_ContentChange_TriggersUpdate(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "agent-custom-config", Namespace: "ns-1"}, Data: map[string]string{"foo": "bar"}}
	r := newRolloutTestReconciler(true, cm)
	ddai := newRolloutTestDDAI()

	reconcileOnce(t, r, ddai, newRolloutTestDaemonSet())
	firstVersion := getDaemonSet(t, r, newRolloutTestDaemonSet()).ResourceVersion

	// Simulate an out-of-band edit to the referenced ConfigMap.
	liveCM := &corev1.ConfigMap{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "agent-custom-config", Namespace: "ns-1"}, liveCM))
	liveCM.Data = map[string]string{"foo": "baz"}
	require.NoError(t, r.client.Update(context.Background(), liveCM))

	// Reconcile again with no direct DDAI/DaemonSet spec change.
	reconcileOnce(t, r, ddai, newRolloutTestDaemonSet())

	got := getDaemonSet(t, r, newRolloutTestDaemonSet())
	assert.NotEqual(t, firstVersion, got.ResourceVersion, "DaemonSet should be updated when a referenced ConfigMap's content changes")
}

func Test_ConfigMapRollout_NoChange_NoUpdate(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "agent-custom-config", Namespace: "ns-1"}, Data: map[string]string{"foo": "bar"}}
	r := newRolloutTestReconciler(true, cm)
	ddai := newRolloutTestDDAI()

	reconcileOnce(t, r, ddai, newRolloutTestDaemonSet())
	firstVersion := getDaemonSet(t, r, newRolloutTestDaemonSet()).ResourceVersion

	// Reconcile again with nothing changed anywhere.
	reconcileOnce(t, r, ddai, newRolloutTestDaemonSet())

	got := getDaemonSet(t, r, newRolloutTestDaemonSet())
	assert.Equal(t, firstVersion, got.ResourceVersion, "DaemonSet should not be updated when nothing changed")
}

func Test_ConfigMapRollout_MissingConfigMap_NoCrash(t *testing.T) {
	r := newRolloutTestReconciler(true) // no ConfigMap seeded
	ddai := newRolloutTestDDAI()

	reconcileOnce(t, r, ddai, newRolloutTestDaemonSet())

	got := getDaemonSet(t, r, newRolloutTestDaemonSet())
	assert.NotEmpty(t, got.ResourceVersion, "DaemonSet should still be created normally despite the dangling ConfigMap reference")
}

func Test_ConfigMapRollout_ConfigMapChangeAndSpecChange_SingleUpdate(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "agent-custom-config", Namespace: "ns-1"}, Data: map[string]string{"foo": "bar"}}
	r := newRolloutTestReconciler(true, cm)
	ddai := newRolloutTestDDAI()

	reconcileOnce(t, r, ddai, newRolloutTestDaemonSet())
	firstVersion := getDaemonSet(t, r, newRolloutTestDaemonSet()).ResourceVersion

	// Mutate the referenced ConfigMap...
	liveCM := &corev1.ConfigMap{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "agent-custom-config", Namespace: "ns-1"}, liveCM))
	liveCM.Data = map[string]string{"foo": "baz"}
	require.NoError(t, r.client.Update(context.Background(), liveCM))

	// ...AND make a direct spec change in the same reconcile pass.
	dsWithSpecChange := newRolloutTestDaemonSet()
	dsWithSpecChange.Spec.Template.Spec.Containers[0].Image = "agent:new-tag"

	reconcileOnce(t, r, ddai, dsWithSpecChange)

	got := getDaemonSet(t, r, newRolloutTestDaemonSet())
	assert.NotEqual(t, firstVersion, got.ResourceVersion, "DaemonSet should be updated")

	// A second reconcile with no further changes should be a no-op, proving the
	// combined change above resulted in exactly one update, not two.
	stableVersion := got.ResourceVersion
	reconcileOnce(t, r, ddai, dsWithSpecChange)
	got = getDaemonSet(t, r, newRolloutTestDaemonSet())
	assert.Equal(t, stableVersion, got.ResourceVersion, "no further update should occur once both changes have been applied once")
}

func newRolloutTestClusterAgentDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dda-foo-cluster-agent", Namespace: "ns-1"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "cluster-agent"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "cluster-agent"}},
				Spec: corev1.PodSpec{
					Volumes:    []corev1.Volume{configMapVolume("cluster-agent-custom-config")},
					Containers: []corev1.Container{{Name: "cluster-agent", Image: "cluster-agent:latest"}},
				},
			},
		},
	}
}

func noopUpdateDepStatus(*appsv1.Deployment, *v1alpha1.DatadogAgentInternalStatus, metav1.Time, metav1.ConditionStatus, string, string) {}

func Test_ConfigMapRollout_ClusterAgent_ContentChange_TriggersUpdate(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cluster-agent-custom-config", Namespace: "ns-1"}, Data: map[string]string{"foo": "bar"}}
	r := newRolloutTestReconciler(true, cm)
	ddai := newRolloutTestDDAI()
	ctx := context.Background()

	reconcileOnDeployment := func(dep *appsv1.Deployment) {
		require.NoError(t, r.annotateWithReferencedConfigMapsChecksum(ctx, ddai.Namespace, &dep.Spec.Template))
		_, err := r.createOrUpdateDeployment(ctx, ddai, dep, &v1alpha1.DatadogAgentInternalStatus{}, noopUpdateDepStatus)
		require.NoError(t, err)
	}
	getDeployment := func() *appsv1.Deployment {
		got := &appsv1.Deployment{}
		require.NoError(t, r.client.Get(ctx, types.NamespacedName{Name: "dda-foo-cluster-agent", Namespace: "ns-1"}, got))
		return got
	}

	reconcileOnDeployment(newRolloutTestClusterAgentDeployment())
	firstVersion := getDeployment().ResourceVersion

	liveCM := &corev1.ConfigMap{}
	require.NoError(t, r.client.Get(ctx, types.NamespacedName{Name: "cluster-agent-custom-config", Namespace: "ns-1"}, liveCM))
	liveCM.Data = map[string]string{"foo": "baz"}
	require.NoError(t, r.client.Update(ctx, liveCM))

	reconcileOnDeployment(newRolloutTestClusterAgentDeployment())

	assert.NotEqual(t, firstVersion, getDeployment().ResourceVersion, "Cluster Agent Deployment should be updated when a referenced ConfigMap's content changes")
}
