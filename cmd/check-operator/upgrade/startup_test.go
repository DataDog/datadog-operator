// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
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
	o := &Options{Options: plugincommon.Options{Client: fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v2alpha1.DatadogAgent{}, &v1alpha1.DatadogAgentInternal{}).WithObjects(objects...).Build(), UserNamespace: "test"}, datadogAgentName: "agent"}
	return &startupCollector{measurementsStarted: true, options: o, output: output, snapshots: map[types.UID][32]byte{}, attempts: map[string]struct{}{}}, output
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

func testStartupRevision() *startupRevision {
	return &startupRevision{Name: "agent-new", Hash: "new", CreatedAt: metav1.NewTime(time.Unix(1, 0))}
}
func collectStartupTestPod(c *startupCollector, pod *corev1.Pod) error {
	if pod.Labels == nil {
		pod.Labels = map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: "new"}
	}
	return c.collectPod(pod, "agent", testStartupRevision())
}
func startupTestRevisionForDS(t *testing.T, ds *appsv1.DaemonSet) *appsv1.ControllerRevision {
	t.Helper()
	controlled := true
	template, err := json.Marshal(ds.Spec.Template)
	require.NoError(t, err)
	var saved map[string]any
	require.NoError(t, json.Unmarshal(template, &saved))
	saved["$patch"] = "replace"
	data, err := json.Marshal(map[string]any{"spec": map[string]any{"template": saved}})
	require.NoError(t, err)
	return &appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: ds.Name + "-new", Namespace: ds.Namespace, UID: types.UID(ds.Name + "-revision"), CreationTimestamp: metav1.NewTime(time.Unix(1, 0)), Labels: map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: "new"}, OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", UID: ds.UID, Controller: &controlled}}}, Data: runtime.RawExtension{Raw: data}, Revision: 2}
}

func TestStartupExecutionAndSnapshots(t *testing.T) {
	c, output := testCollector(t)
	start := metav1.NewTime(time.Unix(100, 0))
	finish := metav1.NewTime(time.Unix(108, 0))
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Unix(1, 0)), Name: "agent-pod", Namespace: "test", UID: "pod-1"}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{{Name: "init-config", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{StartedAt: start, FinishedAt: finish, Reason: "Completed"}}}}}}
	require.NoError(t, collectStartupTestPod(c, pod))
	require.NoError(t, collectStartupTestPod(c, pod))
	records := readStartupRecords(t, output)
	require.Len(t, records, 2)
	assert.Equal(t, "agent_startup_snapshot", records[0].Event)
	assert.Equal(t, 8.0, *records[1].DurationSeconds)
	assert.Equal(t, "init_execution", records[1].Phase)
	assert.Equal(t, int32(0), *records[1].ExitCode)
	// A readiness transition produces a new raw snapshot, not another init sample
	// and not a claim about first readiness.
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: finish}}
	require.NoError(t, collectStartupTestPod(c, pod))
	records = readStartupRecords(t, output)
	require.Len(t, records, 2)
	assert.Empty(t, records[0].Phase)
	assert.Equal(t, "creation_to_latest_ready", records[1].Phase)
	// A new failed attempt must be retained and must not be combined with the old.
	pod.Status.InitContainerStatuses[0].RestartCount = 1
	pod.Status.InitContainerStatuses[0].State.Terminated.ExitCode = 1
	pod.Status.InitContainerStatuses[0].State.Terminated.StartedAt = finish
	pod.Status.InitContainerStatuses[0].State.Terminated.FinishedAt = metav1.NewTime(time.Unix(111, 0))
	require.NoError(t, collectStartupTestPod(c, pod))
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
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "p", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{{Name: "init-volume", State: tc.state}}}}
			require.NoError(t, collectStartupTestPod(c, pod))
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
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test", UID: types.UID(name + "-pod"), Labels: map[string]string{"app": name, appsv1.DefaultDaemonSetUniqueLabelKey: "new"}, CreationTimestamp: metav1.NewTime(time.Unix(100, 0)), OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: name, UID: ds.UID, Controller: &controlled}}}}
		wrong := pod.DeepCopy()
		wrong.Name += "-wrong"
		wrong.UID += "-wrong"
		wrong.OwnerReferences[0].UID = "old-daemonset"
		objects = append(objects, ds, pod, wrong, startupTestRevisionForDS(t, ds))
	}
	c, output := testCollector(t, objects...)
	require.NoError(t, c.collect(context.Background()))
	records := readStartupRecords(t, output)
	require.Len(t, records, 2)
	assert.ElementsMatch(t, []string{"agent", "profile"}, []string{records[0].DaemonSet, records[1].DaemonSet})
}

