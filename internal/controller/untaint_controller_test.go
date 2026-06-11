// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	clocktesting "k8s.io/utils/clock/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogcsidriver"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/untaint"
)

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

const (
	testNodeName = "node-1"
	testPodName  = "agent-pod-1"
	testPodNS    = "default"
)

// testNow returns the current time truncated to second precision. The fake
// client round-trips metav1.Time through RFC3339 (second precision), so any
// pod/node timestamps stored and retrieved through it lose sub-second
// resolution; tests use this to keep elapsed-time assertions exact.
func testNow() time.Time { return time.Now().Truncate(time.Second) }

func taintedNode(name string, createdAgo time.Duration, now time.Time) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(now.Add(-createdAgo)),
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{untaint.AgentNotReadyTaint()},
		},
	}
}

func untaintedNode(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func agentPod(name, ns, nodeName string, ready bool, startedAgo time.Duration, now time.Time) *corev1.Pod {
	cond := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionFalse}
	if ready {
		cond.Status = corev1.ConditionTrue
		cond.LastTransitionTime = metav1.NewTime(now.Add(-time.Second))
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
			},
		},
		Spec: corev1.PodSpec{NodeName: nodeName},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{cond},
		},
	}
	if startedAgo > 0 {
		start := metav1.NewTime(now.Add(-startedAgo))
		pod.Status.StartTime = &start
	}
	return pod
}

func csiNodeServerPod(name, ns, nodeName string, ready bool, startedAgo time.Duration, now time.Time) *corev1.Pod {
	cond := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionFalse}
	if ready {
		cond.Status = corev1.ConditionTrue
		cond.LastTransitionTime = metav1.NewTime(now.Add(-time.Second))
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				datadogcsidriver.AppLabelKey: datadogcsidriver.NodeServerDaemonSetAppValue,
			},
		},
		Spec: corev1.PodSpec{NodeName: nodeName},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{cond},
		},
	}
	if startedAgo > 0 {
		start := metav1.NewTime(now.Add(-startedAgo))
		pod.Status.StartTime = &start
	}
	return pod
}

func nonAgentPod(name, ns, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"app": "other"}},
		Spec:       corev1.PodSpec{NodeName: nodeName},
	}
}

func newFakeClient(t *testing.T, objs ...client.Object) client.WithWatch {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithIndex(&corev1.Pod{}, untaintPodNodeIndex, func(obj client.Object) []string {
			pod, ok := obj.(*corev1.Pod)
			if !ok || pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		}).
		Build()
}

func newReconciler(t *testing.T, c client.Client, now time.Time, policy TimeoutPolicy, readiness, scheduling time.Duration, waitForCSIDriver bool) (*UntaintReconciler, *record.FakeRecorder) {
	t.Helper()
	rec := record.NewFakeRecorder(16)
	r := &UntaintReconciler{
		client:            c,
		log:               log.Log.WithName("test"),
		recorder:          rec,
		clock:             clocktesting.NewFakePassiveClock(now),
		waitForCSIDriver:  waitForCSIDriver,
		eventsEnabled:     true,
		readinessTimeout:  readiness,
		schedulingTimeout: scheduling,
		timeoutPolicy:     policy,
	}
	return r, rec
}

// -----------------------------------------------------------------------------
// helper functions
// -----------------------------------------------------------------------------

func TestHasTaint(t *testing.T) {
	assert.False(t, hasTaint(&corev1.Node{}))
	assert.False(t, hasTaint(&corev1.Node{Spec: corev1.NodeSpec{Taints: nil}}))
	assert.False(t, hasTaint(&corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{{Key: "other"}}}}))
	assert.True(t, hasTaint(&corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{untaint.AgentNotReadyTaint()}}}))
}

