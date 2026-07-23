// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareAgentTemplate(t *testing.T) {
	arm := preparedRolloutDaemonSet()
	require.NoError(t, prepareAgentTemplate(arm, preparedRolloutPhaseArm))
	assert.Equal(t, preparedRolloutPhaseArm, arm.Spec.Template.Annotations[preparedRolloutPhaseAnnotation])
	assert.NotEmpty(t, arm.Spec.Template.Spec.Containers[0].Ports, "arming keeps scheduler-visible ports because it does not overlap Pods")
	for _, container := range arm.Spec.Template.Spec.Containers {
		require.NotNil(t, container.StartupProbe)
		require.NotNil(t, container.StartupProbe.Exec)
		require.NotNil(t, container.ReadinessProbe)
		require.NotNil(t, container.ReadinessProbe.Exec)
		assert.Equal(t, "true", envValue(container.Env, rolloutEnabledEnv))
		assert.Contains(t, envValue(container.Env, rolloutLockPathEnv), container.Name+".lock")
		assert.Contains(t, envValue(container.Env, rolloutStatePathEnv), container.Name+".state")
	}
	assert.Equal(t, "trace-agent", arm.Spec.Template.Spec.Containers[1].Command[0])
	assert.Contains(t, strings.Join(arm.Spec.Template.Spec.Containers[0].LivenessProbe.Exec.Command, " "), "agent health")
	assert.Contains(t, strings.Join(arm.Spec.Template.Spec.Containers[1].LivenessProbe.Exec.Command, " "), "/dev/tcp/127.0.0.1/8126")
	assert.NotContains(t, strings.Join(arm.Spec.Template.Spec.Containers[0].StartupProbe.Exec.Command, " "), "agent health",
		"the sleeping replacement startup probe must not contact the old host-network listener")

	standby := preparedRolloutDaemonSet()
	require.NoError(t, prepareAgentTemplate(standby, preparedRolloutPhaseStandby))
	for _, container := range standby.Spec.Template.Spec.Containers {
		assert.Empty(t, container.Ports)
	}
}

func TestConfigurePreparedRolloutArmsThenStaysInStandby(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	reader := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{apiReader: reader}
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{preparedRolloutAnnotation: "true"}}}
	one := intstr.FromInt(1)

	desired := preparedRolloutDaemonSet()
	phase, err := r.configurePreparedRollout(context.Background(), ddai, desired, one)
	require.NoError(t, err)
	assert.Equal(t, preparedRolloutPhaseArm, phase)
	assert.Equal(t, intstr.FromInt(0), *desired.Spec.UpdateStrategy.RollingUpdate.MaxSurge)

	live := desired.DeepCopy()
	// The API server defaults Pod fields on the live DaemonSet. Phase progress
	// must use the controller-owned desired-template hash, not raw equality.
	live.Spec.Template.Spec.DNSPolicy = corev1.DNSClusterFirst
	live.UID = types.UID("daemonset-uid")
	live.Generation = 2
	live.Status = appsv1.DaemonSetStatus{
		ObservedGeneration:     2,
		DesiredNumberScheduled: 2,
		UpdatedNumberScheduled: 2,
		NumberReady:            2,
		NumberAvailable:        2,
	}
	require.NoError(t, reader.Create(context.Background(), live))

	next := preparedRolloutDaemonSet()
	phase, err = r.configurePreparedRollout(context.Background(), ddai, next, one)
	require.NoError(t, err)
	assert.Equal(t, preparedRolloutPhaseStandby, phase)
	assert.Equal(t, intstr.FromInt(1), *next.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, intstr.FromInt(0), *next.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)

	live.Spec = next.Spec
	live.Status.NumberReady = 1
	live.Status.NumberAvailable = 1
	require.NoError(t, reader.Update(context.Background(), live))

	afterFailure := preparedRolloutDaemonSet()
	phase, err = r.configurePreparedRollout(context.Background(), ddai, afterFailure, one)
	require.NoError(t, err)
	assert.Equal(t, preparedRolloutPhaseStandby, phase, "a mixed or failed rollout must not oscillate back to arm")
}

