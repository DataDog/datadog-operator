// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	controllercommon "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedFreshInstallStagesBlueBeforeCreatingGreen(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	status := &datadoghqv1alpha1.DatadogAgentInternalStatus{}
	key := preparedRolloutNodeLabelKey(ddai)

	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, node.Name, key))
	assert.Error(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: rendered.Name}, &appsv1.DaemonSet{}))

	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	setPreparedDaemonSetStatus(t, ctx, fakeClient, rendered.Namespace, rendered.Name, 1, 0, 0, 1)
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	assert.Error(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: suffixedKubernetesName(rendered.Name, "-green")}, &appsv1.DaemonSet{}), "green must not exist before blue is fully rolled out")

	setPreparedDaemonSetStatus(t, ctx, fakeClient, rendered.Namespace, rendered.Name, 1, 1, 1, 0)
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	blue := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: rendered.Name}, blue))
	assert.Equal(t, appsv1.OnDeleteDaemonSetStrategyType, blue.Spec.UpdateStrategy.Type)
	assert.Equal(t, preparedRolloutSchemaVersion, blue.Spec.Template.Annotations[preparedRolloutSchemaAnnotation])

	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	green := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: suffixedKubernetesName(rendered.Name, "-green")}, green))
	assert.Equal(t, appsv1.OnDeleteDaemonSetStrategyType, green.Spec.UpdateStrategy.Type)
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, node.Name, key))
}

func TestPreparedConventionalMigrationDoesNotCreateGreenBeforeArmingCompletes(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	status := &datadoghqv1alpha1.DatadogAgentInternalStatus{}
	key := preparedRolloutNodeLabelKey(ddai)
	existing := rendered.DeepCopy()
	setPreparedTestController(existing, ddai)
	require.NoError(t, fakeClient.Create(ctx, existing))
	setPreparedDaemonSetStatus(t, ctx, fakeClient, rendered.Namespace, rendered.Name, 1, 1, 1, 0)

	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, node.Name, key))
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	setPreparedDaemonSetStatus(t, ctx, fakeClient, rendered.Namespace, rendered.Name, 1, 0, 1, 0)
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	assert.Error(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: suffixedKubernetesName(rendered.Name, "-green")}, &appsv1.DaemonSet{}))

	setPreparedDaemonSetStatus(t, ctx, fakeClient, rendered.Namespace, rendered.Name, 1, 1, 1, 0)
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: suffixedKubernetesName(rendered.Name, "-green")}, &appsv1.DaemonSet{}))
}

func TestPreparedConventionalMigrationRequiresImagesToBeStagedFirst(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	rendered.Spec.Template.Spec.Containers[0].Image = "agent:gate-capable"
	existing := rendered.DeepCopy()
	existing.Spec.Template.Spec.Containers[0].Image = "agent:old"
	setPreparedTestController(existing, ddai)
	require.NoError(t, fakeClient.Create(ctx, existing))
	setPreparedDaemonSetStatus(t, ctx, fakeClient, rendered.Namespace, rendered.Name, 1, 1, 1, 0)

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err := r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.ErrorContains(t, err, "deployed conventionally first")
	assert.Empty(t, nodeState(t, ctx, fakeClient, node.Name, preparedRolloutNodeLabelKey(ddai)))
	current := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(existing), current))
	assert.Equal(t, "agent:old", current.Spec.Template.Spec.Containers[0].Image)
}

func TestPreparedConventionalMigrationWaitsForStagedRolloutToFinish(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	rendered.Spec.Template.Spec.Containers[0].Image = "agent:gate-capable"
	existing := rendered.DeepCopy()
	setPreparedTestController(existing, ddai)
	require.NoError(t, fakeClient.Create(ctx, existing))
	setPreparedDaemonSetStatus(t, ctx, fakeClient, rendered.Namespace, rendered.Name, 1, 0, 0, 1)

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err := r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.ErrorContains(t, err, "rollout to finish")
	assert.Empty(t, nodeState(t, ctx, fakeClient, node.Name, preparedRolloutNodeLabelKey(ddai)))
}

func TestPreparedConventionalMigrationCoversNodeJoiningWhileOperatorIsDown(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, _, fakeClient, scheme := preparedLifecycleFixture(t)
	status := &datadoghqv1alpha1.DatadogAgentInternalStatus{}
	existing := rendered.DeepCopy()
	setPreparedTestController(existing, ddai)
	require.NoError(t, fakeClient.Create(ctx, existing))
	setPreparedDaemonSetStatus(t, ctx, fakeClient, rendered.Namespace, rendered.Name, 1, 1, 1, 0)

	// The first pass persists blue ownership; the second arms the conventional
	// DaemonSet. Every helper call constructs a new reconciler, modeling an
	// Operator restart at each boundary.
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	arming := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(rendered), arming))
	assert.Equal(t, appsv1.RollingUpdateDaemonSetStrategyType, arming.Spec.UpdateStrategy.Type)
	assert.True(t, slotAffinityAllowsUnlabeled(arming, preparedRolloutNodeLabelKey(ddai)), "blue must cover nodes which join before the Operator can label them")

	joining := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b", Labels: map[string]string{corev1.LabelOSStable: string(corev1.Linux)}}}
	require.NoError(t, fakeClient.Create(ctx, joining))
	setPreparedDaemonSetStatus(t, ctx, fakeClient, rendered.Namespace, rendered.Name, 2, 2, 2, 0)
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	reconcilePreparedLifecycle(t, ctx, fakeClient, scheme, ddai, rendered, status)
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, joining.Name, preparedRolloutNodeLabelKey(ddai)))
}

func TestPreparedMissingInitializedGreenChildIsRecreated(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	key := preparedRolloutNodeLabelKey(ddai)
	node.Labels[key] = rolloutSlotBlue
	require.NoError(t, fakeClient.Update(ctx, node))
	blue := rendered.DeepCopy()
	blue.UID = "blue-uid"
	blue.Annotations = map[string]string{
		preparedRolloutPairInitializedAnnotation: preparedBlueGreenArmed,
		preparedRolloutActiveSlotAnnotation:      rolloutSlotBlue,
		preparedRolloutRevisionAnnotation:        "test-revision",
	}
	blue.Spec.Template.Annotations = map[string]string{
		preparedRolloutModeAnnotation:     preparedBlueGreenMode,
		preparedRolloutRevisionAnnotation: "test-revision",
	}
	setPreparedTestController(blue, ddai)
	require.NoError(t, fakeClient.Create(ctx, blue))
	createPreparedServingPod(t, ctx, fakeClient, blue, node.Name)

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	for range 1 {
		_, err := r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
		require.NoError(t, err)
	}
	green := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: suffixedKubernetesName(rendered.Name, "-green")}, green))
	assert.Equal(t, preparedBlueGreenArmed, green.Annotations[preparedRolloutPairInitializedAnnotation])
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, node.Name, key))
}

func TestPreparedMissingActiveBlueChildRecoversOntoGreenBeforeRecreation(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	key := preparedRolloutNodeLabelKey(ddai)
	node.Labels[key] = rolloutSlotBlue
	require.NoError(t, fakeClient.Update(ctx, node))
	green := rendered.DeepCopy()
	green.Name = suffixedKubernetesName(rendered.Name, "-green")
	green.Spec.Selector = green.Spec.Selector.DeepCopy()
	green.Spec.Selector.MatchLabels[kubernetes.AppKubernetesInstanceLabelKey] = "agent-green"
	green.Spec.Template.Labels[kubernetes.AppKubernetesInstanceLabelKey] = "agent-green"
	green.UID = "green-uid"
	green.Annotations = map[string]string{
		preparedRolloutPairInitializedAnnotation: preparedBlueGreenArmed,
		preparedRolloutActiveSlotAnnotation:      rolloutSlotBlue,
		preparedRolloutRevisionAnnotation:        "test-revision",
	}
	green.Spec.Template.Annotations = map[string]string{
		preparedRolloutModeAnnotation:     preparedBlueGreenMode,
		preparedRolloutRevisionAnnotation: "test-revision",
	}
	setPreparedTestController(green, ddai)
	require.NoError(t, fakeClient.Create(ctx, green))
	createPreparedServingPod(t, ctx, fakeClient, green, node.Name)

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	for range 5 {
		_, err := r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
		require.NoError(t, err)
	}
	blue := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: rendered.Name}, blue))
	assert.Equal(t, preparedBlueGreenArmed, blue.Annotations[preparedRolloutPairInitializedAnnotation])
	assert.Equal(t, rolloutSlotGreen, nodeState(t, ctx, fakeClient, node.Name, key))
}