// A blocked telemetry API call is bounded by the shared upgrade deadline and cleanup budget.
type blockedStartupClient struct {
	client.Client
	entered     chan struct{}
	released    chan struct{}
	enterOnce   sync.Once
	releaseOnce sync.Once
}

func (c *blockedStartupClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		c.enterOnce.Do(func() { close(c.entered) })
		<-ctx.Done()
		c.releaseOnce.Do(func() { close(c.released) })
		return ctx.Err()
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestStartupCollectionCannotOutliveUpgradeDeadline(t *testing.T) {
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
	case <-time.After(2 * startupRequestTimeout):
		t.Fatal("collection exceeded the shared upgrade deadline and cleanup budget")
	}
	select {
	case <-blocking.released:
	case <-time.After(time.Second):
		t.Fatal("collection was not cancelled")
	}
}

type blockedStartupWriter struct {
	entered    chan struct{}
	release    chan struct{}
	finished   chan struct{}
	enterOnce  sync.Once
	finishOnce sync.Once
}

func (w *blockedStartupWriter) Write(p []byte) (int, error) {
	w.enterOnce.Do(func() { close(w.entered) })
	<-w.release
	w.finishOnce.Do(func() { close(w.finished) })
	return len(p), nil
}

func TestStartupOutputCannotOutliveUpgradeDeadline(t *testing.T) {
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
	case <-time.After(2 * startupRequestTimeout):
		t.Fatal("telemetry output exceeded the shared upgrade deadline and cleanup budget")
	}
}

func TestStartupZeroDuration(t *testing.T) {
	c, output := testCollector(t)
	timestamp := metav1.NewTime(time.Unix(100, 0))
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "pod", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{{Name: "init-config", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{StartedAt: timestamp, FinishedAt: timestamp}}}}}}
	require.NoError(t, collectStartupTestPod(c, pod))
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
	require.NoError(t, collectStartupTestPod(c, pod))
	require.NoError(t, collectStartupTestPod(c, pod))
	records := readStartupRecords(t, output)
	require.Len(t, records, 4)
	assert.Equal(t, "creation_to_initialized", records[3].Phase)
	assert.Equal(t, 6.0, *records[3].DurationSeconds)
	assert.Equal(t, "creation_to_scheduled", records[1].Phase)
	assert.Equal(t, 2.0, *records[1].DurationSeconds)
	assert.Equal(t, "creation_to_latest_ready", records[2].Phase)
	assert.Equal(t, 10.0, *records[2].DurationSeconds)
	pod.Status.Conditions[1].Status = corev1.ConditionFalse
	require.NoError(t, collectStartupTestPod(c, pod))
	records = readStartupRecords(t, output)
	require.Len(t, records, 1)
	assert.Nil(t, records[0].DurationSeconds)
	pod.Status.Conditions[1].Status = corev1.ConditionTrue
	pod.Status.Conditions[1].LastTransitionTime = metav1.NewTime(time.Unix(120, 0))
	require.NoError(t, collectStartupTestPod(c, pod))
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
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "pod", OwnerReferences: []metav1.OwnerReference{{Name: "owner"}}}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{status}, ContainerStatuses: []corev1.ContainerStatus{{Name: "agent", ImageID: status.ImageID, State: status.State, LastTerminationState: status.LastTerminationState}}}}
	require.NoError(t, collectStartupTestPod(c, pod))
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
	require.NoError(t, collectStartupTestPod(c, pod))
	assert.Empty(t, output.String())
}

func TestStartupCohortAndContainerAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name, hash string
		revision   *startupRevision
		samples    int
	}{
		{"current revision", "new", testStartupRevision(), 3},
		{"previous revision", "old", testStartupRevision(), 0},
		{"unavailable revision", "new", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, out := testCollector(t)
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "p", CreationTimestamp: metav1.NewTime(time.Unix(101, 0)), Labels: map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: tc.hash}}}
			terminated := corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{StartedAt: metav1.NewTime(time.Unix(102, 0)), FinishedAt: metav1.NewTime(time.Unix(103, 0))}}
			for _, name := range startupInitContainerNames {
				pod.Status.InitContainerStatuses = append(pod.Status.InitContainerStatuses, corev1.ContainerStatus{Name: string(name), State: terminated})
			}
			pod.Status.InitContainerStatuses = append(pod.Status.InitContainerStatuses, corev1.ContainerStatus{Name: "emissary", State: terminated})
			always := corev1.ContainerRestartPolicyAlways
			pod.Spec.InitContainers = []corev1.Container{{Name: "init-volume", RestartPolicy: &always}}
			for _, name := range startupContainerNames {
				pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{Name: string(name)})
			}
			pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{Name: "emissary"})
			require.NoError(t, c.collectPod(pod, "agent", tc.revision))
			encoded := out.String()
			records := readStartupRecords(t, out)
			require.Len(t, records, 1+tc.samples)
			assert.Len(t, records[0].InitContainers, len(startupInitContainerNames))
			assert.Len(t, records[0].Containers, len(startupContainerNames))
			for _, r := range records[1:] {
				assert.NotEqual(t, "emissary", r.ContainerName)
				assert.NotEqual(t, "init-volume", r.ContainerName)
				assert.True(t, r.MeasurementsEnabled)
			}
			assert.NotContains(t, encoded, "emissary")
		})
	}
}

type progressingStartupClient struct {
	client.Client
	scans       int
	readyOnScan int
	fresh       bool
	createdAt   *metav1.Time
}

func (c *progressingStartupClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err := c.Client.List(ctx, list, opts...); err != nil {
		return err
	}
	if pods, ok := list.(*corev1.PodList); ok {
		c.scans++
		if c.fresh && c.createdAt == nil {
			value := metav1.Now()
			c.createdAt = &value
		}
		for i := range pods.Items {
			if c.fresh {
				pods.Items[i].CreationTimestamp = *c.createdAt
			}
			if c.readyOnScan > 0 && c.scans >= c.readyOnScan {
				readyAt := metav1.NewTime(time.Unix(110, 0))
				if c.fresh {
					readyAt = metav1.NewTime(c.createdAt.Add(16 * time.Second))
				}
				pods.Items[i].Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: readyAt}}
			}
		}
	}
	return nil
}

func observationCollector(t *testing.T) (*startupCollector, *bytes.Buffer) {
	t.Helper()
	controlled := true
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "test"}, Status: v2alpha1.DatadogAgentStatus{Agent: &v2alpha1.DaemonSetStatus{DaemonsetName: "agent"}}}
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "test", UID: "ds"}, Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}}}, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, UpdatedNumberScheduled: 1, NumberReady: 1}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "test", UID: "pod", CreationTimestamp: metav1.NewTime(time.Unix(100, 0)), Labels: map[string]string{"app": "agent", appsv1.DefaultDaemonSetUniqueLabelKey: "new"}, OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", UID: "ds", Controller: &controlled}}}, Status: corev1.PodStatus{Phase: corev1.PodPending}}
	pod.Spec.NodeName = "worker"
	c, out := testCollector(t, dda, ds, pod, startupTestRevisionForDS(t, ds))
	c.options.checkPeriod = time.Millisecond
	return c, out
}

func TestStartupObservationCapturesLateReadiness(t *testing.T) {
	c, out := observationCollector(t)
	progressing := &progressingStartupClient{Client: c.options.Client, readyOnScan: 2}
	c.options.Client = progressing
	c.observe(context.Background(), time.Now().Add(time.Second))
	records := readStartupRecords(t, out)
	summary := records[len(records)-1]
	assert.Equal(t, 4, progressing.scans)
	assert.Equal(t, "agent_startup_collection_summary", summary.Event)
	assert.Equal(t, 1, summary.SelectedPods)
	assert.Equal(t, 1, summary.ReadyPods)
	assert.False(t, summary.TimedOut)
	found := false
	for _, r := range records {
		if r.Phase == "creation_to_latest_ready" {
			found = true
			assert.Equal(t, 10.0, *r.DurationSeconds)
		}
	}
	assert.True(t, found)
}

func TestStartupObservationTimesOutWithCoverage(t *testing.T) {
	c, out := observationCollector(t)
	c.observe(context.Background(), time.Now().Add(15*time.Millisecond))
	records := readStartupRecords(t, out)
	summary := records[len(records)-1]
	assert.Equal(t, "agent_startup_collection_summary", summary.Event)
	assert.Equal(t, 1, summary.ExpectedPods)
	assert.Equal(t, 1, summary.SelectedPods)
	assert.Equal(t, 1, summary.UnfinishedPods)
	assert.True(t, summary.TimedOut)
}