func TestDurationFromEnv(t *testing.T) {
	const key = "DD_UNTAINT_CONTROLLER_TEST_DURATION"

	cases := []struct {
		name    string
		value   string // "<unset>" sentinel means do not set
		want    time.Duration
		wantErr bool
	}{
		{name: "unset → default", value: "<unset>", want: 7 * time.Minute},
		{name: "empty → default", value: "", want: 7 * time.Minute},
		{name: "valid duration", value: "42s", want: 42 * time.Second},
		{name: "unparseable", value: "not-a-duration", wantErr: true},
		{name: "zero rejected", value: "0s", wantErr: true},
		{name: "negative rejected", value: "-5s", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == "<unset>" {
				_ = os.Unsetenv(key)
			} else {
				t.Setenv(key, tc.value)
			}
			got, err := durationFromEnv(key, 7*time.Minute)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNewUntaintReconciler_ConfigErrors(t *testing.T) {
	// Each subtest sets one bad env var and verifies NewUntaintReconciler
	// returns an error rather than silently substituting the default.
	cases := []struct {
		name   string
		envKey string
		envVal string
	}{
		{"bad readiness timeout", EnvReadinessTimeout, "bogus"},
		{"bad scheduling timeout", EnvSchedulingTimeout, "bogus"},
		{"bad timeout policy", EnvTimeoutPolicy, "unknown"},
		{"zero readiness timeout", EnvReadinessTimeout, "0s"},
		{"negative scheduling timeout", EnvSchedulingTimeout, "-1m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envKey, tc.envVal)
			_, err := NewUntaintReconciler(newFakeClient(t), log.Log, record.NewFakeRecorder(1), false)
			assert.Error(t, err, "expected NewUntaintReconciler to fail on %s=%q", tc.envKey, tc.envVal)
		})
	}
}

func TestParseTimeoutPolicy(t *testing.T) {
	for _, in := range []string{"", "remove"} {
		p, err := ParseTimeoutPolicy(in)
		assert.NoError(t, err)
		assert.Equal(t, PolicyRemove, p)
	}
	p, err := ParseTimeoutPolicy("keep")
	assert.NoError(t, err)
	assert.Equal(t, PolicyKeep, p)

	_, err = ParseTimeoutPolicy("bogus")
	assert.Error(t, err)
}

func TestLatestPodStartTime(t *testing.T) {
	now := testNow()
	p1 := corev1.Pod{}
	p2 := corev1.Pod{}
	t1 := metav1.NewTime(now.Add(-3 * time.Minute))
	t2 := metav1.NewTime(now.Add(-1 * time.Minute))
	p1.Status.StartTime = &t1
	p2.Status.StartTime = &t2

	got, ok := latestPodStartTime([]corev1.Pod{p1, p2})
	assert.True(t, ok)
	assert.Equal(t, t2.Time, got)

	got, ok = latestPodStartTime([]corev1.Pod{})
	assert.False(t, ok)
	assert.True(t, got.IsZero())

	got, ok = latestPodStartTime([]corev1.Pod{{}}) // no start time
	assert.False(t, ok)
	assert.True(t, got.IsZero())
}

// -----------------------------------------------------------------------------
// predicates
// -----------------------------------------------------------------------------

func TestAgentPodPredicate(t *testing.T) {
	now := testNow()
	r, _ := newReconciler(t, newFakeClient(t), now, PolicyRemove, time.Minute, time.Minute, false)
	p := r.podWatchPredicate()

	readyAgent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	notReadyAgent := agentPod(testPodName, testPodNS, testNodeName, false, 1*time.Minute, now)
	other := nonAgentPod(testPodName, testPodNS, testNodeName)

	// Create: enqueue for any agent pod, regardless of readiness. A NotReady
	// agent pod create must enqueue so the reconciler starts the readiness
	// clock against pod.Status.StartTime — filtering on readiness here would
	// otherwise leave the readiness timeout to fire late or never (e.g. under
	// policy=keep, where no pending requeue exists after a previous timeout).
	assert.True(t, p.Create(event.CreateEvent{Object: readyAgent}), "ready agent on create")
	assert.True(t, p.Create(event.CreateEvent{Object: notReadyAgent}), "not-ready agent on create")
	assert.False(t, p.Create(event.CreateEvent{Object: other}), "non-agent pod on create")

	// Update: enqueue for any agent pod update. Reconcile early-returns when
	// !hasTaint(node) so churn is bounded.
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: notReadyAgent, ObjectNew: readyAgent}), "transition to ready")
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: readyAgent, ObjectNew: readyAgent}), "ready→ready")
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: notReadyAgent, ObjectNew: notReadyAgent}), "not-ready→not-ready")
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: other, ObjectNew: other}), "non-agent on update")

	// Delete: agent pods enqueue so the node's readiness clock can restart
	// for the replacement pod (avoids stuck nodes under PolicyKeep).
	assert.True(t, p.Delete(event.DeleteEvent{Object: readyAgent}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: notReadyAgent}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: other}))

	// Generic events ignored.
	assert.False(t, p.Generic(event.GenericEvent{Object: readyAgent}))
}