func TestPreparedMissingQueuedTargetRecoversBeforeApplyingNewRevision(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	key := preparedRolloutNodeLabelKey(ddai)
	node.Labels[key] = rolloutPendingValue(rolloutSlotGreen)
	require.NoError(t, fakeClient.Update(ctx, node))
	blue := rendered.DeepCopy()
	blue.UID = "blue-uid"
	blue.Annotations = map[string]string{
		preparedRolloutPairInitializedAnnotation: preparedBlueGreenArmed,
		preparedRolloutActiveSlotAnnotation:      rolloutSlotBlue,
		preparedRolloutTargetSlotAnnotation:      rolloutSlotGreen,
		preparedRolloutTargetRevisionAnnotation:  "lost-target-revision",
		preparedRolloutRevisionAnnotation:        "survivor-revision",
	}
	blue.Spec.Template.Annotations = map[string]string{
		preparedRolloutModeAnnotation:     preparedBlueGreenMode,
		preparedRolloutRevisionAnnotation: "survivor-revision",
	}
	setPreparedTestController(blue, ddai)
	require.NoError(t, fakeClient.Create(ctx, blue))
	createPreparedServingPod(t, ctx, fakeClient, blue, node.Name)

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	for range 5 {
		_, err := r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
		require.NoError(t, err)
	}
	green := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: suffixedKubernetesName(rendered.Name, "-green")}, green))
	currentBlue := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: rendered.Name}, currentBlue))
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, node.Name, key))
	assert.Equal(t, rolloutSlotBlue, currentBlue.Annotations[preparedRolloutActiveSlotAnnotation])
	assert.Empty(t, currentBlue.Annotations[preparedRolloutTargetSlotAnnotation])
	assert.NotEqual(t, "lost-target-revision", green.Annotations[preparedRolloutRevisionAnnotation])
}

func TestPreparedMissingChildRepairsStaleUnreadySurvivorBeforeRecreation(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	key := preparedRolloutNodeLabelKey(ddai)
	node.Labels[key] = rolloutSlotBlue
	require.NoError(t, fakeClient.Update(ctx, node))

	blue := rendered.DeepCopy()
	blue.UID = "blue-uid"
	blue.Annotations = map[string]string{
		preparedRolloutPairInitializedAnnotation: preparedBlueGreenArmed,
		preparedRolloutActiveSlotAnnotation:      rolloutSlotBlue,
		preparedRolloutRevisionAnnotation:        "survivor-r2",
	}
	blue.Spec.Template.Annotations = map[string]string{
		preparedRolloutModeAnnotation:     preparedBlueGreenMode,
		preparedRolloutRevisionAnnotation: "survivor-r2",
	}
	setPreparedTestController(blue, ddai)
	require.NoError(t, fakeClient.Create(ctx, blue))
	createPreparedServingPod(t, ctx, fakeClient, blue, node.Name)

	stale := &corev1.Pod{}
	staleKey := types.NamespacedName{Namespace: blue.Namespace, Name: blue.Name + "-pod"}
	require.NoError(t, fakeClient.Get(ctx, staleKey, stale))
	stale.Annotations[preparedRolloutRevisionAnnotation] = "survivor-r1"
	require.NoError(t, fakeClient.Update(ctx, stale))
	stale.Status.Conditions[0].Status = corev1.ConditionFalse
	for i := range stale.Status.ContainerStatuses {
		stale.Status.ContainerStatuses[i].Ready = false
	}
	require.NoError(t, fakeClient.Status().Update(ctx, stale))

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err := r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	assert.True(t, apierrors.IsNotFound(fakeClient.Get(ctx, staleKey, &corev1.Pod{})))
	greenKey := types.NamespacedName{Namespace: rendered.Namespace, Name: suffixedKubernetesName(rendered.Name, "-green")}
	assert.True(t, apierrors.IsNotFound(fakeClient.Get(ctx, greenKey, &appsv1.DaemonSet{})), "the missing child must wait for a Ready survivor at R2")
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, node.Name, key))

	createPreparedServingPod(t, ctx, fakeClient, blue, node.Name)
	_, err = r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	green := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, greenKey, green))
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, node.Name, key))
}

func TestPreparedMissingChildRecoveryDoesNotWaitForDesiredOnlyNode(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	key := "example.com/slot"
	pair := preparedTestPair()
	pair.green = nil
	pair.blue.Namespace = "default"
	pair.blue.UID = "blue-uid"
	pair.blue.Annotations[preparedRolloutActiveSlotAnnotation] = rolloutSlotBlue
	pair.blue.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"slot": rolloutSlotBlue}}
	pair.blue.Spec.Template.Labels = map[string]string{"slot": rolloutSlotBlue}
	pair.blue.Spec.Template.Spec.NodeSelector = map[string]string{"pool": "old"}
	addPreparedSlotAffinity(&pair.blue.Spec.Template.Spec, key, rolloutSlotBlue, true)

	served := preparedNode("served", key, rolloutSlotBlue)
	served.Labels["pool"] = "old"
	desiredOnly := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "new", Labels: map[string]string{"pool": "new"}}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pair.blue.DeepCopy(), served.DeepCopy(), desiredOnly.DeepCopy()).Build()
	createPreparedServingPod(t, ctx, fakeClient, pair.blue, served.Name)
	r := &Reconciler{client: fakeClient}
	base := &appsv1.DaemonSet{Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{Name: "agent"}},
	}}}}

	recovered, changed, err := r.recoverMissingPreparedSlot(ctx, base, pair, key)
	require.NoError(t, err)
	assert.True(t, recovered, "the missing desired slot must be recreated before bootstrapping nodes excluded by the survivor template")
	assert.False(t, changed)
}

func TestPreparedSteadyPairRefreshesInactiveBootstrapSlot(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	key := preparedRolloutNodeLabelKey(ddai)
	node.Labels[key] = rolloutSlotGreen
	require.NoError(t, fakeClient.Update(ctx, node))
	base := rendered.DeepCopy()
	require.NoError(t, prepareAgentTemplate(base))
	base.Spec.Template.Annotations[preparedRolloutArmedAnnotation] = preparedBlueGreenArmed
	base.Spec.Template.Annotations[preparedRolloutSchemaAnnotation] = preparedRolloutSchemaVersion
	controllercommon.FinalizeAppArmorProfile(&base.Spec.Template, kubernetes.PlatformInfo{})
	desiredRevision, err := comparison.GenerateMD5ForSpec(base.Spec.Template)
	require.NoError(t, err)
	blue, err := preparedSlotDaemonSet(base, rolloutSlotBlue, key, "failed-revision", true)
	require.NoError(t, err)
	green, err := preparedSlotDaemonSet(base, rolloutSlotGreen, key, desiredRevision, false)
	require.NoError(t, err)
	blue.Spec.Template.Spec.Containers[0].Image = "agent:failed-target"
	for _, daemonSet := range []*appsv1.DaemonSet{blue, green} {
		setPreparedTestController(daemonSet, ddai)
		daemonSet.Annotations[preparedRolloutPairInitializedAnnotation] = preparedBlueGreenArmed
		daemonSet.Annotations[preparedRolloutActiveSlotAnnotation] = rolloutSlotGreen
		require.NoError(t, fakeClient.Create(ctx, daemonSet))
		setPreparedDaemonSetStatus(t, ctx, fakeClient, daemonSet.Namespace, daemonSet.Name, 0, 0, 0, 0)
	}

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err = r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	refreshedBlue := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(blue), refreshedBlue))
	assert.Equal(t, rendered.Spec.Template.Spec.Containers[0].Image, refreshedBlue.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, desiredRevision, refreshedBlue.Annotations[preparedRolloutRevisionAnnotation])
}

func TestPreparedSteadyPairHandsOffStaleOnDeletePod(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	key := preparedRolloutNodeLabelKey(ddai)
	node.Labels[key] = rolloutSlotBlue
	require.NoError(t, fakeClient.Update(ctx, node))
	base := rendered.DeepCopy()
	require.NoError(t, prepareAgentTemplate(base))
	base.Spec.Template.Annotations[preparedRolloutArmedAnnotation] = preparedBlueGreenArmed
	base.Spec.Template.Annotations[preparedRolloutSchemaAnnotation] = preparedRolloutSchemaVersion
	controllercommon.FinalizeAppArmorProfile(&base.Spec.Template, kubernetes.PlatformInfo{})
	desiredRevision, err := comparison.GenerateMD5ForSpec(base.Spec.Template)
	require.NoError(t, err)
	blue, err := preparedSlotDaemonSet(base, rolloutSlotBlue, key, desiredRevision, true)
	require.NoError(t, err)
	green, err := preparedSlotDaemonSet(base, rolloutSlotGreen, key, desiredRevision, false)
	require.NoError(t, err)
	for slot, daemonSet := range map[string]*appsv1.DaemonSet{rolloutSlotBlue: blue, rolloutSlotGreen: green} {
		daemonSet.UID = types.UID(slot + "-uid")
		setPreparedTestController(daemonSet, ddai)
		daemonSet.Annotations[preparedRolloutPairInitializedAnnotation] = preparedBlueGreenArmed
		daemonSet.Annotations[preparedRolloutActiveSlotAnnotation] = rolloutSlotBlue
		require.NoError(t, fakeClient.Create(ctx, daemonSet))
		setPreparedDaemonSetStatus(t, ctx, fakeClient, daemonSet.Namespace, daemonSet.Name, 1, 1, 1, 0)
	}
	createPreparedServingPod(t, ctx, fakeClient, blue, node.Name)
	stalePod := &corev1.Pod{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: blue.Namespace, Name: blue.Name + "-pod"}, stalePod))
	stalePod.Annotations[preparedRolloutRevisionAnnotation] = "stale-bootstrap-revision"
	require.NoError(t, fakeClient.Update(ctx, stalePod))

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err = r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	currentBlue := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(blue), currentBlue))
	assert.Equal(t, rolloutSlotGreen, currentBlue.Annotations[preparedRolloutTargetSlotAnnotation])
	assert.Equal(t, desiredRevision, currentBlue.Annotations[preparedRolloutTargetRevisionAnnotation])
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, node.Name, key), "the stale Pod keeps serving until green prepares")
}

