// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"testing"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestMigrateDaemonSetToExtendedDaemonSet(t *testing.T) {
	ctx := context.Background()
	scheme := newMigrationTestScheme(t)
	daemonSetUID := uuid.NewUUID()
	ddai := newMigrationTestDDAI()
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent",
			Namespace: "datadog",
			UID:       daemonSetUID,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: migrationTestSelectorLabels("datadog-agent")},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type:          appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: migrationTestSelectorLabels("datadog-agent")},
			},
		},
	}
	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent-old",
			Namespace: "datadog",
			Labels:    migrationTestSelectorLabels("datadog-agent"),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "DaemonSet",
				Name:       daemonSet.Name,
				UID:        daemonSetUID,
				Controller: &controller,
			}},
		},
	}
	eds := &edsv1alpha1.ExtendedDaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: daemonSet.Name, Namespace: daemonSet.Namespace},
		Spec: edsv1alpha1.ExtendedDaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: migrationTestSelectorLabels("datadog-agent")},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(daemonSet, pod).Build()
	reconciler := newMigrationTestReconciler(fakeClient, scheme)
	status := &datadoghqv1alpha1.DatadogAgentInternalStatus{}

	// First pass makes future DaemonSet pods discoverable by the EDS controller
	// without rolling the pods that are already running.
	handled, result, err := reconciler.migrateDaemonSetToExtendedDaemonSet(ctx, ddai, eds, status)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, workloadMigrationResult(), result)

	currentDaemonSet := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(daemonSet), currentDaemonSet))
	assert.Equal(t, appsv1.OnDeleteDaemonSetStrategyType, currentDaemonSet.Spec.UpdateStrategy.Type)
	assert.Nil(t, currentDaemonSet.Spec.UpdateStrategy.RollingUpdate)
	assert.Equal(t, eds.Name, currentDaemonSet.Spec.Template.Labels[edsv1alpha1.ExtendedDaemonSetNameLabelKey])

	// Second pass labels the existing pods and orphans them while deleting the
	// old DaemonSet.
	handled, result, err = reconciler.migrateDaemonSetToExtendedDaemonSet(ctx, ddai, eds, status)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, workloadMigrationResult(), result)
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(daemonSet), &appsv1.DaemonSet{})
	assert.True(t, apierrors.IsNotFound(err))

	currentPod := &corev1.Pod{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(pod), currentPod))
	assert.Equal(t, eds.Name, currentPod.Labels[edsv1alpha1.ExtendedDaemonSetNameLabelKey])

	// Once the old workload is gone, reconciliation can create the EDS. A
	// repeated pass remains a no-op, which also covers controller restarts at
	// this point in the migration.
	handled, result, err = reconciler.migrateDaemonSetToExtendedDaemonSet(ctx, ddai, eds, status)
	require.NoError(t, err)
	assert.False(t, handled)
	assert.True(t, result.IsZero())
	handled, _, err = reconciler.migrateDaemonSetToExtendedDaemonSet(ctx, ddai, eds, status)
	require.NoError(t, err)
	assert.False(t, handled)
}

func TestMigrateExtendedDaemonSetToDaemonSet(t *testing.T) {
	ctx := context.Background()
	scheme := newMigrationTestScheme(t)
	ddai := newMigrationTestDDAI()
	edsUID := uuid.NewUUID()
	replicaSetUID := uuid.NewUUID()
	eds := &edsv1alpha1.ExtendedDaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent",
			Namespace: "datadog",
			UID:       edsUID,
		},
	}
	controller := true
	replicaSet := &edsv1alpha1.ExtendedDaemonSetReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent-abcde",
			Namespace: eds.Namespace,
			UID:       replicaSetUID,
			Labels: map[string]string{
				edsv1alpha1.ExtendedDaemonSetNameLabelKey: eds.Name,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "datadoghq.com/v1alpha1",
				Kind:       "ExtendedDaemonSet",
				Name:       eds.Name,
				UID:        edsUID,
				Controller: &controller,
			}},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent-abcde-node",
			Namespace: eds.Namespace,
			Labels: map[string]string{
				edsv1alpha1.ExtendedDaemonSetNameLabelKey: eds.Name,
				kubernetes.AppKubernetesInstanceLabelKey:  "old-instance",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "datadoghq.com/v1alpha1",
				Kind:       "ExtendedDaemonSetReplicaSet",
				Name:       replicaSet.Name,
				UID:        replicaSetUID,
				Controller: &controller,
			}},
		},
	}
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: eds.Name, Namespace: eds.Namespace},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: migrationTestSelectorLabels("datadog-agent")},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(eds, replicaSet, pod).Build()
	reconciler := newMigrationTestReconciler(fakeClient, scheme)
	status := &datadoghqv1alpha1.DatadogAgentInternalStatus{}

	// The first two passes peel the EDS ownership chain while preserving pods.
	handled, _, err := reconciler.migrateExtendedDaemonSetToDaemonSet(ctx, ddai, daemonSet, status)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.True(t, apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(eds), &edsv1alpha1.ExtendedDaemonSet{})))

	handled, _, err = reconciler.migrateExtendedDaemonSetToDaemonSet(ctx, ddai, daemonSet, status)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.True(t, apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(replicaSet), &edsv1alpha1.ExtendedDaemonSetReplicaSet{})))

	// The fake client does not run Kubernetes garbage collection, so explicitly
	// model orphan propagation removing the ERS controller reference. Until that
	// happens, the migration waits instead of creating a competing DaemonSet.
	handled, _, err = reconciler.migrateExtendedDaemonSetToDaemonSet(ctx, ddai, daemonSet, status)
	require.NoError(t, err)
	assert.True(t, handled)
	currentPod := &corev1.Pod{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(pod), currentPod))
	currentPod.OwnerReferences = nil
	require.NoError(t, fakeClient.Update(ctx, currentPod))

	// The next pass updates immutable-selector labels ahead of adoption, and the
	// following pass allows the native DaemonSet to be created.
	handled, _, err = reconciler.migrateExtendedDaemonSetToDaemonSet(ctx, ddai, daemonSet, status)
	require.NoError(t, err)
	assert.True(t, handled)
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(pod), currentPod))
	assert.Equal(t, "datadog-agent", currentPod.Labels[kubernetes.AppKubernetesInstanceLabelKey])
	assert.Equal(t, "agent", currentPod.Labels[apicommon.AgentDeploymentComponentLabelKey])

	handled, result, err := reconciler.migrateExtendedDaemonSetToDaemonSet(ctx, ddai, daemonSet, status)
	require.NoError(t, err)
	assert.False(t, handled)
	assert.True(t, result.IsZero())
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{}))
}