func TestTaintedNodePredicate(t *testing.T) {
	now := testNow()
	p := taintedNodePredicate()

	tainted := taintedNode(testNodeName, 0, now)
	untainted := untaintedNode(testNodeName)

	// Create
	assert.True(t, p.Create(event.CreateEvent{Object: tainted}))
	assert.False(t, p.Create(event.CreateEvent{Object: untainted}))

	// Update: only "untainted → tainted" should fire
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: untainted, ObjectNew: tainted}), "appearance")
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: tainted, ObjectNew: tainted}), "still tainted")
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: tainted, ObjectNew: untainted}), "disappearance")
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: untainted, ObjectNew: untainted}), "still untainted")

	// Delete / Generic
	assert.False(t, p.Delete(event.DeleteEvent{Object: tainted}))
	assert.False(t, p.Generic(event.GenericEvent{Object: tainted}))
}

// -----------------------------------------------------------------------------
// removeTaint: JSON patch serialization + race handling
// -----------------------------------------------------------------------------

func TestRemoveTaint_PatchContents(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	// Add an unrelated taint we should preserve.
	other := corev1.Taint{Key: "other", Value: "v", Effect: corev1.TaintEffectNoExecute}
	node.Spec.Taints = append(node.Spec.Taints, other)

	var captured []byte
	c := interceptor.NewClient(newFakeClient(t, node), interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			b, err := patch.Data(obj)
			if err != nil {
				return err
			}
			captured = b
			return nil
		},
	})

	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, false)
	result, err := r.removeTaint(context.Background(), node)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	require.NotNil(t, captured)

	var ops []jsonPatchOp
	require.NoError(t, json.Unmarshal(captured, &ops))
	require.Len(t, ops, 2)
	assert.Equal(t, "test", ops[0].Op)
	assert.Equal(t, "/spec/taints", ops[0].Path)
	assert.Equal(t, "replace", ops[1].Op)
	assert.Equal(t, "/spec/taints", ops[1].Path)

	// Marshal-back the values to inspect them as JSON.
	testJSON, _ := json.Marshal(ops[0].Value)
	replJSON, _ := json.Marshal(ops[1].Value)
	assert.Contains(t, string(testJSON), untaint.AgentNotReadyTaintKey, "test op carries current taints")
	assert.Contains(t, string(replJSON), "other", "replace op preserves unrelated taints")
	assert.NotContains(t, string(replJSON), untaint.AgentNotReadyTaintKey, "replace op drops the target taint")
}

func TestRemoveTaint_NilTaintsDefensive(t *testing.T) {
	// A node with nil Spec.Taints should never reach removeTaint via Reconcile,
	// but the helper must not panic and must not produce a JSON `null` for the
	// replace value. Verify by calling directly.
	now := testNow()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: testNodeName}}
	var captured []byte
	c := interceptor.NewClient(newFakeClient(t, node), interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			b, err := patch.Data(obj)
			if err != nil {
				return err
			}
			captured = b
			return nil
		},
	})
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, false)
	result, err := r.removeTaint(context.Background(), node)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	// Patch should not have been issued because there's nothing to remove.
	assert.Nil(t, captured)
}

func TestRemoveTaint_ConflictRequeues(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)

	cases := []struct {
		name string
		err  error
	}{
		{"isConflict", apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, testNodeName, errors.New("race"))},
		{"isInvalid", apierrors.NewInvalid(schema.GroupKind{Kind: "Node"}, testNodeName, nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := interceptor.NewClient(newFakeClient(t, node), interceptor.Funcs{
				Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
					return tc.err
				},
			})
			r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, false)
			result, err := r.removeTaint(context.Background(), node.DeepCopy())
			assert.NoError(t, err, "race must not surface as error")
			assert.Equal(t, conflictRequeueDelay, result.RequeueAfter, "race should requeue with conflictRequeueDelay")
		})
	}
}

func TestRemoveTaint_OtherErrorPropagates(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	c := interceptor.NewClient(newFakeClient(t, node), interceptor.Funcs{
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return errors.New("boom")
		},
	})
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, false)
	_, err := r.removeTaint(context.Background(), node.DeepCopy())
	assert.Error(t, err)
}