func TestConfigurePreparedRolloutRejectsUnsupportedContainerWithoutMutation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	r := &Reconciler{apiReader: fake.NewClientBuilder().WithScheme(scheme).Build()}
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{preparedRolloutAnnotation: "true"}}}
	desired := preparedRolloutDaemonSet()
	desired.Spec.Template.Spec.Containers = append(desired.Spec.Template.Spec.Containers, corev1.Container{Name: "security-agent"})
	original := desired.DeepCopy()

	_, err := r.configurePreparedRollout(context.Background(), ddai, desired, intstr.FromInt(1))
	require.Error(t, err)
	assert.Equal(t, original, desired)
}

func TestConfigurePreparedRolloutRejectsInvalidGates(t *testing.T) {
	scheme := runtime.NewScheme()
	reconciler := &Reconciler{apiReader: fake.NewClientBuilder().WithScheme(scheme).Build()}
	enabled := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{preparedRolloutAnnotation: "true"}}}

	disabled := &datadoghqv1alpha1.DatadogAgentInternal{}
	desired := preparedRolloutDaemonSet()
	original := desired.DeepCopy()
	phase, err := reconciler.configurePreparedRollout(context.Background(), disabled, desired, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Empty(t, phase)
	assert.Equal(t, original, desired)

	_, err = reconciler.configurePreparedRollout(context.Background(), enabled, preparedRolloutDaemonSet(), intstr.FromInt(0))
	require.ErrorContains(t, err, "positive, valid maxUnavailable")

	onDelete := preparedRolloutDaemonSet()
	onDelete.Spec.UpdateStrategy.Type = appsv1.OnDeleteDaemonSetStrategyType
	_, err = reconciler.configurePreparedRollout(context.Background(), enabled, onDelete, intstr.FromInt(1))
	require.ErrorContains(t, err, "RollingUpdate strategy")
}