func TestPrepareOrphanedEDSPodsForDaemonSetDeletesUnmatchablePod(t *testing.T) {
	ctx := context.Background()
	scheme := newMigrationTestScheme(t)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent-old",
			Namespace: "datadog",
			Labels: map[string]string{
				edsv1alpha1.ExtendedDaemonSetNameLabelKey: "datadog-agent",
			},
		},
	}
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent", Namespace: "datadog"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "migration-ready",
					Operator: metav1.LabelSelectorOpExists,
				}},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	reconciler := newMigrationTestReconciler(fakeClient, scheme)

	handled, err := reconciler.prepareOrphanedEDSPodsForDaemonSet(ctx, daemonSet)
	require.NoError(t, err)
	assert.True(t, handled)
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestMigrateExtendedDaemonSetToDaemonSetWithoutEDSTypesInScheme(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent", Namespace: "datadog"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: migrationTestSelectorLabels("datadog-agent")},
		},
	}
	reconciler := newMigrationTestReconciler(fake.NewClientBuilder().WithScheme(scheme).Build(), scheme)

	handled, result, err := reconciler.migrateExtendedDaemonSetToDaemonSet(
		ctx,
		newMigrationTestDDAI(),
		daemonSet,
		&datadoghqv1alpha1.DatadogAgentInternalStatus{},
	)
	require.NoError(t, err)
	assert.False(t, handled)
	assert.True(t, result.IsZero())
}

func TestMigrateDaemonSetToExtendedDaemonSetRollbackCleansStaleReplicaSet(t *testing.T) {
	ctx := context.Background()
	scheme := newMigrationTestScheme(t)
	ddai := newMigrationTestDDAI()
	replicaSet := &edsv1alpha1.ExtendedDaemonSetReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent-stale",
			Namespace: "datadog",
			Labels: map[string]string{
				edsv1alpha1.ExtendedDaemonSetNameLabelKey: "datadog-agent",
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent-stale-node",
			Namespace: "datadog",
			Labels: map[string]string{
				edsv1alpha1.ExtendedDaemonSetNameLabelKey: "datadog-agent",
			},
		},
	}
	eds := &edsv1alpha1.ExtendedDaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent", Namespace: "datadog"},
		Spec: edsv1alpha1.ExtendedDaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: migrationTestSelectorLabels("datadog-agent")},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(replicaSet, pod).Build()
	reconciler := newMigrationTestReconciler(fakeClient, scheme)

	handled, _, err := reconciler.migrateDaemonSetToExtendedDaemonSet(
		ctx,
		ddai,
		eds,
		&datadoghqv1alpha1.DatadogAgentInternalStatus{},
	)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.True(t, apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(replicaSet), &edsv1alpha1.ExtendedDaemonSetReplicaSet{})))
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{}))

	handled, _, err = reconciler.migrateDaemonSetToExtendedDaemonSet(
		ctx,
		ddai,
		eds,
		&datadoghqv1alpha1.DatadogAgentInternalStatus{},
	)
	require.NoError(t, err)
	assert.False(t, handled)
}

func newMigrationTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, edsv1alpha1.AddToScheme(scheme))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))
	return scheme
}

func newMigrationTestReconciler(c client.Client, scheme *runtime.Scheme) *Reconciler {
	return &Reconciler{
		client:   c,
		scheme:   scheme,
		recorder: record.NewFakeRecorder(20),
	}
}

func newMigrationTestDDAI() *datadoghqv1alpha1.DatadogAgentInternal {
	return &datadoghqv1alpha1.DatadogAgentInternal{
		TypeMeta: metav1.TypeMeta{
			APIVersion: datadoghqv1alpha1.GroupVersion.String(),
			Kind:       "DatadogAgentInternal",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "datadog",
			UID:       types.UID("ddai-uid"),
		},
	}
}

func migrationTestSelectorLabels(instance string) map[string]string {
	return map[string]string{
		kubernetes.AppKubernetesInstanceLabelKey:   instance,
		apicommon.AgentDeploymentComponentLabelKey: "agent",
	}
}