func TestPreparedSteadyPairRecreatesStaleUnreadyOnDeletePod(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	pair := preparedTestPair()
	pair.blue.Namespace = "default"
	pair.blue.UID = "blue-uid"
	pair.blue.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"slot": rolloutSlotBlue}}
	node := preparedNode("node-a", "rollout", rolloutSlotBlue)
	stale := preparedPod("stale", node.Name, false)
	stale.Namespace = pair.blue.Namespace
	stale.Labels = map[string]string{"slot": rolloutSlotBlue}
	stale.Annotations[preparedRolloutRevisionAnnotation] = "stale-revision"
	controller := true
	stale.OwnerReferences = []metav1.OwnerReference{{Kind: "DaemonSet", Name: pair.blue.Name, UID: pair.blue.UID, Controller: &controller}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&stale).Build()
	r := &Reconciler{client: fakeClient}

	staleServing, repaired, err := r.reconcileStalePreparedPods(ctx, []corev1.Node{node}, []corev1.Pod{stale}, pair.blue)
	require.NoError(t, err)
	assert.False(t, staleServing)
	assert.True(t, repaired)
	assert.True(t, apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(&stale), &corev1.Pod{})))
}

func TestPreparedObsoleteTargetContinuesAfterHandoffStarts(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	key := preparedRolloutNodeLabelKey(ddai)
	node.Labels[key] = rolloutPendingValue(rolloutSlotGreen)
	require.NoError(t, fakeClient.Update(ctx, node))
	base := rendered.DeepCopy()
	require.NoError(t, prepareAgentTemplate(base))
	base.Spec.Template.Annotations[preparedRolloutArmedAnnotation] = preparedBlueGreenArmed
	blue, err := preparedSlotDaemonSet(base, rolloutSlotBlue, key, "obsolete-revision", true)
	require.NoError(t, err)
	green, err := preparedSlotDaemonSet(base, rolloutSlotGreen, key, "obsolete-revision", false)
	require.NoError(t, err)
	for _, ds := range []*appsv1.DaemonSet{blue, green} {
		setPreparedTestController(ds, ddai)
		ds.Annotations[preparedRolloutPairInitializedAnnotation] = preparedBlueGreenArmed
		ds.Annotations[preparedRolloutActiveSlotAnnotation] = rolloutSlotBlue
		ds.Annotations[preparedRolloutTargetSlotAnnotation] = rolloutSlotGreen
		ds.Annotations[preparedRolloutTargetRevisionAnnotation] = "obsolete-revision"
		require.NoError(t, fakeClient.Create(ctx, ds))
	}

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err = r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	assert.Equal(t, rolloutPendingValue(rolloutSlotGreen), nodeState(t, ctx, fakeClient, node.Name, key))
}

func TestPreparedPairDeletionRemovesBothSlotsAndNodeState(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	key := preparedRolloutNodeLabelKey(ddai)
	candidateKey := preparedRolloutCandidateAnnotationKey(key)
	node.Labels[key] = rolloutPendingValue(rolloutSlotGreen)
	node.Annotations = map[string]string{candidateKey: "green-pod-uid"}
	require.NoError(t, fakeClient.Update(ctx, node))
	blue := rendered.DeepCopy()
	blue.Spec.Template.Annotations = map[string]string{preparedRolloutModeAnnotation: preparedBlueGreenMode}
	green := rendered.DeepCopy()
	green.Name = suffixedKubernetesName(rendered.Name, "-green")
	green.Spec.Template.Annotations = map[string]string{preparedRolloutModeAnnotation: preparedBlueGreenMode}
	setPreparedTestController(blue, ddai)
	setPreparedTestController(green, ddai)
	require.NoError(t, fakeClient.Create(ctx, blue))
	require.NoError(t, fakeClient.Create(ctx, green))

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	handled, err := r.deletePreparedDaemonSetPairIfPresent(ctx, ddai, rendered, &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Error(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: rendered.Name}, &appsv1.DaemonSet{}))
	assert.Error(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: green.Name}, &appsv1.DaemonSet{}))
	pendingNode := &corev1.Node{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, pendingNode))
	assert.Contains(t, pendingNode.Labels, key)
	pendingDDAI := &datadoghqv1alpha1.DatadogAgentInternal{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(ddai), pendingDDAI))
	assert.Equal(t, preparedBlueGreenArmed, pendingDDAI.Annotations[preparedRolloutCleanupAnnotation])

	handled, err = r.deletePreparedDaemonSetPairIfPresent(ctx, pendingDDAI, rendered, &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	assert.True(t, handled)
	cleanNode := &corev1.Node{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, cleanNode))
	assert.NotContains(t, cleanNode.Labels, key)
	assert.NotContains(t, cleanNode.Annotations, candidateKey)
	cleanDDAI := &datadoghqv1alpha1.DatadogAgentInternal{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(ddai), cleanDDAI))
	assert.NotContains(t, cleanDDAI.Annotations, preparedRolloutCleanupAnnotation)
}

func TestPreparedDisableConvergesToConventionalBlue(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	base := rendered.DeepCopy()
	require.NoError(t, prepareAgentTemplate(base))
	key := preparedRolloutNodeLabelKey(ddai)
	blue, err := preparedSlotDaemonSet(base, rolloutSlotBlue, key, "revision", true)
	require.NoError(t, err)
	green, err := preparedSlotDaemonSet(base, rolloutSlotGreen, key, "revision", false)
	require.NoError(t, err)
	for _, ds := range []*appsv1.DaemonSet{blue, green} {
		setPreparedTestController(ds, ddai)
		ds.Annotations[preparedRolloutPairInitializedAnnotation] = preparedBlueGreenArmed
		ds.Annotations[preparedRolloutActiveSlotAnnotation] = rolloutSlotBlue
		require.NoError(t, fakeClient.Create(ctx, ds))
	}
	node.Labels[key] = rolloutSlotBlue
	require.NoError(t, fakeClient.Update(ctx, node))

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err = r.reconcilePreparedDisable(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	assert.True(t, apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(green), &appsv1.DaemonSet{})))
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(blue), &appsv1.DaemonSet{}), "the serving blue DaemonSet must remain")

	_, err = r.reconcilePreparedDisable(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	cleared := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(blue), cleared))
	assert.NotContains(t, cleared.Annotations, preparedRolloutPairInitializedAnnotation)
	assert.NotContains(t, cleared.Annotations, preparedRolloutRevisionAnnotation)
	assert.Empty(t, nodeState(t, ctx, fakeClient, node.Name, key))

	_, err = r.reconcilePreparedDisable(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	conventional := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(blue), conventional))
	assert.False(t, preparedDaemonSetInitialized(conventional))
	assert.Equal(t, appsv1.RollingUpdateDaemonSetStrategyType, conventional.Spec.UpdateStrategy.Type)
}

func TestPreparedDisableKeepsGreenWhileItIsAuthoritative(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, _, fakeClient, scheme := preparedLifecycleFixture(t)
	base := rendered.DeepCopy()
	require.NoError(t, prepareAgentTemplate(base))
	key := preparedRolloutNodeLabelKey(ddai)
	blue, err := preparedSlotDaemonSet(base, rolloutSlotBlue, key, "revision", true)
	require.NoError(t, err)
	green, err := preparedSlotDaemonSet(base, rolloutSlotGreen, key, "revision", false)
	require.NoError(t, err)
	for _, ds := range []*appsv1.DaemonSet{blue, green} {
		setPreparedTestController(ds, ddai)
		ds.Annotations[preparedRolloutPairInitializedAnnotation] = preparedBlueGreenArmed
		ds.Annotations[preparedRolloutActiveSlotAnnotation] = rolloutSlotGreen
		require.NoError(t, fakeClient.Create(ctx, ds))
	}

	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err = r.reconcilePreparedDisable(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(green), &appsv1.DaemonSet{}), "green must remain until a prepared handoff makes blue authoritative")
}

func TestPreparedCleanupResumesAfterBothDaemonSetsDisappear(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, node, fakeClient, scheme := preparedLifecycleFixture(t)
	key := preparedRolloutNodeLabelKey(ddai)
	candidateKey := preparedRolloutCandidateAnnotationKey(key)
	ddai.Annotations = map[string]string{preparedRolloutCleanupAnnotation: preparedBlueGreenArmed}
	require.NoError(t, fakeClient.Update(ctx, ddai))
	node.Labels[key] = rolloutPendingValue(rolloutSlotGreen)
	node.Annotations = map[string]string{candidateKey: "deleted-target-uid"}
	require.NoError(t, fakeClient.Update(ctx, node))
	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)

	handled, err := r.deletePreparedDaemonSetPairIfPresent(ctx, ddai, rendered, &datadoghqv1alpha1.DatadogAgentInternalStatus{})
	require.NoError(t, err)
	assert.True(t, handled)
	cleanNode := &corev1.Node{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, cleanNode))
	assert.NotContains(t, cleanNode.Labels, key)
	assert.NotContains(t, cleanNode.Annotations, candidateKey)
	cleanDDAI := &datadoghqv1alpha1.DatadogAgentInternal{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(ddai), cleanDDAI))
	assert.NotContains(t, cleanDDAI.Annotations, preparedRolloutCleanupAnnotation)
}