func TestStartupCoverageRetainsMissingAndFailedPods(t *testing.T) {
	c, out := observationCollector(t)
	require.NoError(t, c.collect(context.Background()))
	pod := &corev1.Pod{}
	require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "pod"}, pod))
	pod.Status.Phase = corev1.PodFailed
	require.NoError(t, collectStartupTestPod(c, pod))
	require.NoError(t, c.options.Client.Delete(context.Background(), pod))
	require.NoError(t, c.collect(context.Background()))
	c.emitSummary(false)
	records := readStartupRecords(t, out)
	summary := records[len(records)-1]
	assert.Equal(t, 1, summary.SelectedPods)
	assert.Equal(t, 1, summary.TerminalPods)
	assert.Equal(t, 0, summary.ReadyPods)
	assert.False(t, c.coverageComplete())
}

func TestStartupObservationDoesNotHideMissingPods(t *testing.T) {
	c, _ := observationCollector(t)
	require.NoError(t, c.collect(context.Background()))
	pod := &corev1.Pod{}
	require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "pod"}, pod))
	require.NoError(t, c.options.Client.Delete(context.Background(), pod))
	require.NoError(t, c.collect(context.Background()))
	assert.True(t, c.outcomes["pod"].missing)
	assert.Len(t, c.outcomes, 1)
}

func TestStartupObservationFailurePreservesUpgradeResult(t *testing.T) {
	// The ordinary upgrade check treats an absent DDA as success. Observation
	// cannot turn that success into a failure, even when every telemetry read fails.
	c, _ := testCollector(t)
	o := c.options
	o.IOStreams = genericclioptions.IOStreams{Out: io.Discard, ErrOut: io.Discard}
	o.collectStartupTimings = true
	o.checkPeriod = time.Millisecond
	o.checkTimeout = 10 * time.Millisecond
	started := time.Now()
	require.NoError(t, o.Run())
	assert.Less(t, time.Since(started), time.Second)
}

func TestStartupCurrentRevisionMatchesTemplateAndOwner(t *testing.T) {
	c, _ := observationCollector(t)
	ds := &appsv1.DaemonSet{}
	require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "agent"}, ds))
	current, err := c.currentRevision(context.Background(), ds)
	require.NoError(t, err)
	assert.Equal(t, "agent-new", current.Name)
	// A rollback/dedup can leave identical templates with several revisions.
	duplicate := startupTestRevisionForDS(t, ds)
	duplicate.Name = "agent-duplicate"
	duplicate.UID = "duplicate"
	duplicate.Revision = 3
	duplicate.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "duplicate"
	require.NoError(t, c.options.Client.Create(context.Background(), duplicate))
	wrongOwner := duplicate.DeepCopy()
	wrongOwner.Name = "foreign"
	wrongOwner.ResourceVersion = ""
	wrongOwner.UID = "foreign"
	wrongOwner.Revision = 100
	wrongOwner.OwnerReferences[0].UID = "other-ds"
	require.NoError(t, c.options.Client.Create(context.Background(), wrongOwner))
	previous := duplicate.DeepCopy()
	previous.Name = "agent-old"
	previous.ResourceVersion = ""
	previous.UID = "old"
	previous.Revision = 99
	previous.Data.Raw = []byte(`{"spec":{"template":{"spec":{"containers":[{"name":"agent","image":"old"}]},"$patch":"replace"}}}`)
	require.NoError(t, c.options.Client.Create(context.Background(), previous))
	current, err = c.currentRevision(context.Background(), ds)
	require.NoError(t, err)
	assert.Equal(t, "agent-duplicate", current.Name)
	assert.Equal(t, "duplicate", current.Hash)
}

func TestStartupRevisionUnavailableKeepsDiagnostics(t *testing.T) {
	c, out := observationCollector(t)
	revision := &appsv1.ControllerRevision{}
	require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "agent-new"}, revision))
	require.NoError(t, c.options.Client.Delete(context.Background(), revision))
	require.NoError(t, c.collect(context.Background()))
	records := readStartupRecords(t, out)
	require.Len(t, records, 2)
	assert.Equal(t, "agent_startup_collection_error", records[0].Event)
	assert.Contains(t, records[0].Error, "no owned ControllerRevision")
	assert.Equal(t, "agent_startup_snapshot", records[1].Event)
	assert.False(t, records[1].MeasurementsEnabled)
	assert.False(t, c.collectionComplete)
	assert.Empty(t, c.outcomes)
}

type deniedRevisionClient struct{ client.Client }

