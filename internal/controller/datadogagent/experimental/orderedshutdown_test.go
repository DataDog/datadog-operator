// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

const (
	wantSystemProbeWrapper = "exec 9>/var/run/sysprobe/.running.lock && flock -x 9 && exec system-probe --config=/etc/datadog-agent/system-probe.yaml"
	wantPreStop60          = "flock -s -w 60 /var/run/sysprobe/.running.lock true || true"
)

func TestIsOrderedShutdownEnabled(t *testing.T) {
	key := getExperimentalAnnotationKey(ExperimentalOrderedShutdownSubkey)
	assert.Equal(t, "experimental.agent.datadoghq.com/ordered-shutdown", key)

	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{name: "true", annotations: map[string]string{key: "true"}, want: true},
		{name: "TRUE", annotations: map[string]string{key: "TRUE"}, want: true},
		{name: "false", annotations: map[string]string{key: "false"}, want: false},
		{name: "other value", annotations: map[string]string{key: "yes"}, want: false},
		{name: "missing", annotations: map[string]string{"foo": "bar"}, want: false},
		{name: "no annotations", annotations: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsOrderedShutdownEnabled(&metav1.ObjectMeta{Annotations: tt.annotations}))
		})
	}
	assert.False(t, IsOrderedShutdownEnabled(nil))
}

// newOrderedShutdownPodTemplate mimics the node Agent pod template after the
// features ran: system-probe mounts the socket volume read-write, the core
// agent read-only, and the other containers don't mount it.
func newOrderedShutdownPodTemplate() *corev1.PodTemplateSpec {
	socketRW := corev1.VolumeMount{Name: common.SystemProbeSocketVolumeName, MountPath: common.SystemProbeSocketVolumePath}
	socketRO := corev1.VolumeMount{Name: common.SystemProbeSocketVolumeName, MountPath: common.SystemProbeSocketVolumePath, ReadOnly: true}
	return &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: string(apicommon.InitVolumeContainerName), Command: []string{"cp"}},
				{Name: string(apicommon.InitConfigContainerName), Command: []string{"bash"}},
			},
			Containers: []corev1.Container{
				{Name: string(apicommon.CoreAgentContainerName), Command: []string{"agent", "run"}, VolumeMounts: []corev1.VolumeMount{socketRO}},
				{Name: string(apicommon.TraceAgentContainerName), Command: []string{"trace-agent"}},
				{Name: string(apicommon.ProcessAgentContainerName), Command: []string{"process-agent"}},
				{Name: string(apicommon.SystemProbeContainerName), Command: common.DefaultSystemProbeCommand(), VolumeMounts: []corev1.VolumeMount{socketRW}},
			},
			Volumes: []corev1.Volume{
				{Name: common.SystemProbeSocketVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}
}

func getContainer(t *testing.T, spec *corev1.PodSpec, name apicommon.AgentContainerName) *corev1.Container {
	t.Helper()
	for i := range spec.Containers {
		if spec.Containers[i].Name == string(name) {
			return &spec.Containers[i]
		}
	}
	require.Failf(t, "container not found", "%s", name)
	return nil
}

func socketMounts(c *corev1.Container) []corev1.VolumeMount {
	var mounts []corev1.VolumeMount
	for _, m := range c.VolumeMounts {
		if m.Name == common.SystemProbeSocketVolumeName || m.MountPath == common.SystemProbeSocketVolumePath {
			mounts = append(mounts, m)
		}
	}
	return mounts
}

func preStopCommand(c *corev1.Container) []string {
	if c.Lifecycle == nil || c.Lifecycle.PreStop == nil || c.Lifecycle.PreStop.Exec == nil {
		return nil
	}
	return c.Lifecycle.PreStop.Exec.Command
}

// assertWaiter checks that a non-system-probe container waits for the lock.
func assertWaiter(t *testing.T, c *corev1.Container, wantPreStop string) {
	t.Helper()
	assert.Equal(t, []string{"sh", "-c", wantPreStop}, preStopCommand(c), "preStop of %s", c.Name)
	mounts := socketMounts(c)
	require.Len(t, mounts, 1, "socket mounts of %s", c.Name)
	assert.True(t, mounts[0].ReadOnly, "socket mount of %s should be read-only", c.Name)
}