func TestPreparedCleanupIntentKeepsNativePathAfterChildrenDisappear(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, _, fakeClient, scheme := preparedLifecycleFixture(t)
	ddai.Annotations = map[string]string{preparedRolloutCleanupAnnotation: preparedBlueGreenArmed}
	require.NoError(t, fakeClient.Update(ctx, ddai))
	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)

	initialized, err := r.preparedBlueGreenInitialized(ctx, ddai, rendered)
	require.NoError(t, err)
	assert.True(t, initialized, "persisted cleanup must not fall back to the EDS branch between child deletion and node-label cleanup")
}

func TestPreparedPairRejectsUnownedGreenNameCollision(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, _, fakeClient, scheme := preparedLifecycleFixture(t)
	collision := rendered.DeepCopy()
	collision.Name = suffixedKubernetesName(rendered.Name, "-green")
	collision.Spec.Template.Annotations = map[string]string{preparedRolloutModeAnnotation: preparedBlueGreenMode}
	require.NoError(t, fakeClient.Create(ctx, collision))
	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)

	_, err := r.getPreparedPair(ctx, ddai, rendered)
	require.ErrorContains(t, err, "name collision")
	stillThere := &appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(collision), stillThere))
}

func TestPreparedPairRejectsNodeAgentNameChange(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, _, fakeClient, scheme := preparedLifecycleFixture(t)
	blue := rendered.DeepCopy()
	blue.Spec.Template.Annotations = map[string]string{preparedRolloutModeAnnotation: preparedBlueGreenMode}
	setPreparedTestController(blue, ddai)
	require.NoError(t, fakeClient.Create(ctx, blue))

	renamed := rendered.DeepCopy()
	renamed.Name = "renamed-agent"
	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err := r.getPreparedPair(ctx, ddai, renamed)
	require.ErrorContains(t, err, "name cannot change after initialization")
	assert.Error(t, fakeClient.Get(ctx, client.ObjectKey{Namespace: renamed.Namespace, Name: renamed.Name}, &appsv1.DaemonSet{}))
}

func TestPreparedIdentitySurvivesTemplateModeAnnotationRemoval(t *testing.T) {
	ctx := context.Background()
	ddai, rendered, _, fakeClient, scheme := preparedLifecycleFixture(t)
	blue := rendered.DeepCopy()
	blue.Annotations = map[string]string{
		preparedRolloutPairInitializedAnnotation: preparedBlueGreenArmed,
		preparedRolloutRevisionAnnotation:        "persisted-revision",
	}
	setPreparedTestController(blue, ddai)
	require.NoError(t, fakeClient.Create(ctx, blue))
	r := NewReconciler(ReconcilerOptions{}, fakeClient, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)

	initialized, err := r.preparedBlueGreenInitialized(ctx, ddai, rendered)
	require.NoError(t, err)
	assert.True(t, initialized)
	renamed := rendered.DeepCopy()
	renamed.Name = "renamed-agent"
	_, err = r.getPreparedPair(ctx, ddai, renamed)
	require.ErrorContains(t, err, "name cannot change after initialization")
}

func TestPreparedContainerTopologyCannotChangeAfterArming(t *testing.T) {
	current := corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent"}, {Name: "trace-agent"}}}}
	desired := current.DeepCopy()
	desired.Spec.Containers[0].Image = "agent:new"
	desired.Spec.Containers[0].Resources.Requests = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")}
	assert.NoError(t, validatePreparedContainerTopology(current, *desired), "image and resource updates preserve lock ownership")

	desired.Spec.Containers = append(desired.Spec.Containers, corev1.Container{Name: "system-probe"})
	require.ErrorContains(t, validatePreparedContainerTopology(current, *desired), "cannot add or remove Agent containers")
}

func TestPreparedInitializedPairRejectsImmutableSelectorChange(t *testing.T) {
	pair := preparedTestPair()
	for _, ds := range []*appsv1.DaemonSet{pair.blue, pair.green} {
		ds.Annotations[preparedRolloutPairInitializedAnnotation] = preparedBlueGreenArmed
		ds.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"slot": ds.Name}}
	}
	blueDesired := pair.blue.DeepCopy()
	greenDesired := pair.green.DeepCopy()
	greenDesired.Spec.Selector.MatchLabels["new-selector"] = "unsafe"

	require.ErrorContains(t, validatePreparedPairSelectors(pair, blueDesired, greenDesired), "keeping the serving pair unchanged")
	assert.NotContains(t, pair.green.Spec.Selector.MatchLabels, "new-selector")
}

func TestPreparedSlotDaemonSetsHaveDistinctSelectors(t *testing.T) {
	base := preparedTestDaemonSet(true)
	base.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{
		kubernetes.AppKubernetesInstanceLabelKey: "agent",
		"component":                              "agent",
	}}
	base.Spec.Template.Labels[kubernetes.AppKubernetesInstanceLabelKey] = "agent"

	blue, err := preparedSlotDaemonSet(base, rolloutSlotBlue, "example.com/slot", "revision", true)
	require.NoError(t, err)
	green, err := preparedSlotDaemonSet(base, rolloutSlotGreen, "example.com/slot", "revision", false)
	require.NoError(t, err)

	assert.Equal(t, "agent", blue.Name)
	assert.Equal(t, "agent-green", green.Name)
	assert.NotEqual(t, blue.Spec.Selector, green.Spec.Selector)
	assert.Equal(t, appsv1.OnDeleteDaemonSetStrategyType, blue.Spec.UpdateStrategy.Type)
	assert.Equal(t, appsv1.OnDeleteDaemonSetStrategyType, green.Spec.UpdateStrategy.Type)
	assert.Equal(t, "revision", blue.Spec.Template.Annotations[preparedRolloutRevisionAnnotation])
	assert.Equal(t, "revision", green.Spec.Template.Annotations[preparedRolloutRevisionAnnotation])
	assert.True(t, slotAffinityAllows(blue, rolloutSlotBlue))
	assert.True(t, slotAffinityAllowsUnlabeled(blue, "example.com/slot"))
	assert.True(t, slotAffinityAllows(blue, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen)))
	assert.False(t, slotAffinityAllows(blue, rolloutSlotGreen))
	assert.True(t, slotAffinityAllows(green, rolloutSlotGreen))
	assert.False(t, slotAffinityAllowsUnlabeled(green, "example.com/slot"))
	assert.True(t, slotAffinityAllows(green, rolloutPendingValue(rolloutSlotGreen)))
	for i := range blue.Spec.Template.Spec.Containers {
		assert.Nil(t, containerEnv(&blue.Spec.Template.Spec.Containers[i], coreAgentCmdPortEnv))
	}
	for i := range green.Spec.Template.Spec.Containers {
		env := containerEnv(&green.Spec.Template.Spec.Containers[i], coreAgentCmdPortEnv)
		require.NotNil(t, env)
		assert.Equal(t, fmt.Sprint(greenCoreAgentCmdPort), env.Value)
	}
}

func TestPreparedGreenCoreUsesDistinctCommandPortAndSharedHealthPort(t *testing.T) {
	base := preparedTestDaemonSet(true)
	base.Spec.Selector.MatchLabels[kubernetes.AppKubernetesInstanceLabelKey] = "agent"
	base.Spec.Template.Labels[kubernetes.AppKubernetesInstanceLabelKey] = "agent"
	require.NoError(t, prepareAgentTemplate(base))

	green, err := preparedSlotDaemonSet(base, rolloutSlotGreen, "example.com/slot", "revision", false)
	require.NoError(t, err)
	core := &green.Spec.Template.Spec.Containers[0]
	require.Equal(t, string(apicommon.CoreAgentContainerName), core.Name)
	assert.Equal(t, fmt.Sprint(greenCoreAgentCmdPort), containerEnv(core, coreAgentCmdPortEnv).Value)
	require.NotNil(t, core.StartupProbe)
	require.NotNil(t, core.StartupProbe.Exec)
	assert.Contains(t, core.StartupProbe.Exec.Command, fmt.Sprint(constants.DefaultAgentHealthPort))
	require.NotNil(t, core.LivenessProbe)
	require.NotNil(t, core.LivenessProbe.HTTPGet)
	assert.Equal(t, constants.DefaultAgentHealthPort, core.LivenessProbe.HTTPGet.Port.IntVal)
	require.NotNil(t, core.ReadinessProbe)
	require.NotNil(t, core.ReadinessProbe.HTTPGet)
	assert.Equal(t, constants.DefaultAgentHealthPort, core.ReadinessProbe.HTTPGet.Port.IntVal)
}

func TestPreparedUnlabeledFallbackFollowsTheSourceSlot(t *testing.T) {
	key := "example.com/slot"
	base := preparedTestDaemonSet(true)
	base.Spec.Selector.MatchLabels[kubernetes.AppKubernetesInstanceLabelKey] = "agent"
	base.Spec.Template.Labels[kubernetes.AppKubernetesInstanceLabelKey] = "agent"

	blue, err := preparedSlotDaemonSet(base, rolloutSlotBlue, key, "revision", true)
	require.NoError(t, err)
	green, err := preparedSlotDaemonSet(base, rolloutSlotGreen, key, "revision", false)
	require.NoError(t, err)
	assert.True(t, slotAffinityAllowsUnlabeled(blue, key))
	assert.False(t, slotAffinityAllowsUnlabeled(green, key))

	blue, err = preparedSlotDaemonSet(base, rolloutSlotBlue, key, "revision", false)
	require.NoError(t, err)
	green, err = preparedSlotDaemonSet(base, rolloutSlotGreen, key, "revision", true)
	require.NoError(t, err)
	assert.False(t, slotAffinityAllowsUnlabeled(blue, key))
	assert.True(t, slotAffinityAllowsUnlabeled(green, key))
}