func TestConfigureArmStrategyInitializesRollingUpdate(t *testing.T) {
	ds := &appsv1.DaemonSet{}
	configureArmStrategy(ds, intstr.FromString("25%"))
	require.NotNil(t, ds.Spec.UpdateStrategy.RollingUpdate)
	assert.Equal(t, intstr.FromInt(0), *ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, intstr.FromString("25%"), *ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)

	previous := ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable
	configureArmStrategy(ds, intstr.FromInt(0))
	assert.Equal(t, previous, ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
}

func TestPrepareAgentTemplateRejectsUnsafeTemplates(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*appsv1.DaemonSet)
		wantErr string
	}{
		{
			name: "host network disabled",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.HostNetwork = false
			},
			wantErr: "hostNetwork=true",
		},
		{
			name: "windows pod os",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.OS = &corev1.PodOS{Name: corev1.Windows}
			},
			wantErr: "Linux-only",
		},
		{
			name: "windows node selector",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.NodeSelector = map[string]string{corev1.LabelOSStable: "windows"}
			},
			wantErr: "Linux-only",
		},
		{
			name: "legacy windows node selector",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.NodeSelector = map[string]string{"beta.kubernetes.io/os": "windows"}
			},
			wantErr: "Linux-only",
		},
		{
			name: "missing trace container",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers = ds.Spec.Template.Spec.Containers[:1]
			},
			wantErr: "exactly agent and trace-agent",
		},
		{
			name: "unsupported container",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[1].Name = "security-agent"
			},
			wantErr: "does not support container",
		},
		{
			name: "duplicate container",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[1].Name = string(apicommon.CoreAgentContainerName)
			},
			wantErr: "duplicate container",
		},
		{
			name: "container lifecycle hook",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Lifecycle = &corev1.Lifecycle{}
			},
			wantErr: "lifecycle hooks",
		},
		{
			name: "container arguments",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Args = []string{"--extra"}
			},
			wantErr: "command arguments",
		},
		{
			name: "nonstandard core command",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Command = []string{"agent", "start"}
			},
			wantErr: "standard agent run command",
		},
		{
			name: "reserved mount path",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "custom", MountPath: preparedRolloutLockDir}}
			},
			wantErr: "reserved name or path",
		},
		{
			name: "reserved mount name",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: preparedRolloutLockVolume, MountPath: "/custom"}}
			},
			wantErr: "reserved name or path",
		},
		{
			name: "unknown trace loader",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[1].Command = []string{"custom-loader"}
			},
			wantErr: "unknown trace-agent loader",
		},
		{
			name: "unexpected init container",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.InitContainers[1].Name = "custom-init"
			},
			wantErr: "does not support init container",
		},
		{
			name: "missing init container",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.InitContainers = ds.Spec.Template.Spec.InitContainers[:1]
			},
			wantErr: "only init-volume and init-config",
		},
		{
			name: "duplicate init container",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.InitContainers[1].Name = string(apicommon.InitVolumeContainerName)
			},
			wantErr: "requires init-volume and init-config",
		},
		{
			name: "init container lifecycle hook",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.InitContainers[0].Lifecycle = &corev1.Lifecycle{}
			},
			wantErr: "ports or lifecycle hooks",
		},
		{
			name: "init container port",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.InitContainers[0].Ports = []corev1.ContainerPort{{ContainerPort: 1234}}
			},
			wantErr: "ports or lifecycle hooks",
		},
		{
			name: "reserved volume name",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: preparedRolloutStateVolume}}
			},
			wantErr: "reserved",
		},
		{
			name: "custom anti-affinity",
			mutate: func(ds *appsv1.DaemonSet) {
				ds.Spec.Template.Spec.Affinity = &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{}}
			},
			wantErr: "custom Pod anti-affinity",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ds := preparedRolloutDaemonSet()
			test.mutate(ds)
			err := prepareAgentTemplate(ds, preparedRolloutPhaseArm)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestConfigurePreparedContainerReplacesRolloutEnvironment(t *testing.T) {
	container := &corev1.Container{
		Name: "agent",
		Env: []corev1.EnvVar{
			{Name: rolloutEnabledEnv, Value: "false"},
			{Name: "KEEP_ME", Value: "value"},
		},
	}

	configurePreparedContainer(container)

	assert.Equal(t, "true", envValue(container.Env, rolloutEnabledEnv))
	assert.Equal(t, "value", envValue(container.Env, "KEEP_ME"))
	count := 0
	for i := range container.Env {
		if container.Env[i].Name == rolloutEnabledEnv {
			count++
		}
	}
	assert.Equal(t, 1, count, "rollout configuration must replace, not duplicate, an existing environment variable")
}

func TestRequeuePreparedArm(t *testing.T) {
	t.Run("polls while arm status is incomplete", func(t *testing.T) {
		result := reconcile.Result{}
		requeuePreparedArm(&result, preparedRolloutPhaseArm)
		assert.Equal(t, time.Second, result.RequeueAfter)
	})

	t.Run("keeps an earlier requeue", func(t *testing.T) {
		result := reconcile.Result{RequeueAfter: 500 * time.Millisecond}
		requeuePreparedArm(&result, preparedRolloutPhaseArm)
		assert.Equal(t, 500*time.Millisecond, result.RequeueAfter)
	})

	t.Run("does not poll once standby starts", func(t *testing.T) {
		result := reconcile.Result{}
		requeuePreparedArm(&result, preparedRolloutPhaseStandby)
		assert.Zero(t, result.RequeueAfter)
	})
}

