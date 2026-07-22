// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
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

func TestPodPreparedForHandoff(t *testing.T) {
	pod := preparedReplacementPod()
	assert.True(t, podPreparedForHandoff(pod))
	pod.Status.ContainerStatuses[1].Started = ptr.To(false)
	assert.False(t, podPreparedForHandoff(pod))
	pod.Status.ContainerStatuses[1].Started = ptr.To(true)
	pod.Status.ContainerStatuses[1].RestartCount = 1
	assert.False(t, podPreparedForHandoff(pod))
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