func (c *deniedRevisionClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*appsv1.ControllerRevisionList); ok {
		return fmt.Errorf("controllerrevisions is forbidden")
	}
	return c.Client.List(ctx, list, opts...)
}
func TestStartupRevisionReadDeniedKeepsDiagnostics(t *testing.T) {
	c, out := observationCollector(t)
	c.options.Client = &deniedRevisionClient{Client: c.options.Client}
	require.NoError(t, c.collect(context.Background()))
	records := readStartupRecords(t, out)
	require.Len(t, records, 2)
	assert.Contains(t, records[0].Error, "forbidden")
	assert.False(t, records[1].MeasurementsEnabled)
	assert.Empty(t, c.outcomes)
}

func TestStartupStableRevisionReportsHistoricalDates(t *testing.T) {
	c, out := observationCollector(t)
	c.options.Client = &progressingStartupClient{Client: c.options.Client, readyOnScan: 1}
	// These fixtures predate this invocation by decades. Current-revision startup
	// remains usable for diagnostics, but the dates must not imply a fresh rollout.
	c.observe(context.Background(), time.Now().Add(time.Second))
	records := readStartupRecords(t, out)
	summary := records[len(records)-1]
	require.NotNil(t, summary.OldestPodCreatedAt)
	assert.Equal(t, int64(100), summary.OldestPodCreatedAt.Unix())
	assert.Equal(t, summary.OldestPodCreatedAt, summary.NewestPodCreatedAt)
	require.Contains(t, summary.Revisions, "agent")
	assert.Equal(t, int64(1), summary.Revisions["agent"].CreatedAt.Unix())
	assert.Equal(t, 1, summary.ReadyPods)
}

func TestStartupCoverageRequiresEachDaemonSet(t *testing.T) {
	c, _ := testCollector(t)
	c.collectionComplete = true
	c.rolloutComplete = true
	c.expectedByDS = map[string]int{"agent": 1, "profile": 1}
	c.outcomes = map[types.UID]startupPodOutcome{"a": {daemonset: "agent", currentlyReady: true, nodeName: "worker1"}, "b": {daemonset: "agent", currentlyReady: true, nodeName: "worker2"}}
	assert.False(t, c.coverageComplete())
	c.outcomes["b"] = startupPodOutcome{daemonset: "profile", currentlyReady: true, nodeName: "worker1"}
	assert.True(t, c.coverageComplete())
}

func TestStartupDefersMeasurementsUntilFinalRevision(t *testing.T) {
	c, out := observationCollector(t)
	c.measurementsStarted = false
	require.NoError(t, c.collect(context.Background()))
	records := readStartupRecords(t, out)
	require.Len(t, records, 1)
	assert.False(t, records[0].MeasurementsEnabled)
	assert.Len(t, c.outcomes, 1)
	ds := &appsv1.DaemonSet{}
	require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "agent"}, ds))
	ds.Spec.Template.Spec.Containers = []corev1.Container{{Name: "agent", Image: "new"}}
	require.NoError(t, c.options.Client.Update(context.Background(), ds))
	revision := startupTestRevisionForDS(t, ds)
	revision.Name = "agent-final"
	revision.UID = "final"
	revision.Revision = 3
	revision.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "final"
	require.NoError(t, c.options.Client.Create(context.Background(), revision))
	pod := &corev1.Pod{}
	require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "pod"}, pod))
	pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "final"
	require.NoError(t, c.options.Client.Update(context.Background(), pod))
	c.options.Client = &progressingStartupClient{Client: c.options.Client, readyOnScan: 1}
	c.observe(context.Background(), time.Now().Add(time.Second))
	records = readStartupRecords(t, out)
	measurements := 0
	for _, record := range records {
		if record.Event == "agent_startup_measurement" {
			measurements++
			require.NotNil(t, record.Revision)
			assert.Equal(t, "agent-final", record.Revision.Name)
		}
	}
	assert.Equal(t, 1, measurements)
	assert.Equal(t, "agent-final", c.revisions["agent"].Name)
}

func TestStartupCompletionRequiresDistinctReadyNodes(t *testing.T) {
	c, _ := testCollector(t)
	c.collectionComplete = true
	c.rolloutComplete = true
	c.expectedByDS = map[string]int{"agent": 2}
	c.outcomes = map[types.UID]startupPodOutcome{
		"one": {daemonset: "agent", currentlyReady: true, nodeName: "worker1"},
		"two": {daemonset: "agent", currentlyReady: true, nodeName: "worker1"},
	}
	assert.False(t, c.coverageComplete())
	second := c.outcomes["two"]
	second.nodeName = "worker2"
	c.outcomes["two"] = second
	assert.True(t, c.coverageComplete())
	second.terminal = true
	c.outcomes["two"] = second
	assert.False(t, c.coverageComplete())
	second.terminal = false
	second.currentlyReady = false
	c.outcomes["two"] = second
	assert.False(t, c.coverageComplete())
	second.currentlyReady = true
	second.missing = true
	c.outcomes["two"] = second
	assert.False(t, c.coverageComplete())
}