func TestPreparedNodeProgressionUsesBudgetAndRealReadiness(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	nodes := []corev1.Node{
		preparedNode("node-a", key, rolloutSlotBlue),
		preparedNode("node-b", key, rolloutSlotBlue),
	}
	objects := []runtime.Object{nodes[0].DeepCopy(), nodes[1].DeepCopy()}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
	r := &Reconciler{client: client}
	pair := preparedTestPair()

	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{
		preparedPod("blue-a", "node-a", true),
		preparedPod("blue-b", "node-b", true),
	}
	changed, err := r.advancePreparedNodes(ctx, nodes, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, client, "node-a", key))
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, client, "node-b", key))

	nodes = currentNodes(t, ctx, client, "node-a", "node-b")
	pods[rolloutSlotGreen] = []corev1.Pod{preparedPod("green-a", "node-a", false)}
	changed, err = r.advancePreparedNodes(ctx, nodes, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, client, "node-a", key))

	nodes = currentNodes(t, ctx, client, "node-a", "node-b")
	changed, err = r.advancePreparedNodes(ctx, nodes, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutPendingValue(rolloutSlotGreen), nodeState(t, ctx, client, "node-a", key))
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, client, "node-b", key))

	nodes = currentNodes(t, ctx, client, "node-a", "node-b")
	pods[rolloutSlotGreen] = []corev1.Pod{preparedPod("green-a", "node-a", true)}
	pods[rolloutSlotBlue] = []corev1.Pod{preparedPod("blue-b", "node-b", true)}
	changed, err = r.advancePreparedNodes(ctx, nodes, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutSlotGreen, nodeState(t, ctx, client, "node-a", key))
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, client, "node-b", key))
}

func TestPreparedUpdateAdmitsTargetOnAlreadyUncoveredNode(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("newly-eligible", key, rolloutSlotBlue)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}

	changed, err := r.advancePreparedNodes(ctx, []corev1.Node{node}, emptyPairPods(), preparedTestPair(), key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, node.Name, key))
}

func TestPreparedUpdateAdmitsTargetBesideUnhealthySource(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	nodes := []corev1.Node{
		preparedNode("unhealthy", key, rolloutSlotBlue),
		preparedNode("healthy", key, rolloutSlotBlue),
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes[0].DeepCopy(), nodes[1].DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{
		preparedPod("blue-unhealthy", nodes[0].Name, false),
		preparedPod("blue-healthy", nodes[1].Name, true),
	}

	changed, err := r.advancePreparedNodes(ctx, nodes, pods, preparedTestPair(), key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, nodes[0].Name, key))
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, nodes[1].Name, key), "existing unavailability must continue to consume the rollout budget")
}

func TestPreparedNodeUsesDeleteFirstFallbackForNodeResources(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pair := preparedTestPair()
	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{preparedPod("blue-a", "node-a", true)}
	pods[rolloutSlotGreen] = []corev1.Pod{unschedulablePreparedPod("green-a", "node-a", "0/1 nodes are available: 1 Insufficient cpu")}

	changed, err := r.advancePreparedNodes(ctx, []corev1.Node{node}, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutPendingValue(rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))
}

func TestPreparedCapacityFallbackRepairsAlreadyUnavailableNodeAtZeroAdditionalCost(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	source := preparedPod("blue-a", node.Name, false)
	source.Spec.Containers = []corev1.Container{{Name: "agent", Image: "agent:v1", ImagePullPolicy: corev1.PullIfNotPresent}}
	target := unschedulablePreparedPod("green-a", node.Name, "0/1 nodes are available: 1 Insufficient memory")
	target.Spec.Containers = []corev1.Container{{Name: "agent", Image: "agent:v1", ImagePullPolicy: corev1.PullIfNotPresent}}
	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{source}
	pods[rolloutSlotGreen] = []corev1.Pod{target}

	changed, err := r.advancePreparedNodes(ctx, []corev1.Node{node}, pods, preparedTestPair(), key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutPendingValue(rolloutSlotGreen), nodeState(t, ctx, fakeClient, node.Name, key))
}

func TestPreparedNodeCapacityFallbackAcceptsAnUnpulledNewImage(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pair := preparedTestPair()
	source := preparedPod("blue-a", "node-a", true)
	source.Spec.Containers = []corev1.Container{{Name: "agent", Image: "agent:v1", ImagePullPolicy: corev1.PullIfNotPresent}}
	target := unschedulablePreparedPod("green-a", "node-a", "0/1 nodes are available: 1 Insufficient memory")
	target.Spec.Containers = []corev1.Container{{Name: "agent", Image: "agent:v2", ImagePullPolicy: corev1.PullAlways}}
	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{source}
	pods[rolloutSlotGreen] = []corev1.Pod{target}

	changed, err := r.advancePreparedNodes(ctx, []corev1.Node{node}, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutPendingValue(rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))
}

func TestPreparedNodeDoesNotDeleteSourceForNonCapacitySchedulingFailure(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{preparedPod("blue-a", "node-a", true)}
	pods[rolloutSlotGreen] = []corev1.Pod{unschedulablePreparedPod("green-a", "node-a", "0/1 nodes are available: 1 node(s) had untolerated taint")}

	changed, err := r.advancePreparedNodes(ctx, []corev1.Node{node}, pods, preparedTestPair(), key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))
}

func TestPreparedCandidateMustBeObservedTwiceWithSameUID(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pair := preparedTestPair()
	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{preparedPod("blue-a", "node-a", true)}
	first := preparedPod("green-a", "node-a", false)
	pods[rolloutSlotGreen] = []corev1.Pod{first}

	changed, err := r.advancePreparedNodes(ctx, []corev1.Node{node}, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))

	replacement := preparedPod("green-b", "node-a", false)
	pods[rolloutSlotGreen] = []corev1.Pod{replacement}
	nodes := currentNodes(t, ctx, fakeClient, "node-a")
	changed, err = r.advancePreparedNodes(ctx, nodes, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))

	nodes = currentNodes(t, ctx, fakeClient, "node-a")
	changed, err = r.advancePreparedNodes(ctx, nodes, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutPendingValue(rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))
}

func TestPreparedOverlappingNodesCannotExceedHandoffBudget(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	nodes := []corev1.Node{
		preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen)),
		preparedNode("node-b", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen)),
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodes[0].DeepCopy(), nodes[1].DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pair := preparedTestPair()
	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{
		preparedPod("blue-a", "node-a", true),
		preparedPod("blue-b", "node-b", true),
	}
	pods[rolloutSlotGreen] = []corev1.Pod{
		preparedPod("green-a", "node-a", false),
		preparedPod("green-b", "node-b", false),
	}

	// First observation records both candidate UIDs without removing a source.
	changed, err := r.advancePreparedNodes(ctx, nodes, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-b", key))

	// Even though both replacements are Prepared, only one source may become
	// ineligible under a maxUnavailable budget of one.
	nodes = currentNodes(t, ctx, fakeClient, "node-a", "node-b")
	changed, err = r.advancePreparedNodes(ctx, nodes, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutPendingValue(rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-b", key))
}

func TestPreparedNodeDoesNotUseCapacityFallbackForStaleTargetRevision(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pods := emptyPairPods()
	stale := unschedulablePreparedPod("green-a", "node-a", "0/1 nodes are available: 1 Insufficient memory")
	stale.Annotations[preparedRolloutRevisionAnnotation] = "previous-revision"
	pods[rolloutSlotGreen] = []corev1.Pod{stale}

	changed, err := r.advancePreparedNodes(ctx, []corev1.Node{node}, pods, preparedTestPair(), key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed, "the stale Pod may be cleaned up, but it must not trigger source removal")
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))
}

func TestPreparedNodeDoesNotDeleteSourceForImagePullFailure(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{preparedPod("blue-a", "node-a", true)}
	failedPull := unschedulablePreparedPod("green-a", "node-a", "")
	failedPull.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}}
	failedPull.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "agent",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason: "ImagePullBackOff",
		}},
	}}
	pods[rolloutSlotGreen] = []corev1.Pod{failedPull}

	changed, err := r.advancePreparedNodes(ctx, []corev1.Node{node}, pods, preparedTestPair(), key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, "node-a", key))
}

func TestPreparedCandidateMustBePristine(t *testing.T) {
	ds := preparedTestPair().green
	pod := preparedPod("candidate", "node-a", true)
	assert.True(t, podPrepared(&pod, ds))
	pod.Status.ContainerStatuses[0].RestartCount = 1
	assert.False(t, podPrepared(&pod, ds), "a restarted gate must not trigger source removal")
	pod.Status.ContainerStatuses[0].RestartCount = 0
	assert.True(t, podReady(&pod, ds))
	assert.True(t, podServingReady(&pod, ds), "a recovered serving Pod may have historical restarts")
	pod.Status.ContainerStatuses[0].State.Running.StartedAt = metav1.Now()
	assert.False(t, podPrepared(&pod, ds), "a restarted gate must remain stable before handoff")
	pod.Status.ContainerStatuses[0].State.Running.StartedAt = metav1.NewTime(time.Now().Add(-2 * preparedRolloutReadySoak))
	pod.Status.ContainerStatuses[0].Started = ptr.To(false)
	assert.True(t, podPrepared(&pod, ds), "a sleeping gate has Started=false until it acquires the component lock")
	pod.Status.ContainerStatuses[0].Started = ptr.To(true)
	pod.Annotations[preparedRolloutRevisionAnnotation] = "previous-revision"
	assert.False(t, podPrepared(&pod, ds), "an old OnDelete Pod must not satisfy the desired revision")
}