func TestApplyOrderedShutdown(t *testing.T) {
	waiters := []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.TraceAgentContainerName,
		apicommon.ProcessAgentContainerName,
	}

	tests := []struct {
		name string
		// mutate customizes the default pod template before the pass runs.
		mutate func(*corev1.PodTemplateSpec)
		// wantNoChange expects the pass to leave the pod template untouched.
		wantNoChange bool
		check        func(t *testing.T, spec *corev1.PodSpec)
	}{
		{
			name: "no system-probe container",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Containers = tmpl.Spec.Containers[:3]
			},
			wantNoChange: true,
		},
		{
			name: "system-probe command overridden",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Containers[3].Command = []string{"system-probe", "--config=/custom.yaml"}
			},
			wantNoChange: true,
		},
		{
			name: "system-probe args overridden",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Containers[3].Args = []string{"--pidfile=/tmp/sp.pid"}
			},
			wantNoChange: true,
		},
		{
			name: "system-probe mounts the socket volume read-only",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Containers[3].VolumeMounts[0].ReadOnly = true
			},
			wantNoChange: true,
		},
		{
			name: "system-probe mounts another volume at the socket path",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Containers[3].VolumeMounts[0].Name = "custom"
			},
			wantNoChange: true,
		},
		{
			name: "default",
			check: func(t *testing.T, spec *corev1.PodSpec) {
				sp := getContainer(t, spec, apicommon.SystemProbeContainerName)
				assert.Equal(t, []string{"sh", "-c", wantSystemProbeWrapper}, sp.Command)
				assert.Empty(t, sp.Args)
				assert.Nil(t, sp.Lifecycle, "system-probe must not wait on itself")
				mounts := socketMounts(sp)
				require.Len(t, mounts, 1)
				assert.False(t, mounts[0].ReadOnly, "system-probe needs the socket volume read-write")

				for _, name := range waiters {
					assertWaiter(t, getContainer(t, spec, name), wantPreStop60)
				}

				for _, ic := range spec.InitContainers {
					assert.Nil(t, ic.Lifecycle, "init container %s", ic.Name)
					assert.Empty(t, ic.VolumeMounts, "init container %s", ic.Name)
				}

				require.NotNil(t, spec.TerminationGracePeriodSeconds)
				assert.Equal(t, int64(75), *spec.TerminationGracePeriodSeconds)
				assert.Len(t, spec.Volumes, 1)
			},
		},
		{
			name: "socket volume and system-probe mount missing are added",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Volumes = nil
				tmpl.Spec.Containers[3].VolumeMounts = nil
			},
			check: func(t *testing.T, spec *corev1.PodSpec) {
				require.Len(t, spec.Volumes, 1)
				assert.Equal(t, common.SystemProbeSocketVolumeName, spec.Volumes[0].Name)
				require.NotNil(t, spec.Volumes[0].EmptyDir)
				mounts := socketMounts(getContainer(t, spec, apicommon.SystemProbeContainerName))
				require.Len(t, mounts, 1)
				assert.False(t, mounts[0].ReadOnly)
			},
		},
		{
			name: "existing socket volume is kept",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Volumes[0].EmptyDir.Medium = corev1.StorageMediumMemory
			},
			check: func(t *testing.T, spec *corev1.PodSpec) {
				require.Len(t, spec.Volumes, 1)
				assert.Equal(t, corev1.StorageMediumMemory, spec.Volumes[0].EmptyDir.Medium)
			},
		},
		{
			name: "existing read-write mount on a waiter is not duplicated or downgraded",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Containers[2].VolumeMounts = []corev1.VolumeMount{
					{Name: common.SystemProbeSocketVolumeName, MountPath: common.SystemProbeSocketVolumePath},
				}
			},
			check: func(t *testing.T, spec *corev1.PodSpec) {
				mounts := socketMounts(getContainer(t, spec, apicommon.ProcessAgentContainerName))
				require.Len(t, mounts, 1)
				assert.False(t, mounts[0].ReadOnly)
				// The core agent's existing read-only mount isn't duplicated either.
				assertWaiter(t, getContainer(t, spec, apicommon.CoreAgentContainerName), wantPreStop60)
			},
		},
		{
			name: "existing grace of 190 is kept",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.TerminationGracePeriodSeconds = ptr.To[int64](190)
			},
			check: func(t *testing.T, spec *corev1.PodSpec) {
				assert.Equal(t, int64(190), *spec.TerminationGracePeriodSeconds)
				assertWaiter(t, getContainer(t, spec, apicommon.CoreAgentContainerName), wantPreStop60)
			},
		},
		{
			name: "existing grace of 40 is raised to 75",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.TerminationGracePeriodSeconds = ptr.To[int64](40)
			},
			check: func(t *testing.T, spec *corev1.PodSpec) {
				assert.Equal(t, int64(75), *spec.TerminationGracePeriodSeconds)
				assertWaiter(t, getContainer(t, spec, apicommon.CoreAgentContainerName), wantPreStop60)
			},
		},
		{
			name: "existing preStop is not overwritten",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Containers[1].Lifecycle = &corev1.Lifecycle{
					PreStop: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{Command: []string{"custom"}}},
				}
			},
			check: func(t *testing.T, spec *corev1.PodSpec) {
				assert.Equal(t, []string{"custom"}, preStopCommand(getContainer(t, spec, apicommon.TraceAgentContainerName)))
				assertWaiter(t, getContainer(t, spec, apicommon.CoreAgentContainerName), wantPreStop60)
			},
		},
		{
			name: "existing postStart is kept alongside the preStop",
			mutate: func(tmpl *corev1.PodTemplateSpec) {
				tmpl.Spec.Containers[0].Lifecycle = &corev1.Lifecycle{
					PostStart: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{Command: []string{"poststart"}}},
				}
			},
			check: func(t *testing.T, spec *corev1.PodSpec) {
				agent := getContainer(t, spec, apicommon.CoreAgentContainerName)
				require.NotNil(t, agent.Lifecycle.PostStart)
				assert.Equal(t, []string{"poststart"}, agent.Lifecycle.PostStart.Exec.Command)
				assertWaiter(t, agent, wantPreStop60)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := newOrderedShutdownPodTemplate()
			if tt.mutate != nil {
				tt.mutate(tmpl)
			}
			before := tmpl.DeepCopy()

			ApplyOrderedShutdown(logr.Discard(), feature.NewPodTemplateManagers(tmpl))

			if tt.wantNoChange {
				assert.Equal(t, before, tmpl)
				return
			}
			assert.NotEqual(t, before, tmpl)
			tt.check(t, &tmpl.Spec)
		})
	}
}

func TestApplyOrderedShutdownIsIdempotent(t *testing.T) {
	tmpl := newOrderedShutdownPodTemplate()
	ApplyOrderedShutdown(logr.Discard(), feature.NewPodTemplateManagers(tmpl))
	once := tmpl.DeepCopy()
	// The wrapped command is no longer the default, so a second pass is a no-op.
	ApplyOrderedShutdown(logr.Discard(), feature.NewPodTemplateManagers(tmpl))
	assert.Equal(t, once, tmpl)
}