func TestStartupCompletionChecksDaemonSetStatus(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		updated, ready       int32
		generation, observed int64
	}{
		{"not all updated", 0, 1, 1, 1}, {"not all ready", 1, 0, 1, 1}, {"stale status", 1, 1, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := observationCollector(t)
			c.options.Client = &progressingStartupClient{Client: c.options.Client, readyOnScan: 1}
			ds := &appsv1.DaemonSet{}
			require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "agent"}, ds))
			ds.Generation = tc.generation
			require.NoError(t, c.options.Client.Update(context.Background(), ds))
			ds.Status.UpdatedNumberScheduled = tc.updated
			ds.Status.NumberReady = tc.ready
			ds.Status.ObservedGeneration = tc.observed
			require.NoError(t, c.options.Client.Status().Update(context.Background(), ds))
			require.NoError(t, c.collect(context.Background()))
			assert.False(t, c.coverageComplete())
		})
	}
}

func TestStartupWaitUsesRemainingUpgradeDeadline(t *testing.T) {
	c, out := observationCollector(t)
	c.options.Client = &progressingStartupClient{Client: c.options.Client, readyOnScan: 1000, fresh: true}
	o := c.options
	o.IOStreams = genericclioptions.IOStreams{Out: io.Discard, ErrOut: out}
	o.collectStartupTimings = true
	o.checkTimeout = 80 * time.Millisecond
	started := time.Now()
	require.NoError(t, o.Run())
	elapsed := time.Since(started)
	assert.GreaterOrEqual(t, elapsed, 70*time.Millisecond)
	assert.Less(t, elapsed, 500*time.Millisecond)
	records := readStartupRecords(t, out)
	summary := records[len(records)-1]
	assert.Equal(t, "agent_startup_collection_summary", summary.Event)
	assert.True(t, summary.TimedOut)
	assert.False(t, summary.RolloutComplete)
	assert.Equal(t, 1, summary.UnfinishedPods)
}

func TestStartupWaitsForLargeCohortAfterThresholdSuccess(t *testing.T) {
	c, out := observationCollector(t)
	pod := &corev1.Pod{}
	require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "pod"}, pod))
	for i := 1; i < 50; i++ {
		next := pod.DeepCopy()
		next.Name = fmt.Sprintf("pod-%d", i)
		next.UID = types.UID(next.Name)
		next.ResourceVersion = ""
		next.Spec.NodeName = fmt.Sprintf("worker-%d", i)
		require.NoError(t, c.options.Client.Create(context.Background(), next))
	}
	ds := &appsv1.DaemonSet{}
	require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "agent"}, ds))
	ds.Status.DesiredNumberScheduled = 50
	ds.Status.UpdatedNumberScheduled = 50
	ds.Status.NumberReady = 50
	require.NoError(t, c.options.Client.Status().Update(context.Background(), ds))
	progressing := &progressingStartupClient{Client: c.options.Client, readyOnScan: 8, fresh: true}
	c.options.Client = progressing
	o := c.options
	o.IOStreams = genericclioptions.IOStreams{Out: io.Discard, ErrOut: out}
	o.collectStartupTimings = true
	o.checkPeriod = 5 * time.Millisecond
	o.checkTimeout = 2 * time.Second
	require.NoError(t, o.Run())
	records := readStartupRecords(t, out)
	summary := records[len(records)-1]
	assert.True(t, summary.RolloutComplete)
	assert.False(t, summary.TimedOut)
	assert.Equal(t, 50, summary.ReadyPods)
	assert.GreaterOrEqual(t, progressing.scans, 9)
}