func TestPreparedGreenAdoptionAdmitsBothSlotsOnUnlabeledNode(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	base := preparedTestDaemonSet(true)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "new-node", Labels: map[string]string{corev1.LabelOSStable: "linux"}}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{client: fakeClient}

	_, changed, err := r.reconcilePreparedNodeLabels(ctx, base, key, rolloutSlotBlue, rolloutSlotGreen)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, node.Name, key))
}

func TestPreparedBlueTargetLabelsNewNodeForLastHealthyGreenSlot(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	base := preparedTestDaemonSet(true)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "new-node", Labels: map[string]string{corev1.LabelOSStable: "linux"}}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &Reconciler{client: fakeClient}

	_, changed, err := r.reconcilePreparedNodeLabels(ctx, base, key, rolloutSlotGreen, rolloutSlotBlue)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, rolloutSlotGreen, nodeState(t, ctx, fakeClient, node.Name, key),
		"an unproven blue target must not replace the last known-good green slot on a new node")
}

func TestPreparedNodeEligibilityMatchesHardTaints(t *testing.T) {
	template := preparedTestDaemonSet(true).Spec.Template
	custom := corev1.Taint{Key: "dedicated", Value: "other", Effect: corev1.TaintEffectNoSchedule}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{corev1.LabelOSStable: "linux"}}, Spec: corev1.NodeSpec{Taints: []corev1.Taint{custom}}}

	eligible, err := preparedNodeEligible(&template, node)
	require.NoError(t, err)
	assert.False(t, eligible)
	template.Spec.Tolerations = append(template.Spec.Tolerations, corev1.Toleration{Key: custom.Key, Value: custom.Value, Effect: custom.Effect})
	eligible, err = preparedNodeEligible(&template, node)
	require.NoError(t, err)
	assert.True(t, eligible)

	template.Spec.Tolerations = nil
	node.Spec.Taints = []corev1.Taint{{Key: corev1.TaintNodeDiskPressure, Effect: corev1.TaintEffectNoSchedule}}
	eligible, err = preparedNodeEligible(&template, node)
	require.NoError(t, err)
	assert.True(t, eligible, "native DaemonSet Pods receive a disk-pressure toleration automatically")
}

func TestPreparedStaleBootstrapWaitsForOtherSlotBeforeDeletion(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotGreen, rolloutSlotBlue))
	stale := preparedPod("stale-blue", "node-a", true)
	stale.Annotations[preparedRolloutRevisionAnnotation] = "previous-revision"
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy(), &stale).Build()
	r := &Reconciler{client: fakeClient}
	pods := emptyPairPods()
	pods[rolloutSlotBlue] = []corev1.Pod{stale}

	changed, err := r.advancePreparedNodes(ctx, []corev1.Node{node}, pods, preparedTestPair(), key, rolloutSlotBlue, intstr.FromInt(1))
	require.NoError(t, err)
	assert.False(t, changed)
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: stale.Name}, &corev1.Pod{}))

	pods[rolloutSlotGreen] = []corev1.Pod{preparedPod("green", "node-a", false)}
	changed, err = r.advancePreparedNodes(ctx, []corev1.Node{node}, pods, preparedTestPair(), key, rolloutSlotBlue, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Error(t, fakeClient.Get(ctx, types.NamespacedName{Name: stale.Name}, &corev1.Pod{}))
}

func TestUnhealthySteadyNodeBlocksAnotherTransition(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	nodes := []corev1.Node{
		preparedNode("node-a", key, rolloutSlotGreen),
		preparedNode("node-b", key, rolloutSlotBlue),
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(nodes[0].DeepCopy(), nodes[1].DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pods := emptyPairPods()
	pods[rolloutSlotGreen] = []corev1.Pod{preparedPod("green-a", "node-a", false)}
	pods[rolloutSlotBlue] = []corev1.Pod{preparedPod("blue-b", "node-b", true)}

	changed, err := r.advancePreparedNodes(ctx, nodes, pods, preparedTestPair(), key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, "node-b", key))
}

func TestPreparedTargetCanBeAbortedBeforeHandoff(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	key := "example.com/slot"
	nodes := []corev1.Node{
		preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotGreen, rolloutSlotBlue)),
		preparedNode("node-b", key, rolloutSlotGreen),
	}
	pair := preparedTestPair()
	pair.blue.Annotations = map[string]string{
		preparedRolloutTargetSlotAnnotation:     rolloutSlotBlue,
		preparedRolloutTargetRevisionAnnotation: "obsolete",
	}
	objects := []runtime.Object{nodes[0].DeepCopy(), nodes[1].DeepCopy(), pair.blue.DeepCopy(), pair.green.DeepCopy()}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
	r := &Reconciler{client: fakeClient}
	pods := emptyPairPods()
	pods[rolloutSlotGreen] = []corev1.Pod{
		preparedPod("green-a", "node-a", true),
		preparedPod("green-b", "node-b", true),
	}

	aborted, err := r.abortPreparedTargetBeforeHandoff(ctx, nodes, pods, pair, key, rolloutSlotBlue)
	require.NoError(t, err)
	assert.True(t, aborted)
	assert.Equal(t, rolloutSlotGreen, nodeState(t, ctx, fakeClient, "node-a", key))

	blue := appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: pair.blue.Name}, &blue))
	assert.NotContains(t, blue.Annotations, preparedRolloutTargetSlotAnnotation)
	assert.NotContains(t, blue.Annotations, preparedRolloutTargetRevisionAnnotation)
	green := appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: pair.green.Name}, &green))
	assert.NotContains(t, green.Annotations, preparedRolloutTargetSlotAnnotation)
}

func TestPreparedTargetIsNotAbortedAfterHandoffStarts(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutPendingValue(rolloutSlotBlue))
	pair := preparedTestPair()
	pair.blue.Annotations = map[string]string{
		preparedRolloutTargetSlotAnnotation:     rolloutSlotBlue,
		preparedRolloutTargetRevisionAnnotation: "obsolete",
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(node.DeepCopy(), pair.blue.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}

	aborted, err := r.abortPreparedTargetBeforeHandoff(ctx, []corev1.Node{node}, emptyPairPods(), pair, key, rolloutSlotBlue)
	require.NoError(t, err)
	assert.False(t, aborted)
	assert.Equal(t, rolloutPendingValue(rolloutSlotBlue), nodeState(t, ctx, fakeClient, "node-a", key))
}

func TestPreparedFailedTargetOnUncoveredNodeCanBeReplaced(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("newly-eligible", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen))
	pair := preparedTestPair()
	for _, ds := range []*appsv1.DaemonSet{pair.blue, pair.green} {
		ds.Annotations = map[string]string{
			preparedRolloutTargetSlotAnnotation:     rolloutSlotGreen,
			preparedRolloutTargetRevisionAnnotation: "failed-revision",
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(node.DeepCopy(), pair.blue.DeepCopy(), pair.green.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pods := emptyPairPods()
	failed := unschedulablePreparedPod("failed-green", node.Name, "")
	require.NoError(t, fakeClient.Create(ctx, &failed))
	pods[rolloutSlotGreen] = []corev1.Pod{failed}

	aborted, err := r.abortPreparedTargetBeforeHandoff(ctx, []corev1.Node{node}, pods, pair, key, rolloutSlotGreen)
	require.NoError(t, err)
	assert.True(t, aborted, "a corrected revision must not be blocked when neither generation serves the newly eligible node")
	assert.Equal(t, rolloutSlotBlue, nodeState(t, ctx, fakeClient, node.Name, key))

	// Once the corrected target is installed, its OnDelete controller still
	// sees the failed old Pod. Re-admit overlap and prove that the stale Pod is
	// removed even though this newly eligible node never had a source Pod.
	current := currentNodes(t, ctx, fakeClient, node.Name)
	require.NoError(t, r.setPreparedNodeState(ctx, &current[0], key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen)))
	pair.green.Annotations[preparedRolloutRevisionAnnotation] = "corrected-revision"
	current = currentNodes(t, ctx, fakeClient, node.Name)
	changed, err := r.advancePreparedNodes(ctx, current, pods, pair, key, rolloutSlotGreen, intstr.FromInt(1))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.True(t, apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(&failed), &corev1.Pod{})))
}

func TestPreparedTargetServingInTransitionIsNotAborted(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	key := "example.com/slot"
	node := preparedNode("node-a", key, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen))
	pair := preparedTestPair()
	pair.blue.Annotations = map[string]string{
		preparedRolloutTargetSlotAnnotation:     rolloutSlotGreen,
		preparedRolloutTargetRevisionAnnotation: "obsolete",
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(node.DeepCopy(), pair.blue.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	pods := emptyPairPods()
	pods[rolloutSlotGreen] = []corev1.Pod{preparedPod("green", node.Name, true)}

	aborted, err := r.abortPreparedTargetBeforeHandoff(ctx, []corev1.Node{node}, pods, pair, key, rolloutSlotGreen)
	require.NoError(t, err)
	assert.False(t, aborted, "a target which already restored service is a completed runtime handoff")
	assert.Equal(t, rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), nodeState(t, ctx, fakeClient, node.Name, key))
}