// -----------------------------------------------------------------------------
// Reconcile: outcomes
// -----------------------------------------------------------------------------

func TestReconcile_NodeNotFound(t *testing.T) {
	now := testNow()
	r, _ := newReconciler(t, newFakeClient(t), now, PolicyRemove, time.Minute, time.Minute, false)
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_NoTaint(t *testing.T) {
	now := testNow()
	node := untaintedNode(testNodeName)
	r, _ := newReconciler(t, newFakeClient(t, node), now, PolicyRemove, time.Minute, time.Minute, false)
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_PodReady_RemovesTaint(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	pod := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)

	c := newFakeClient(t, node, pod)
	r, rec := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, false)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.False(t, hasTaint(fresh), "taint should be removed")

	select {
	case ev := <-rec.Events:
		assert.Contains(t, ev, "TaintRemoved")
	default:
		t.Fatal("expected TaintRemoved event")
	}
}

func TestReconcile_TerminatingReadyPod_RemovesTaint(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	pod := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	deleteTime := metav1.NewTime(now)
	pod.DeletionTimestamp = &deleteTime
	pod.Finalizers = []string{"test/finalizer"}

	c := newFakeClient(t, node, pod)
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, false)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.False(t, hasTaint(fresh), "a Ready agent pod satisfies the initial join condition even if it is terminating")
}

func TestReconcile_ReadinessTimeout(t *testing.T) {
	const readiness = 10 * time.Minute
	const scheduling = 5 * time.Minute
	now := testNow()

	cases := []struct {
		name          string
		startedAgo    time.Duration
		policy        TimeoutPolicy
		expectRemoved bool
		expectRequeue time.Duration // 0 means "no requeue expected"
	}{
		{
			name: "within timeout requeues for remaining window",
			// Pod started 3m ago, readiness timeout 10m → 7m remaining.
			startedAgo:    3 * time.Minute,
			policy:        PolicyRemove,
			expectRemoved: false,
			expectRequeue: 7 * time.Minute,
		},
		{
			name:          "past timeout with policy=remove untaints",
			startedAgo:    11 * time.Minute,
			policy:        PolicyRemove,
			expectRemoved: true,
		},
		{
			name:          "past timeout with policy=keep does not untaint",
			startedAgo:    11 * time.Minute,
			policy:        PolicyKeep,
			expectRemoved: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := taintedNode(testNodeName, 30*time.Minute, now)
			pod := agentPod(testPodName, testPodNS, testNodeName, false, tc.startedAgo, now)
			c := newFakeClient(t, node, pod)
			r, _ := newReconciler(t, c, now, tc.policy, readiness, scheduling, false)

			result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
			require.NoError(t, err)
			if tc.expectRequeue > 0 {
				assert.Equal(t, tc.expectRequeue, result.RequeueAfter)
			}

			fresh := &corev1.Node{}
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
			if tc.expectRemoved {
				assert.False(t, hasTaint(fresh), "taint should be removed")
			} else {
				assert.True(t, hasTaint(fresh), "taint should remain")
			}
		})
	}
}

func TestReconcile_SchedulingTimeout(t *testing.T) {
	const readiness = 10 * time.Minute
	const scheduling = 5 * time.Minute
	now := testNow()

	cases := []struct {
		name          string
		nodeAge       time.Duration
		policy        TimeoutPolicy
		expectRemoved bool
		expectRequeue time.Duration
	}{
		{
			name:          "within scheduling timeout requeues",
			nodeAge:       2 * time.Minute,
			policy:        PolicyRemove,
			expectRemoved: false,
			expectRequeue: 3 * time.Minute,
		},
		{
			name:          "past scheduling timeout with policy=remove untaints",
			nodeAge:       6 * time.Minute,
			policy:        PolicyRemove,
			expectRemoved: true,
		},
		{
			name:          "past scheduling timeout with policy=keep does not untaint",
			nodeAge:       6 * time.Minute,
			policy:        PolicyKeep,
			expectRemoved: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := taintedNode(testNodeName, tc.nodeAge, now)
			c := newFakeClient(t, node)
			r, _ := newReconciler(t, c, now, tc.policy, readiness, scheduling, false)

			result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
			require.NoError(t, err)
			if tc.expectRequeue > 0 {
				assert.Equal(t, tc.expectRequeue, result.RequeueAfter)
			}

			fresh := &corev1.Node{}
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
			if tc.expectRemoved {
				assert.False(t, hasTaint(fresh))
			} else {
				assert.True(t, hasTaint(fresh))
			}
		})
	}
}

