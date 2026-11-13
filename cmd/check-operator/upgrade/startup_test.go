// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	plugincommon "github.com/DataDog/datadog-operator/pkg/plugin/common"
)

func testCollector(t *testing.T, objects ...client.Object) (*startupCollector, *bytes.Buffer) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	output := &bytes.Buffer{}
	o := &Options{Options: plugincommon.Options{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(), UserNamespace: "test"}, datadogAgentName: "agent"}
	return &startupCollector{options: o, output: output, snapshots: map[types.UID][32]byte{}, attempts: map[string]struct{}{}}, output
}

func readStartupRecords(t *testing.T, output *bytes.Buffer) []startupRecord {
	t.Helper()
	var records []startupRecord
	decoder := json.NewDecoder(output)
	for {
		var record startupRecord
		err := decoder.Decode(&record)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		records = append(records, record)
	}
	return records
}

func TestStartupExecutionAndSnapshots(t *testing.T) {
	c, output := testCollector(t)
	start := metav1.NewTime(time.Unix(100, 0))
	finish := metav1.NewTime(time.Unix(108, 0))
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "agent-pod", Namespace: "test", UID: "pod-1"}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{{Name: "init-config", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{StartedAt: start, FinishedAt: finish, Reason: "Completed"}}}}}}
	require.NoError(t, c.collectPod(pod, "agent"))
	require.NoError(t, c.collectPod(pod, "agent"))
	records := readStartupRecords(t, output)
	require.Len(t, records, 2)
	assert.Equal(t, "agent_startup_snapshot", records[0].Event)
	assert.Equal(t, 8.0, *records[1].DurationSeconds)
	assert.Equal(t, "init_execution", records[1].Phase)
	assert.Equal(t, int32(0), *records[1].ExitCode)
	// A readiness transition produces a new raw snapshot, not another init sample
	// and not a claim about first readiness.
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: finish}}
	require.NoError(t, c.collectPod(pod, "agent"))
	records = readStartupRecords(t, output)
	require.Len(t, records, 1)
	assert.Empty(t, records[0].Phase)
	// A new failed attempt must be retained and must not be combined with the old.
	pod.Status.InitContainerStatuses[0].RestartCount = 1
	pod.Status.InitContainerStatuses[0].State.Terminated.ExitCode = 1
	pod.Status.InitContainerStatuses[0].State.Terminated.StartedAt = finish
	pod.Status.InitContainerStatuses[0].State.Terminated.FinishedAt = metav1.NewTime(time.Unix(111, 0))
	require.NoError(t, c.collectPod(pod, "agent"))
	records = readStartupRecords(t, output)
	require.Len(t, records, 2)
	assert.Equal(t, 3.0, *records[1].DurationSeconds)
	assert.Equal(t, int32(1), records[1].RestartCount)
	assert.Equal(t, int32(1), *records[1].ExitCode)
}

func TestStartupMissingAndInvalidTimestamps(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state corev1.ContainerState
	}{
		{"pending", corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}},
		{"missing", corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}},
		{"backwards", corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{StartedAt: metav1.NewTime(time.Unix(10, 0)), FinishedAt: metav1.NewTime(time.Unix(9, 0))}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, output := testCollector(t)
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "p"}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{{Name: "init-volume", State: tc.state}}}}
			require.NoError(t, c.collectPod(pod, "agent"))
			records := readStartupRecords(t, output)
			require.Len(t, records, 1)
			assert.Nil(t, records[0].DurationSeconds)
		})
	}
}

func TestStartupSelectsDaemonSetOwnersAndProfiles(t *testing.T) {
	controlled := true
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "test"}, Status: v2alpha1.DatadogAgentStatus{Agent: &v2alpha1.DaemonSetStatus{DaemonsetName: "agent"}}}
	ddai := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "profile", Namespace: "test", Labels: map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"}}}
	ddai.Status.Agent = &v2alpha1.DaemonSetStatus{DaemonsetName: "profile"}
	objects := []client.Object{dda, ddai}
	for _, name := range []string{"agent", "profile"} {
		ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test", UID: types.UID(name)}, Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}}}}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test", UID: types.UID(name + "-pod"), Labels: map[string]string{"app": name}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: name, UID: ds.UID, Controller: &controlled}}}}
		wrong := pod.DeepCopy()
		wrong.Name += "-wrong"
		wrong.UID += "-wrong"
		wrong.OwnerReferences[0].UID = "old-daemonset"
		objects = append(objects, ds, pod, wrong)
	}
	c, output := testCollector(t, objects...)
	require.NoError(t, c.collect(context.Background()))
	records := readStartupRecords(t, output)
	require.Len(t, records, 2)
	assert.ElementsMatch(t, []string{"agent", "profile"}, []string{records[0].DaemonSet, records[1].DaemonSet})
}

// A blocked telemetry API call must not hold up an otherwise completed upgrade.
type blockedStartupClient struct {
	client.Client
	entered  chan struct{}
	released chan struct{}
}

func (c *blockedStartupClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		close(c.entered)
		<-ctx.Done()
		close(c.released)
		return ctx.Err()
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestStartupCollectionDoesNotBlockUpgrade(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "test"}}
	c, _ := testCollector(t, dda)
	blocking := &blockedStartupClient{Client: c.options.Client, entered: make(chan struct{}), released: make(chan struct{})}
	o := c.options
	o.Client = blocking
	o.IOStreams = genericclioptions.IOStreams{Out: io.Discard, ErrOut: io.Discard}
	o.collectStartupTimings = true
	o.checkPeriod = 20 * time.Millisecond
	o.checkTimeout = time.Second
	done := make(chan error, 1)
	go func() { done <- o.Run() }()
	select {
	case <-blocking.entered:
	case <-time.After(time.Second):
		t.Fatal("collector did not start")
	}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("collection delayed upgrade completion")
	}
	select {
	case <-blocking.released:
	case <-time.After(time.Second):
		t.Fatal("collection was not cancelled")
	}
}