func TestStartupAggregateAgentStatus(t *testing.T) {
	for _, tc := range []struct {
		name           string
		source         string
		pendingProfile bool
		complete       bool
	}{
		{name: "named DDAI", source: "ddai", complete: true},
		{name: "named AgentList", source: "agentList", complete: true},
		{name: "aggregate without concrete targets"},
		{name: "pending profile", source: "ddai", pendingProfile: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, out := observationCollector(t)
			ctx := context.Background()
			dda := &v2alpha1.DatadogAgent{}
			require.NoError(t, c.options.Client.Get(ctx, client.ObjectKey{Namespace: "test", Name: "agent"}, dda))
			dda.Status.Agent = &v2alpha1.DaemonSetStatus{Current: 1, UpToDate: 1}
			if tc.source == "agentList" {
				dda.Status.AgentList = []*v2alpha1.DaemonSetStatus{{DaemonsetName: "agent"}}
			}
			require.NoError(t, c.options.Client.Status().Update(ctx, dda))
			if tc.source == "ddai" {
				ddai := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "base", Namespace: "test", Labels: map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"}}}
				ddai.Status.Agent = &v2alpha1.DaemonSetStatus{DaemonsetName: "agent"}
				require.NoError(t, c.options.Client.Create(ctx, ddai))
			}
			if tc.pendingProfile {
				pending := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "pending-profile", Namespace: "test", Labels: map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"}}}
				require.NoError(t, c.options.Client.Create(ctx, pending))
			}
			c.options.Client = &progressingStartupClient{Client: c.options.Client, readyOnScan: 1}
			c.observe(ctx, time.Now().Add(30*time.Millisecond))
			records := readStartupRecords(t, out)
			summary := records[len(records)-1]
			assert.Equal(t, tc.complete, summary.RolloutComplete)
			measured := false
			for _, record := range records {
				if record.Event == "agent_startup_measurement" {
					measured = true
				}
				if tc.complete {
					assert.NotEqual(t, "agent_startup_collection_error", record.Event)
				}
			}
			assert.Equal(t, tc.complete, measured)
		})
	}
}

func TestStartupUnknownAgentStatusCannotComplete(t *testing.T) {
	for _, profile := range []bool{false, true} {
		t.Run(fmt.Sprintf("profile=%t", profile), func(t *testing.T) {
			c, out := observationCollector(t)
			if profile {
				ddai := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "pending-profile", Namespace: "test", Labels: map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"}}}
				require.NoError(t, c.options.Client.Create(context.Background(), ddai))
			} else {
				dda := &v2alpha1.DatadogAgent{}
				require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "agent"}, dda))
				dda.Status.Agent = nil
				require.NoError(t, c.options.Client.Status().Update(context.Background(), dda))
			}
			c.options.Client = &progressingStartupClient{Client: c.options.Client, readyOnScan: 1}
			require.NoError(t, c.collect(context.Background()))
			assert.False(t, c.collectionComplete)
			assert.False(t, c.coverageComplete())
			records := readStartupRecords(t, out)
			found := false
			for _, record := range records {
				if strings.Contains(record.Error, "status is not available yet") {
					found = true
				}
			}
			assert.True(t, found)
		})
	}
}

func TestStartupDisabledAgentAndZeroNodes(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		disabled := true
		dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "test"}, Spec: v2alpha1.DatadogAgentSpec{Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{v2alpha1.NodeAgentComponentName: {Disabled: &disabled}}}}
		c, out := testCollector(t, dda)
		require.NoError(t, c.collect(context.Background()))
		assert.True(t, c.coverageComplete())
		c.emitSummary(false)
		records := readStartupRecords(t, out)
		require.Len(t, records, 1)
		assert.Equal(t, "node_agent_disabled", records[0].Reason)
		assert.True(t, records[0].RolloutComplete)
	})
	t.Run("known DaemonSet with zero nodes", func(t *testing.T) {
		c, _ := observationCollector(t)
		ds := &appsv1.DaemonSet{}
		require.NoError(t, c.options.Client.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "agent"}, ds))
		ds.Status.DesiredNumberScheduled = 0
		ds.Status.UpdatedNumberScheduled = 0
		ds.Status.NumberReady = 0
		require.NoError(t, c.options.Client.Status().Update(context.Background(), ds))
		require.NoError(t, c.collect(context.Background()))
		assert.True(t, c.collectionComplete)
		assert.True(t, c.coverageComplete())
		assert.Contains(t, c.revisions, "agent")
		assert.Zero(t, c.expectedPods)
	})
}

func TestStartupRepeatedRunsMeasureExistingReadyPods(t *testing.T) {
	c, out := observationCollector(t)
	c.options.Client = &progressingStartupClient{Client: c.options.Client, readyOnScan: 1}
	o := c.options
	o.IOStreams = genericclioptions.IOStreams{Out: io.Discard, ErrOut: out}
	o.collectStartupTimings = true
	o.checkTimeout = time.Second
	for range 2 {
		out.Reset()
		require.NoError(t, o.Run())
		records := readStartupRecords(t, out)
		measured := false
		for _, record := range records {
			if record.Event == "agent_startup_measurement" {
				measured = true
			}
		}
		assert.True(t, measured)
		summary := records[len(records)-1]
		assert.True(t, summary.RolloutComplete)
		assert.True(t, summary.MeasurementsEnabled)
		assert.Equal(t, 1, summary.SelectedPods)
	}
}