func TestReconcile_PodExistsWithoutStartTime_DoesNotFireSchedulingTimeout(t *testing.T) {
	// Edge case: a pod is scheduled on the node but Status.StartTime has not
	// been set yet (envtest, or kubelet pre-PodInitializing). The controller
	// must NOT fall through to the scheduling-timeout path: a pod IS
	// scheduled, so the scheduling timeout's precondition does not hold.
	// Expected behavior: requeue (readiness path), taint stays put even though
	// the node CreationTimestamp is well past the scheduling timeout.
	now := testNow()
	node := taintedNode(testNodeName, 1*time.Hour, now)                  // node old enough to trip scheduling
	pod := agentPod(testPodName, testPodNS, testNodeName, false, 0, now) // no StartTime
	c := newFakeClient(t, node, pod)
	r, _ := newReconciler(t, c, now, PolicyRemove, 10*time.Minute, 5*time.Minute, false)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Greater(t, result.RequeueAfter, time.Duration(0), "expected a requeue, not a no-op")

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.True(t, hasTaint(fresh), "scheduling timeout must NOT fire when any pod is present on the node")
}

func TestReconcile_ConflictReturnsRequeue(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	pod := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)

	base := newFakeClient(t, node, pod)
	c := interceptor.NewClient(base, interceptor.Funcs{
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, testNodeName, errors.New("race"))
		},
	})
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, false)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	assert.NoError(t, err, "conflict must not be returned as an error")
	assert.Equal(t, conflictRequeueDelay, result.RequeueAfter)
}

func TestReconcile_GenericPatchErrorBubbles(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	pod := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)

	base := newFakeClient(t, node, pod)
	c := interceptor.NewClient(base, interceptor.Funcs{
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return errors.New("boom")
		},
	})
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, false)

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	assert.Error(t, err)
}

func TestReconcile_CSI_enforceBothReadyRemovesTaint(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	csi := csiNodeServerPod("csi-1", testPodNS, testNodeName, true, 1*time.Minute, now)
	c := newFakeClient(t, node, agent, csi)
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, true)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.False(t, hasTaint(fresh))
}

func TestReconcile_CSI_agentReadyCsiNotReadyKeepsTaint(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	// Readiness clock uses the later of agent and CSI max(StartTime); CSI started
	// 30s ago stays inside readinessTimeout so we requeue instead of timing out.
	csi := csiNodeServerPod("csi-1", testPodNS, testNodeName, false, 30*time.Second, now)
	c := newFakeClient(t, node, agent, csi)
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, true)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Greater(t, result.RequeueAfter, time.Duration(0))

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.True(t, hasTaint(fresh))
}

func TestReconcile_CSI_bothPodsNotReady_readinessUsesLaterStartTime(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	// Agent started more recently than CSI; readiness clock must use the later
	// max(StartTime), not CSI-only.
	agent := agentPod(testPodName, testPodNS, testNodeName, false, 10*time.Second, now)
	csi := csiNodeServerPod("csi-1", testPodNS, testNodeName, false, 40*time.Second, now)
	c := newFakeClient(t, node, agent, csi)
	const readiness = time.Minute
	r, _ := newReconciler(t, c, now, PolicyRemove, readiness, 5*time.Minute, true)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Equal(t, 50*time.Second, result.RequeueAfter)

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.True(t, hasTaint(fresh))
}

func TestReconcile_CSI_agentReadyNoCsiPodKeepsTaint(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	c := newFakeClient(t, node, agent)
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, true)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Greater(t, result.RequeueAfter, time.Duration(0))

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.True(t, hasTaint(fresh))
}

func TestReconcile_CSI_agentReadyNoCsiPod_zeroCreationTimestampRequeuesSchedulingTimeout(t *testing.T) {
	now := testNow()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              testNodeName,
			CreationTimestamp: metav1.Time{}, // zero — defensive branch in schedulingTimeoutResult
		},
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{untaint.AgentNotReadyTaint()}},
	}
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	c := newFakeClient(t, node, agent)
	const scheduling = 7 * time.Minute
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, scheduling, true)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Equal(t, scheduling, result.RequeueAfter)

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	require.True(t, hasTaint(fresh))
}