func TestPodPreparedForHandoff(t *testing.T) {
	pod := preparedReplacementPod()
	assert.True(t, podPreparedForHandoff(pod))
	pod.Status.ContainerStatuses[1].Started = ptr.To(false)
	assert.False(t, podPreparedForHandoff(pod))
	pod.Status.ContainerStatuses[1].Started = ptr.To(true)
	pod.Status.ContainerStatuses[1].RestartCount = 1
	assert.False(t, podPreparedForHandoff(pod))

	tests := []struct {
		name   string
		mutate func(*corev1.Pod)
	}{
		{name: "deleting", mutate: func(p *corev1.Pod) { now := metav1.Now(); p.DeletionTimestamp = &now }},
		{name: "not running", mutate: func(p *corev1.Pod) { p.Status.Phase = corev1.PodPending }},
		{name: "missing init status", mutate: func(p *corev1.Pod) { p.Status.InitContainerStatuses = p.Status.InitContainerStatuses[:1] }},
		{name: "missing container status", mutate: func(p *corev1.Pod) { p.Status.ContainerStatuses = p.Status.ContainerStatuses[:1] }},
		{name: "failed init", mutate: func(p *corev1.Pod) { p.Status.InitContainerStatuses[0].State.Terminated.ExitCode = 1 }},
		{name: "running init", mutate: func(p *corev1.Pod) { p.Status.InitContainerStatuses[0].State.Terminated = nil }},
		{name: "unknown container", mutate: func(p *corev1.Pod) { p.Status.ContainerStatuses[0].Name = "security-agent" }},
		{name: "container stopped", mutate: func(p *corev1.Pod) { p.Status.ContainerStatuses[0].State.Running = nil }},
		{name: "started unknown", mutate: func(p *corev1.Pod) { p.Status.ContainerStatuses[0].Started = nil }},
		{name: "duplicate agent", mutate: func(p *corev1.Pod) { p.Status.ContainerStatuses[1].Name = "agent" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := preparedReplacementPod()
			test.mutate(candidate)
			assert.False(t, podPreparedForHandoff(candidate))
		})
	}
}

func TestReconcilePreparedHandoffReservesThenDeletesOldPod(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))

	ddai := &datadoghqv1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "datadog-agent", UID: "ddai-uid", Annotations: map[string]string{preparedRolloutAnnotation: "true"}},
	}
	ds := preparedRolloutDaemonSet()
	ds.UID = "daemonset-uid"
	ds.Generation = 2
	ds.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: datadoghqv1alpha1.GroupVersion.String(),
		Kind:       "DatadogAgentInternal",
		Name:       ddai.Name,
		UID:        ddai.UID,
		Controller: ptr.To(true),
	}}
	require.NoError(t, prepareAgentTemplate(ds, preparedRolloutPhaseStandby))
	require.True(t, configureResourceFallback(ds, intstr.FromInt(1)))
	ds.Status = appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 1}

	old := readyPod("old", "old-uid", "node-a", "old-revision", time.Now().Add(-time.Minute))
	old.Namespace = ds.Namespace
	old.Labels["app"] = "agent"
	old.OwnerReferences = []metav1.OwnerReference{daemonSetOwner(ds)}
	replacement := preparedReplacementPod()
	replacement.ObjectMeta = metav1.ObjectMeta{
		Name:      "new",
		Namespace: ds.Namespace,
		UID:       "new-uid",
		Labels: map[string]string{
			"app":                                 "agent",
			appsv1.DefaultDaemonSetUniqueLabelKey: "new-revision",
		},
		OwnerReferences: []metav1.OwnerReference{daemonSetOwner(ds)},
	}
	replacement.Spec.NodeName = "node-a"
	revision := controllerRevisionForTemplate(t, ds, "new-revision")

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ddai, ds, old, replacement, revision).Build()
	r := &Reconciler{client: c, apiReader: c}
	result, err := r.reconcilePreparedHandoff(context.Background(), ddai, ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)

	err = c.Get(context.Background(), client.ObjectKeyFromObject(old), &corev1.Pod{})
	assert.True(t, apierrors.IsNotFound(err), "the exact old UID should be deleted after both replacement processes report Prepared")
	updated := &corev1.Pod{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(replacement), updated))
	assert.Equal(t, string(old.UID), updated.Annotations[resourceFallbackOldPodAnnotation])
}