type blockedStartupWriter struct {
	entered  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (w *blockedStartupWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	close(w.finished)
	return len(p), nil
}

func TestStartupOutputDoesNotBlockUpgrade(t *testing.T) {
	// An absent DDA completes the existing check. Telemetry's NotFound error
	// exercises the output path without relying on a particular pod state.
	c, _ := testCollector(t)
	writer := &blockedStartupWriter{entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	defer close(writer.release)
	o := c.options
	o.IOStreams = genericclioptions.IOStreams{Out: io.Discard, ErrOut: writer}
	o.collectStartupTimings = true
	o.checkPeriod = 20 * time.Millisecond
	o.checkTimeout = time.Second
	done := make(chan error, 1)
	go func() { done <- o.Run() }()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("collector did not write")
	}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("telemetry output delayed completion")
	}
}

func TestStartupZeroDuration(t *testing.T) {
	c, output := testCollector(t)
	timestamp := metav1.NewTime(time.Unix(100, 0))
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "pod"}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{{Name: "init", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{StartedAt: timestamp, FinishedAt: timestamp}}}}}}
	require.NoError(t, c.collectPod(pod, "agent"))
	records := readStartupRecords(t, output)
	require.Len(t, records, 2)
	require.NotNil(t, records[1].DurationSeconds)
	assert.Zero(t, *records[1].DurationSeconds)
}

func TestStartupDerivedPodDurations(t *testing.T) {
	c, output := testCollector(t)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "pod", CreationTimestamp: metav1.NewTime(time.Unix(100, 0))}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{
		{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Unix(102, 0))},
		{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Unix(110, 0))},
		{Type: corev1.PodInitialized, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Unix(106, 0))},
	}}}
	require.NoError(t, c.collectPod(pod, "agent"))
	require.NoError(t, c.collectPod(pod, "agent"))
	records := readStartupRecords(t, output)
	require.Len(t, records, 4)
	assert.Equal(t, "creation_to_initialized", records[3].Phase)
	assert.Equal(t, 6.0, *records[3].DurationSeconds)
	assert.Equal(t, "creation_to_scheduled", records[1].Phase)
	assert.Equal(t, 2.0, *records[1].DurationSeconds)
	assert.Equal(t, "creation_to_latest_ready", records[2].Phase)
	assert.Equal(t, 10.0, *records[2].DurationSeconds)
	pod.Status.Conditions[1].Status = corev1.ConditionFalse
	require.NoError(t, c.collectPod(pod, "agent"))
	records = readStartupRecords(t, output)
	require.Len(t, records, 1)
	assert.Nil(t, records[0].DurationSeconds)
	pod.Status.Conditions[1].Status = corev1.ConditionTrue
	pod.Status.Conditions[1].LastTransitionTime = metav1.NewTime(time.Unix(120, 0))
	require.NoError(t, c.collectPod(pod, "agent"))
	records = readStartupRecords(t, output)
	require.Len(t, records, 2)
	assert.Equal(t, "creation_to_latest_ready", records[1].Phase)
	assert.Equal(t, 20.0, *records[1].DurationSeconds)
}

func TestStartupSnapshotOmitsUnrelatedFields(t *testing.T) {
	c, output := testCollector(t)
	status := corev1.ContainerStatus{
		Name: "init-config", Image: "agent:test", ImageID: "sha256:image", ContainerID: "containerd://current", Ready: true, RestartCount: 1,
		State:                corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(time.Unix(110, 0))}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ContainerID: "containerd://previous", Message: "verbose message", ExitCode: 1, Reason: "Error", StartedAt: metav1.NewTime(time.Unix(100, 0)), FinishedAt: metav1.NewTime(time.Unix(105, 0))}},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "pod", OwnerReferences: []metav1.OwnerReference{{Name: "owner"}}}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{status}, ContainerStatuses: []corev1.ContainerStatus{status}}}
	require.NoError(t, c.collectPod(pod, "agent"))
	encoded := output.String()
	for _, field := range []string{"containerID", "volumeMounts", "allocatedResources", "resources", "user", "owners", "verbose message"} {
		assert.NotContains(t, encoded, field)
	}
	records := readStartupRecords(t, output)
	require.Len(t, records, 1)
	for _, container := range []startupContainer{records[0].InitContainers[0], records[0].Containers[0]} {
		assert.Equal(t, status.ImageID, container.ImageID)
		assert.Equal(t, status.State.Running.StartedAt, container.State.Running.StartedAt)
		assert.Equal(t, int32(1), container.LastState.Terminated.ExitCode)
		assert.Equal(t, "Error", container.LastState.Terminated.Reason)
		assert.Equal(t, status.LastTerminationState.Terminated.FinishedAt, container.LastState.Terminated.FinishedAt)
	}
	assert.Equal(t, "containerd://previous", pod.Status.InitContainerStatuses[0].LastTerminationState.Terminated.ContainerID)
	// Changes unrelated to startup must not emit a duplicate snapshot.
	pod.Status.InitContainerStatuses[0].ContainerID = "changed"
	pod.Status.InitContainerStatuses[0].LastTerminationState.Terminated.Message = "changed"
	require.NoError(t, c.collectPod(pod, "agent"))
	assert.Empty(t, output.String())
}