func TestPreparedPairStateCanBeReadFromRemainingChild(t *testing.T) {
	pair := preparedTestPair()
	pair.green.Annotations = map[string]string{
		preparedRolloutActiveSlotAnnotation:     rolloutSlotGreen,
		preparedRolloutTargetSlotAnnotation:     rolloutSlotBlue,
		preparedRolloutTargetRevisionAnnotation: "revision-2",
	}
	pair.blue = nil

	active, err := pairActiveSlot(pair)
	require.NoError(t, err)
	assert.Equal(t, rolloutSlotGreen, active)
	target, revision, err := pairTarget(pair)
	require.NoError(t, err)
	assert.Equal(t, rolloutSlotBlue, target)
	assert.Equal(t, "revision-2", revision)
}

func TestPreparedPairStateIsCopiedWhenAChildIsCreatedBeforeInitialization(t *testing.T) {
	pair := preparedTestPair()
	pair.green.Annotations = map[string]string{
		preparedRolloutActiveSlotAnnotation:     rolloutSlotGreen,
		preparedRolloutTargetSlotAnnotation:     rolloutSlotBlue,
		preparedRolloutTargetRevisionAnnotation: "revision-2",
	}
	base := preparedTestDaemonSet(true)
	base.Spec.Selector.MatchLabels[kubernetes.AppKubernetesInstanceLabelKey] = "agent"
	base.Spec.Template.Labels[kubernetes.AppKubernetesInstanceLabelKey] = "agent"
	recreated, err := preparedSlotDaemonSet(base, rolloutSlotBlue, "example.com/slot", "revision-2", false)
	require.NoError(t, err)

	copyPreparedPairState(recreated, pair.green)

	assert.Equal(t, appsv1.OnDeleteDaemonSetStrategyType, recreated.Spec.UpdateStrategy.Type)
	assert.Equal(t, pair.green.Annotations[preparedRolloutActiveSlotAnnotation], recreated.Annotations[preparedRolloutActiveSlotAnnotation])
	assert.Equal(t, pair.green.Annotations[preparedRolloutTargetSlotAnnotation], recreated.Annotations[preparedRolloutTargetSlotAnnotation])
	assert.Equal(t, pair.green.Annotations[preparedRolloutTargetRevisionAnnotation], recreated.Annotations[preparedRolloutTargetRevisionAnnotation])
}

func TestPreparedPairRejectsDivergentPersistentState(t *testing.T) {
	pair := preparedTestPair()
	pair.blue.Annotations = map[string]string{
		preparedRolloutActiveSlotAnnotation:     rolloutSlotBlue,
		preparedRolloutTargetSlotAnnotation:     rolloutSlotGreen,
		preparedRolloutTargetRevisionAnnotation: "revision-2",
	}
	pair.green.Annotations = map[string]string{preparedRolloutActiveSlotAnnotation: rolloutSlotGreen}

	_, err := pairActiveSlot(pair)
	require.ErrorContains(t, err, "active state diverged")
	pair.green.Annotations[preparedRolloutTargetSlotAnnotation] = rolloutSlotBlue
	pair.green.Annotations[preparedRolloutTargetRevisionAnnotation] = "revision-3"
	_, _, err = pairTarget(pair)
	require.ErrorContains(t, err, "target state diverged")
}

func TestPreparedPairAcceptsPartiallyPersistedState(t *testing.T) {
	pair := preparedTestPair()
	pair.blue.Annotations = map[string]string{
		preparedRolloutActiveSlotAnnotation:     rolloutSlotGreen,
		preparedRolloutTargetSlotAnnotation:     rolloutSlotBlue,
		preparedRolloutTargetRevisionAnnotation: "revision-2",
	}

	active, err := pairActiveSlot(pair)
	require.NoError(t, err)
	assert.Equal(t, rolloutSlotGreen, active)
	target, revision, err := pairTarget(pair)
	require.NoError(t, err)
	assert.Equal(t, rolloutSlotBlue, target)
	assert.Equal(t, "revision-2", revision)
}

func TestPreparedPairRepairsPartiallyPersistedState(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	pair := preparedTestPair()
	pair.blue.Namespace = "default"
	pair.green.Namespace = "default"
	pair.blue.Annotations = map[string]string{
		preparedRolloutActiveSlotAnnotation:     rolloutSlotGreen,
		preparedRolloutTargetSlotAnnotation:     rolloutSlotBlue,
		preparedRolloutTargetRevisionAnnotation: "revision-2",
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pair.blue.DeepCopy(), pair.green.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}

	for {
		current := getPreparedTestPair(t, ctx, fakeClient, pair)
		changed, err := r.reconcilePreparedPairState(ctx, current)
		require.NoError(t, err)
		if !changed {
			break
		}
	}
	green := appsv1.DaemonSet{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: pair.green.Name}, &green))
	assert.Equal(t, rolloutSlotGreen, green.Annotations[preparedRolloutActiveSlotAnnotation])
	assert.Equal(t, rolloutSlotBlue, green.Annotations[preparedRolloutTargetSlotAnnotation])
	assert.Equal(t, "revision-2", green.Annotations[preparedRolloutTargetRevisionAnnotation])
}

func TestPreparedPairRecoversWhenCompletionSecondPatchFails(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	pair := preparedTestPair()
	pair.blue.Namespace = "default"
	pair.green.Namespace = "default"
	for _, ds := range []*appsv1.DaemonSet{pair.blue, pair.green} {
		ds.Annotations[preparedRolloutActiveSlotAnnotation] = rolloutSlotGreen
		ds.Annotations[preparedRolloutTargetSlotAnnotation] = rolloutSlotBlue
		ds.Annotations[preparedRolloutTargetRevisionAnnotation] = "revision-2"
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pair.blue.DeepCopy(), pair.green.DeepCopy()).Build()
	failing := &failNthPatchClient{Client: baseClient, failAt: 2}
	r := &Reconciler{client: failing}
	require.ErrorContains(t, r.completePairTarget(ctx, pair, rolloutSlotBlue), "injected patch failure")

	partial := getPreparedTestPair(t, ctx, baseClient, pair)
	_, _, err := pairTarget(partial)
	require.NoError(t, err)
	r.client = baseClient
	for {
		partial = getPreparedTestPair(t, ctx, baseClient, pair)
		changed, repairErr := r.reconcilePreparedPairState(ctx, partial)
		require.NoError(t, repairErr)
		if !changed {
			break
		}
	}
	repaired := getPreparedTestPair(t, ctx, baseClient, pair)
	active, err := pairActiveSlot(repaired)
	require.NoError(t, err)
	assert.Equal(t, rolloutSlotGreen, active)
	target, revision, err := pairTarget(repaired)
	require.NoError(t, err)
	assert.Equal(t, rolloutSlotBlue, target)
	assert.Equal(t, "revision-2", revision)
	require.NoError(t, r.completePairTarget(ctx, repaired, rolloutSlotBlue))
}

func TestPreparedPairRecoversWhenTargetSecondPatchFails(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	pair := preparedTestPair()
	pair.blue.Namespace = "default"
	pair.green.Namespace = "default"
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pair.blue.DeepCopy(), pair.green.DeepCopy()).Build()
	r := &Reconciler{client: &failNthPatchClient{Client: baseClient, failAt: 2}}
	require.ErrorContains(t, r.setPairTarget(ctx, pair, rolloutSlotGreen, "revision-2"), "injected patch failure")

	partial := getPreparedTestPair(t, ctx, baseClient, pair)
	target, revision, err := pairTarget(partial)
	require.NoError(t, err)
	assert.Equal(t, rolloutSlotGreen, target)
	assert.Equal(t, "revision-2", revision)
	r.client = baseClient
	for {
		partial = getPreparedTestPair(t, ctx, baseClient, pair)
		changed, repairErr := r.reconcilePreparedPairState(ctx, partial)
		require.NoError(t, repairErr)
		if !changed {
			break
		}
	}
	repaired := getPreparedTestPair(t, ctx, baseClient, pair)
	assert.Equal(t, rolloutSlotGreen, repaired.green.Annotations[preparedRolloutTargetSlotAnnotation])
	assert.Equal(t, "revision-2", repaired.green.Annotations[preparedRolloutTargetRevisionAnnotation])
}

func TestPreparedIneligibleNodeCleanupRemovesCandidateUID(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := "example.com/slot"
	candidateKey := preparedRolloutCandidateAnnotationKey(key)
	node := preparedNode("node-a", key, rolloutPendingValue(rolloutSlotGreen))
	node.Labels[corev1.LabelOSStable] = "windows"
	node.Annotations = map[string]string{candidateKey: "stale-uid"}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node.DeepCopy()).Build()
	r := &Reconciler{client: fakeClient}
	base := preparedTestDaemonSet(true)

	nodes, changed, err := r.reconcilePreparedNodeLabels(ctx, base, key, rolloutSlotBlue, rolloutSlotGreen)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Empty(t, nodes)
	clean := &corev1.Node{}
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, clean))
	assert.NotContains(t, clean.Labels, key)
	assert.NotContains(t, clean.Annotations, candidateKey)
}