func TestReconcilePreparedHandoffResumesReservedCandidateAtBudget(t *testing.T) {
	fixture := newPreparedHandoffFixture(t)
	fixture.replacement.Annotations = map[string]string{resourceFallbackOldPodAnnotation: string(fixture.old.UID)}
	base := fixture.client(t, true, true, true)
	r := &Reconciler{client: base, apiReader: base}

	result, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	err = base.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{})
	assert.True(t, apierrors.IsNotFound(err), "a restart after reserving the full budget must resume and delete the exact old Pod")
}

func TestReconcilePreparedHandoffFindsReservedCandidateAfterUnreservedAtBudget(t *testing.T) {
	fixture := newPreparedHandoffFixture(t)
	fixture.old.Spec.NodeName = "node-b"
	fixture.replacement.Spec.NodeName = "node-b"
	fixture.replacement.Annotations = map[string]string{resourceFallbackOldPodAnnotation: string(fixture.old.UID)}

	oldA := fixture.old.DeepCopy()
	oldA.Name = "old-a"
	oldA.UID = "old-a-uid"
	oldA.Spec.NodeName = "node-a"
	replacementA := fixture.replacement.DeepCopy()
	replacementA.Name = "new-a"
	replacementA.UID = "new-a-uid"
	replacementA.Spec.NodeName = "node-a"
	replacementA.Annotations = nil

	base := fixture.client(t, true, true, true)
	require.NoError(t, base.Create(context.Background(), oldA))
	require.NoError(t, base.Create(context.Background(), replacementA))
	r := &Reconciler{client: base, apiReader: base}

	result, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	err = base.Get(context.Background(), client.ObjectKeyFromObject(fixture.old), &corev1.Pod{})
	assert.True(t, apierrors.IsNotFound(err), "the reserved handoff must resume even when an unreserved node sorts first")
	assertOldPodStillExists(t, base, oldA)
}

func TestReconcilePreparedHandoffReleasesStaleReservation(t *testing.T) {
	fixture := newPreparedHandoffFixture(t)
	fixture.replacement.Annotations = map[string]string{resourceFallbackOldPodAnnotation: string(fixture.old.UID)}
	fixture.replacement.Status.ContainerStatuses[0].RestartCount = 1
	base := fixture.client(t, true, true, true)
	r := &Reconciler{client: base, apiReader: base}

	result, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	updated := &corev1.Pod{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(fixture.replacement), updated))
	assert.Empty(t, updated.Annotations[resourceFallbackOldPodAnnotation], "an ineligible replacement must stop consuming rollout budget")
	assertOldPodStillExists(t, base, fixture.old)
}

func TestReconcilePreparedHandoffReleasesReservationAboveReducedBudget(t *testing.T) {
	fixture := newPreparedHandoffFixture(t)
	fixture.old.Spec.NodeName = "node-a"
	fixture.replacement.Spec.NodeName = "node-a"
	fixture.replacement.Annotations = map[string]string{resourceFallbackOldPodAnnotation: string(fixture.old.UID)}

	oldB := fixture.old.DeepCopy()
	oldB.Name = "old-b"
	oldB.UID = "old-b-uid"
	oldB.Spec.NodeName = "node-b"
	replacementB := fixture.replacement.DeepCopy()
	replacementB.Name = "new-b"
	replacementB.UID = "new-b-uid"
	replacementB.Spec.NodeName = "node-b"
	replacementB.Annotations = map[string]string{resourceFallbackOldPodAnnotation: string(oldB.UID)}

	base := fixture.client(t, true, true, true)
	require.NoError(t, base.Create(context.Background(), oldB))
	require.NoError(t, base.Create(context.Background(), replacementB))
	r := &Reconciler{client: base, apiReader: base}

	result, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	updated := &corev1.Pod{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(replacementB), updated))
	assert.Empty(t, updated.Annotations[resourceFallbackOldPodAnnotation], "one reservation must be released when the budget is reduced")
	assertOldPodStillExists(t, base, fixture.old)
	assertOldPodStillExists(t, base, oldB)
}