func TestReconcile_CSI_agentReadyNoCsiPod_schedulingTimeoutRemovesTaint(t *testing.T) {
	now := testNow()
	const scheduling = 5 * time.Minute
	node := taintedNode(testNodeName, scheduling+time.Minute, now)
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	c := newFakeClient(t, node, agent)
	r, _ := newReconciler(t, c, now, PolicyRemove, 10*time.Minute, scheduling, true)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.False(t, hasTaint(fresh))
}

func TestReconcile_CSI_agentReadyCsiNotReady_noStartTimeCoarseReadinessRequeue(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	// CSI not Ready, startedAgo 0 → no Status.StartTime: both workloads are on
	// the node, so use the same coarse readiness requeue as agent-only (not
	// node-age scheduling), avoiding instant timeout on an old node.
	csi := csiNodeServerPod("csi-1", testPodNS, testNodeName, false, 0, now)
	c := newFakeClient(t, node, agent, csi)
	const readiness = 9 * time.Minute
	const scheduling = time.Minute
	r, _ := newReconciler(t, c, now, PolicyRemove, readiness, scheduling, true)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Equal(t, readiness, result.RequeueAfter)

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.True(t, hasTaint(fresh))
}

func TestReconcile_CSI_agentReadyCsiNotReady_readinessTimeoutRemovesTaint(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	const readiness = time.Minute
	csi := csiNodeServerPod("csi-1", testPodNS, testNodeName, false, 2*readiness, now)
	c := newFakeClient(t, node, agent, csi)
	r, _ := newReconciler(t, c, now, PolicyRemove, readiness, 5*time.Minute, true)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	fresh := &corev1.Node{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testNodeName}, fresh))
	assert.False(t, hasTaint(fresh))
}

func TestReconcile_CSI_listDriverPodsError(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	base := newFakeClient(t, node, agent)
	var listCalls atomic.Int32
	c := interceptor.NewClient(base, interceptor.Funcs{
		List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if listCalls.Add(1) == 2 {
				return errors.New("simulated CSI pod list failure")
			}
			return base.List(ctx, list, opts...)
		},
	})
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, true)

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list CSI driver pods")
}

func TestReconcile_CSI_bothReadyConflictReturnsRequeue(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	csi := csiNodeServerPod("csi-1", testPodNS, testNodeName, true, 1*time.Minute, now)
	base := newFakeClient(t, node, agent, csi)
	c := interceptor.NewClient(base, interceptor.Funcs{
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, testNodeName, errors.New("race"))
		},
	})
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, true)

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.NoError(t, err)
	assert.Equal(t, conflictRequeueDelay, result.RequeueAfter)
}

func TestReconcile_CSI_bothReadyPatchErrorBubbles(t *testing.T) {
	now := testNow()
	node := taintedNode(testNodeName, 0, now)
	agent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	csi := csiNodeServerPod("csi-1", testPodNS, testNodeName, true, 1*time.Minute, now)
	base := newFakeClient(t, node, agent, csi)
	c := interceptor.NewClient(base, interceptor.Funcs{
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return errors.New("boom")
		},
	})
	r, _ := newReconciler(t, c, now, PolicyRemove, time.Minute, time.Minute, true)

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testNodeName}})
	require.Error(t, err)
}

func TestPodWatchPredicate_withCSI(t *testing.T) {
	now := testNow()
	r, _ := newReconciler(t, newFakeClient(t), now, PolicyRemove, time.Minute, time.Minute, true)
	p := r.podWatchPredicate()
	readyAgent := agentPod(testPodName, testPodNS, testNodeName, true, 1*time.Minute, now)
	csi := csiNodeServerPod("csi-1", testPodNS, testNodeName, false, 1*time.Minute, now)
	other := nonAgentPod("x", testPodNS, testNodeName)
	assert.True(t, p.Create(event.CreateEvent{Object: csi}))
	assert.True(t, p.Create(event.CreateEvent{Object: readyAgent}))
	assert.False(t, p.Create(event.CreateEvent{Object: other}))
}