func TestPreparedNodeStateValidationRepairsUnknownAndStaleStates(t *testing.T) {
	assert.True(t, preparedNodeStateValid(rolloutSlotBlue, rolloutSlotBlue, ""))
	assert.False(t, preparedNodeStateValid("corrupt", rolloutSlotBlue, ""))
	assert.False(t, preparedNodeStateValid(rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), rolloutSlotBlue, ""))
	assert.True(t, preparedNodeStateValid(rolloutTransitionValue(rolloutSlotBlue, rolloutSlotGreen), rolloutSlotBlue, rolloutSlotGreen))
	assert.True(t, preparedNodeStateValid(rolloutPendingValue(rolloutSlotGreen), rolloutSlotBlue, rolloutSlotGreen))
	assert.True(t, preparedNodeStateValid(rolloutSlotGreen, rolloutSlotBlue, rolloutSlotGreen))
}

func TestPreparedPodListingRejectsPreviousDaemonSetUID(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	pair := preparedTestPair()
	pair.blue.Namespace = "default"
	pair.blue.UID = "current-uid"
	pair.blue.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"slot": "blue"}}
	pair.green = nil
	controller := true
	current := preparedPod("current", "node-a", true)
	current.Namespace = "default"
	current.Labels = map[string]string{"slot": "blue"}
	current.OwnerReferences = []metav1.OwnerReference{{Kind: "DaemonSet", Name: pair.blue.Name, UID: pair.blue.UID, Controller: &controller}}
	stale := current.DeepCopy()
	stale.Name = "stale"
	stale.OwnerReferences[0].UID = "previous-uid"
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current.DeepCopy(), stale).Build()
	r := &Reconciler{client: fakeClient}

	pods, err := r.listPreparedPairPods(ctx, pair)
	require.NoError(t, err)
	require.Len(t, pods[rolloutSlotBlue], 1)
	assert.Equal(t, "current", pods[rolloutSlotBlue][0].Name)
}

func preparedTestPair() preparedPair {
	container := corev1.Container{Name: "agent"}
	metadata := metav1.ObjectMeta{Annotations: map[string]string{preparedRolloutRevisionAnnotation: "test-revision"}}
	template := corev1.PodTemplateSpec{ObjectMeta: metadata, Spec: corev1.PodSpec{Containers: []corev1.Container{container}}}
	return preparedPair{
		blue:  &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "blue", Annotations: metadata.Annotations}, Spec: appsv1.DaemonSetSpec{Template: *template.DeepCopy()}},
		green: &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "green", Annotations: metadata.Annotations}, Spec: appsv1.DaemonSetSpec{Template: *template.DeepCopy()}},
	}
}

func preparedNode(name, key, value string) corev1.Node {
	return corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{key: value}}}
}

func preparedPod(name, node string, ready bool) corev1.Pod {
	stableSince := metav1.NewTime(time.Now().Add(-2 * preparedRolloutReadySoak))
	conditions := []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, LastTransitionTime: stableSince}}
	if ready {
		conditions[0].Status = corev1.ConditionTrue
	}
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name + "-uid"), Annotations: map[string]string{preparedRolloutRevisionAnnotation: "test-revision"}},
		Spec:       corev1.PodSpec{NodeName: node},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "agent", Started: ptr.To(true), Ready: ready,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: stableSince}},
			}},
			Conditions: conditions,
		},
	}
}

func unschedulablePreparedPod(name, node, message string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{preparedRolloutRevisionAnnotation: "test-revision"}},
		Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchFields: []corev1.NodeSelectorRequirement{{Key: metav1.ObjectNameField, Operator: corev1.NodeSelectorOpIn, Values: []string{node}}},
			}}},
		}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: message,
			}},
		},
	}
}

func ownedPreparedPod(name, node string, ready bool, ds *appsv1.DaemonSet, slot string) corev1.Pod {
	pod := preparedPod(name, node, ready)
	pod.Namespace = ds.Namespace
	pod.Labels = map[string]string{"slot": slot}
	controller := true
	pod.OwnerReferences = []metav1.OwnerReference{{Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: &controller}}
	return pod
}

func createPreparedServingPod(t *testing.T, ctx context.Context, c client.Client, ds *appsv1.DaemonSet, node string) {
	t.Helper()
	pod := preparedPod(ds.Name+"-pod", node, true)
	pod.Annotations[preparedRolloutRevisionAnnotation] = ds.Annotations[preparedRolloutRevisionAnnotation]
	status := pod.Status.ContainerStatuses[0]
	pod.Status.ContainerStatuses = make([]corev1.ContainerStatus, len(ds.Spec.Template.Spec.Containers))
	for i := range ds.Spec.Template.Spec.Containers {
		status.Name = ds.Spec.Template.Spec.Containers[i].Name
		pod.Status.ContainerStatuses[i] = status
	}
	pod.Namespace = ds.Namespace
	pod.Labels = make(map[string]string, len(ds.Spec.Selector.MatchLabels))
	for key, value := range ds.Spec.Selector.MatchLabels {
		pod.Labels[key] = value
	}
	controller := true
	pod.OwnerReferences = []metav1.OwnerReference{{Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: &controller}}
	require.NoError(t, c.Create(ctx, &pod))
}

type failNthPatchClient struct {
	client.Client
	patches int
	failAt  int
}

func (c *failNthPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	c.patches++
	if c.patches == c.failAt {
		return errors.New("injected patch failure")
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func getPreparedTestPair(t *testing.T, ctx context.Context, c client.Client, names preparedPair) preparedPair {
	t.Helper()
	pair := preparedPair{blue: &appsv1.DaemonSet{}, green: &appsv1.DaemonSet{}}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Namespace: names.blue.Namespace, Name: names.blue.Name}, pair.blue))
	require.NoError(t, c.Get(ctx, types.NamespacedName{Namespace: names.green.Namespace, Name: names.green.Name}, pair.green))
	return pair
}

func emptyPairPods() map[string][]corev1.Pod {
	return map[string][]corev1.Pod{rolloutSlotBlue: {}, rolloutSlotGreen: {}}
}

func currentNodes(t *testing.T, ctx context.Context, c client.Client, names ...string) []corev1.Node {
	t.Helper()
	nodes := make([]corev1.Node, 0, len(names))
	for _, name := range names {
		node := corev1.Node{}
		require.NoError(t, c.Get(ctx, types.NamespacedName{Name: name}, &node))
		nodes = append(nodes, node)
	}
	return nodes
}

func nodeState(t *testing.T, ctx context.Context, c client.Client, name, key string) string {
	t.Helper()
	node := corev1.Node{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: name}, &node))
	return node.Labels[key]
}

func preparedLifecycleFixture(t *testing.T) (*datadoghqv1alpha1.DatadogAgentInternal, *appsv1.DaemonSet, *corev1.Node, client.Client, *runtime.Scheme) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{
		TypeMeta:   metav1.TypeMeta{APIVersion: datadoghqv1alpha1.GroupVersion.String(), Kind: "DatadogAgentInternal"},
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default", UID: types.UID("ddai-uid")},
	}
	rendered := preparedTestDaemonSet(true)
	rendered.Spec.Selector.MatchLabels[kubernetes.AppKubernetesInstanceLabelKey] = "agent"
	rendered.Spec.Template.Labels[kubernetes.AppKubernetesInstanceLabelKey] = "agent"
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{corev1.LabelOSStable: string(corev1.Linux)}}}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&appsv1.DaemonSet{}).
		WithObjects(ddai, node).
		Build()
	return ddai, rendered, node, fakeClient, scheme
}

func setPreparedTestController(object metav1.Object, ddai *datadoghqv1alpha1.DatadogAgentInternal) {
	controller := true
	object.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: ddai.APIVersion,
		Kind:       ddai.Kind,
		Name:       ddai.Name,
		UID:        ddai.UID,
		Controller: &controller,
	}})
}

func reconcilePreparedLifecycle(
	t *testing.T,
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	ddai *datadoghqv1alpha1.DatadogAgentInternal,
	rendered *appsv1.DaemonSet,
	status *datadoghqv1alpha1.DatadogAgentInternalStatus,
) {
	t.Helper()
	r := NewReconciler(ReconcilerOptions{}, c, kubernetes.PlatformInfo{}, scheme, record.NewFakeRecorder(100), nil)
	_, err := r.reconcilePreparedDaemonSetPair(ctx, ddai, rendered.DeepCopy(), intstr.FromInt(1), status)
	require.NoError(t, err)
}

func setPreparedDaemonSetStatus(t *testing.T, ctx context.Context, c client.Client, namespace, name string, desired, updated, available, unavailable int32) {
	t.Helper()
	ds := &appsv1.DaemonSet{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, ds))
	ds.Status.ObservedGeneration = ds.Generation
	ds.Status.DesiredNumberScheduled = desired
	ds.Status.UpdatedNumberScheduled = updated
	ds.Status.NumberAvailable = available
	ds.Status.NumberUnavailable = unavailable
	require.NoError(t, c.Status().Update(ctx, ds))
}

func slotAffinityAllows(ds *appsv1.DaemonSet, value string) bool {
	required := ds.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	for _, expression := range required.NodeSelectorTerms[0].MatchExpressions {
		for _, allowed := range expression.Values {
			if allowed == value {
				return true
			}
		}
	}
	return false
}

func slotAffinityAllowsUnlabeled(ds *appsv1.DaemonSet, key string) bool {
	required := ds.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	for _, term := range required.NodeSelectorTerms {
		for _, expression := range term.MatchExpressions {
			if expression.Key == key && expression.Operator == corev1.NodeSelectorOpDoesNotExist {
				return true
			}
		}
	}
	return false
}