func TestReconcilePreparedHandoffFailsClosed(t *testing.T) {
	t.Run("API reader error", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		base := fixture.client(t, true, true, true)
		reader := interceptor.NewClient(base, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("read failed")
			},
		})
		r := &Reconciler{client: base, apiReader: reader}
		_, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.ErrorContains(t, err, "read failed")
	})

	t.Run("foreign DaemonSet", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		fixture.ds.OwnerReferences[0].UID = "other-ddai"
		base := fixture.client(t, true, true, true)
		r := &Reconciler{client: base, apiReader: base}
		result, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, result)
		assertOldPodStillExists(t, base, fixture.old)
	})

	t.Run("missing current revision", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		base := fixture.client(t, false, true, true)
		r := &Reconciler{client: base, apiReader: base}
		result, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, result)
		assertOldPodStillExists(t, base, fixture.old)
	})

	t.Run("revision list error", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		base := fixture.client(t, true, true, true)
		reader := interceptor.NewClient(base, interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*appsv1.ControllerRevisionList); ok {
					return errors.New("revision list failed")
				}
				return c.List(ctx, list, opts...)
			},
		})
		r := &Reconciler{client: base, apiReader: reader}
		_, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.ErrorContains(t, err, "revision list failed")
		assertOldPodStillExists(t, base, fixture.old)
	})

	t.Run("Pod list error", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		base := fixture.client(t, true, true, true)
		reader := interceptor.NewClient(base, interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*corev1.PodList); ok {
					return errors.New("Pod list failed")
				}
				return c.List(ctx, list, opts...)
			},
		})
		r := &Reconciler{client: base, apiReader: reader}
		_, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.ErrorContains(t, err, "Pod list failed")
		assertOldPodStillExists(t, base, fixture.old)
	})

	t.Run("invalid budget", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		base := fixture.client(t, true, true, true)
		r := &Reconciler{client: base, apiReader: base}
		_, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromString("invalid"))
		require.Error(t, err)
		assertOldPodStillExists(t, base, fixture.old)
	})

	t.Run("budget already consumed", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		fixture.ds.Status.NumberUnavailable = 1
		base := fixture.client(t, true, true, true)
		r := &Reconciler{client: base, apiReader: base}
		result, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.NoError(t, err)
		assert.Equal(t, reconcile.Result{}, result)
		assertOldPodStillExists(t, base, fixture.old)
	})

	t.Run("mismatched reservation", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		fixture.replacement.Annotations = map[string]string{resourceFallbackOldPodAnnotation: "another-old-pod"}
		base := fixture.client(t, true, true, true)
		r := &Reconciler{client: base, apiReader: base}
		result, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.NoError(t, err)
		assert.Equal(t, time.Second, result.RequeueAfter)
		updated := &corev1.Pod{}
		require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(fixture.replacement), updated))
		assert.Empty(t, updated.Annotations[resourceFallbackOldPodAnnotation])
		assertOldPodStillExists(t, base, fixture.old)
	})

	t.Run("reservation patch error", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		base := fixture.client(t, true, true, true)
		writer := interceptor.NewClient(base, interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errors.New("patch failed")
			},
		})
		r := &Reconciler{client: writer, apiReader: base}
		_, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(1))
		require.ErrorContains(t, err, "reserve prepared Agent handoff")
		assertOldPodStillExists(t, base, fixture.old)
	})

	t.Run("old Pod delete error", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		fixture.replacement.Annotations = map[string]string{resourceFallbackOldPodAnnotation: string(fixture.old.UID)}
		base := fixture.client(t, true, true, true)
		writer := interceptor.NewClient(base, interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
				return errors.New("delete failed")
			},
		})
		r := &Reconciler{client: writer, apiReader: base}
		_, err := r.reconcilePreparedHandoff(context.Background(), fixture.ddai, fixture.ds, intstr.FromInt(2))
		require.ErrorContains(t, err, "delete old Agent Pod")
		assertOldPodStillExists(t, base, fixture.old)
	})
}