func TestStartupExistingPodsAndProfiles(t *testing.T) {
	c, out := testCollector(t)
	for _, name := range []string{"agent", "profile"} {
		for _, created := range []int64{100, 101, 102} {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%d", name, created), UID: types.UID(fmt.Sprintf("%s-%d", name, created)), CreationTimestamp: metav1.NewTime(time.Unix(created, 0)), Labels: map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: "new"}}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Unix(110, 0))}}}}
			require.NoError(t, c.collectPod(pod, name, testStartupRevision()))
		}
	}
	measured := []string{}
	for _, r := range readStartupRecords(t, out) {
		if r.Event == "agent_startup_measurement" {
			measured = append(measured, r.PodName)
		}
	}
	assert.ElementsMatch(t, []string{"agent-100", "agent-101", "agent-102", "profile-100", "profile-101", "profile-102"}, measured)
	assert.Len(t, c.outcomes, 6)
}

type delayedStartupRevisionClient struct {
	client.Client
	scans int
}

func (c *delayedStartupRevisionClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if pods, ok := list.(*corev1.PodList); ok {
		c.scans++
		if c.scans == 2 {
			ds := &appsv1.DaemonSet{}
			if err := c.Client.Get(ctx, client.ObjectKey{Namespace: "test", Name: "agent"}, ds); err != nil {
				return err
			}
			ds.Spec.Template.Annotations = map[string]string{"test": "new-revision"}
			if err := c.Client.Update(ctx, ds); err != nil {
				return err
			}
			revision := startupTestRevisionForDSData(ds)
			if err := c.Client.Create(ctx, revision); err != nil {
				return err
			}
		}
		if err := c.Client.List(ctx, pods, opts...); err != nil {
			return err
		}
		for i := range pods.Items {
			if c.scans >= 2 {
				pods.Items[i].Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Unix(110, 0))}}
				pods.Items[i].CreationTimestamp = metav1.NewTime(time.Unix(102, 0))
				pods.Items[i].Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "later"
			}
		}
		return nil
	}
	return c.Client.List(ctx, list, opts...)
}

func startupTestRevisionForDSData(ds *appsv1.DaemonSet) *appsv1.ControllerRevision {
	data, _ := json.Marshal(map[string]any{"spec": map[string]any{"template": ds.Spec.Template}})
	controlled := true
	return &appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: ds.Name + "-later", Namespace: ds.Namespace, CreationTimestamp: metav1.NewTime(time.Unix(101, 0)), Labels: map[string]string{appsv1.DefaultDaemonSetUniqueLabelKey: "later"}, OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", UID: ds.UID, Controller: &controlled}}}, Revision: 3, Data: runtime.RawExtension{Raw: data}}
}

func TestStartupFollowsRevisionWhileIncomplete(t *testing.T) {
	c, out := observationCollector(t)
	delayed := &delayedStartupRevisionClient{Client: c.options.Client}
	c.options.Client = delayed
	c.observe(context.Background(), time.Now().Add(time.Second))
	records := readStartupRecords(t, out)
	measured := false
	for _, r := range records {
		if r.Event == "agent_startup_measurement" {
			measured = true
			require.NotNil(t, r.Revision)
			assert.Equal(t, "later", r.Revision.Hash)
		}
	}
	assert.True(t, measured)
	summary := records[len(records)-1]
	assert.True(t, summary.RolloutComplete)
	assert.False(t, summary.TimedOut)
	assert.GreaterOrEqual(t, delayed.scans, 4)
}

func TestStartupTimestampRangeRetainsOtherProfilesOnRevisionChange(t *testing.T) {
	c, _ := observationCollector(t)
	c.outcomes = map[types.UID]startupPodOutcome{"profile-pod": {daemonset: "profile", createdAt: metav1.NewTime(time.Unix(50, 0))}}
	c.revisions = map[string]*startupRevision{"agent": {Name: "preceding-revision", Hash: "old"}}
	require.NoError(t, c.collect(context.Background()))
	require.NotNil(t, c.oldestPod)
	require.NotNil(t, c.newestPod)
	assert.Equal(t, int64(50), c.oldestPod.Unix())
	assert.Equal(t, int64(100), c.newestPod.Unix())
}