func TestRevalidatePreparedHandoffRejectsStaleState(t *testing.T) {
	t.Run("API reader error", func(t *testing.T) {
		fixture := newPreparedHandoffFixture(t)
		base := fixture.client(t, true, true, true)
		reader := interceptor.NewClient(base, interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("read failed")
			},
		})
		r := &Reconciler{apiReader: reader}
		candidate := fixture.candidate(true)
		got, err := r.revalidatePreparedHandoff(context.Background(), fixture.ds, candidate, "new-revision")
		require.ErrorContains(t, err, "read failed")
		assert.Nil(t, got)
	})

	tests := []struct {
		name               string
		mutateExpected     func(*appsv1.DaemonSet)
		mutateCandidate    func(*preparedHandoffCandidate)
		mutateObjects      func(*preparedHandoffFixture)
		includeReplacement bool
		includeOld         bool
		expectedRevision   string
	}{
		{name: "DaemonSet UID changed", mutateExpected: func(ds *appsv1.DaemonSet) { ds.UID = "stale-daemonset" }, includeReplacement: true, includeOld: true, expectedRevision: "new-revision"},
		{name: "revision changed", includeReplacement: true, includeOld: true, expectedRevision: "other-revision"},
		{name: "replacement disappeared", includeOld: true, expectedRevision: "new-revision"},
		{name: "old Pod disappeared", includeReplacement: true, expectedRevision: "new-revision"},
		{name: "replacement UID changed", mutateCandidate: func(candidate *preparedHandoffCandidate) { candidate.replacement.UID = "stale-replacement" }, includeReplacement: true, includeOld: true, expectedRevision: "new-revision"},
		{name: "reservation changed", mutateObjects: func(fixture *preparedHandoffFixture) {
			fixture.replacement.Annotations = map[string]string{resourceFallbackOldPodAnnotation: "different-old-pod"}
		}, includeReplacement: true, includeOld: true, expectedRevision: "new-revision"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPreparedHandoffFixture(t)
			fixture.replacement.Annotations = map[string]string{resourceFallbackOldPodAnnotation: string(fixture.old.UID)}
			if test.mutateObjects != nil {
				test.mutateObjects(fixture)
			}
			base := fixture.client(t, true, test.includeReplacement, test.includeOld)
			r := &Reconciler{apiReader: base}
			expected := fixture.ds.DeepCopy()
			if test.mutateExpected != nil {
				test.mutateExpected(expected)
			}
			candidate := fixture.candidate(true)
			if test.mutateCandidate != nil {
				test.mutateCandidate(&candidate)
			}
			got, err := r.revalidatePreparedHandoff(context.Background(), expected, candidate, test.expectedRevision)
			require.NoError(t, err)
			assert.Nil(t, got)
		})
	}
}

type preparedHandoffFixture struct {
	scheme      *runtime.Scheme
	ddai        *datadoghqv1alpha1.DatadogAgentInternal
	ds          *appsv1.DaemonSet
	old         *corev1.Pod
	replacement *corev1.Pod
	revision    *appsv1.ControllerRevision
}

func newPreparedHandoffFixture(t *testing.T) *preparedHandoffFixture {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))

	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name: "agent", Namespace: "datadog-agent", UID: "ddai-uid", Annotations: map[string]string{preparedRolloutAnnotation: "true"},
	}}
	ds := preparedRolloutDaemonSet()
	ds.UID = "daemonset-uid"
	ds.Generation = 2
	ds.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: datadoghqv1alpha1.GroupVersion.String(), Kind: "DatadogAgentInternal", Name: ddai.Name, UID: ddai.UID, Controller: ptr.To(true),
	}}
	require.NoError(t, prepareAgentTemplate(ds, preparedRolloutPhaseStandby))
	require.True(t, configureResourceFallback(ds, intstr.FromInt(1)))
	ds.Status = appsv1.DaemonSetStatus{ObservedGeneration: 2, DesiredNumberScheduled: 1}

	old := readyPod("old", "old-uid", "node-a", "old-revision", time.Now().Add(-time.Minute))
	old.Namespace = ds.Namespace
	old.Labels["app"] = "agent"
	old.OwnerReferences = []metav1.OwnerReference{daemonSetOwner(ds)}
	replacement := preparedReplacementPod()
	replacement.ObjectMeta = metav1.ObjectMeta{
		Name: "new", Namespace: ds.Namespace, UID: "new-uid",
		Labels:          map[string]string{"app": "agent", appsv1.DefaultDaemonSetUniqueLabelKey: "new-revision"},
		OwnerReferences: []metav1.OwnerReference{daemonSetOwner(ds)},
	}
	replacement.Spec.NodeName = "node-a"

	return &preparedHandoffFixture{
		scheme: scheme, ddai: ddai, ds: ds, old: old, replacement: replacement,
		revision: controllerRevisionForTemplate(t, ds, "new-revision"),
	}
}

func (f *preparedHandoffFixture) client(t *testing.T, includeRevision, includeReplacement, includeOld bool) client.WithWatch {
	t.Helper()
	objects := []client.Object{f.ddai, f.ds}
	if includeRevision {
		objects = append(objects, f.revision)
	}
	if includeReplacement {
		objects = append(objects, f.replacement)
	}
	if includeOld {
		objects = append(objects, f.old)
	}
	return fake.NewClientBuilder().WithScheme(f.scheme).WithObjects(objects...).Build()
}

func (f *preparedHandoffFixture) candidate(reserved bool) preparedHandoffCandidate {
	return preparedHandoffCandidate{replacement: f.replacement.DeepCopy(), old: f.old.DeepCopy(), nodeName: "node-a", reserved: reserved}
}

func assertOldPodStillExists(t *testing.T, c client.Client, old *corev1.Pod) {
	t.Helper()
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(old), &corev1.Pod{}))
}

func preparedReplacementPod() *corev1.Pod {
	return &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodRunning,
		InitContainerStatuses: []corev1.ContainerStatus{
			{Name: "init-volume", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
			{Name: "init-config", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
		},
		ContainerStatuses: []corev1.ContainerStatus{
			{Name: "agent", Started: ptr.To(true), State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			{Name: "trace-agent", Started: ptr.To(true), State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
		},
	}}
}

func preparedRolloutDaemonSet() *appsv1.DaemonSet {
	one := intstr.FromInt(1)
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent", Namespace: "datadog-agent"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: &one,
					MaxSurge:       &one,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "agent"}},
				Spec: corev1.PodSpec{
					HostNetwork: true,
					InitContainers: []corev1.Container{
						{Name: "init-volume"},
						{Name: "init-config"},
					},
					Containers: []corev1.Container{
						{Name: "agent", Command: []string{"agent", "run"}, Ports: []corev1.ContainerPort{{ContainerPort: 8125}}},
						{Name: "trace-agent", Command: []string{"trace-loader", "/etc/datadog-agent/datadog.yaml", "trace-agent", "--config=/etc/datadog-agent/datadog.yaml"}, Ports: []corev1.ContainerPort{{ContainerPort: 8126}}},
					},
				},
			},
		},
	}
}

func envValue(env []corev1.EnvVar, name string) string {
	for i := range env {
		if env[i].Name == name {
			return env[i].Value
		}
	}
	return ""
}
