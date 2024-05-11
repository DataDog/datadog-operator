// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package strategy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
	datadoghqv1alpha1test "github.com/DataDog/extendeddaemonset/api/v1alpha1/test"
)

var (
	testLogger          = logf.Log.WithName("test")
	testCanaryNodeNames = []string{"a", "b", "c"}
	testCanaryNodes     = map[string]*NodeItem{
		"a": {
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "a",
				},
			},
		},
		"b": {
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "b",
				},
			},
		},
		"c": {
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c",
				},
			},
		},
	}
	readyPodStatus = v1.PodStatus{
		Conditions: []v1.PodCondition{
			{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			},
		},
		StartTime: &metav1.Time{Time: time.Now()},
	}
)

func newTestPodOnNode(name, nodeName, hash string, status v1.PodStatus) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"extendeddaemonset.datadoghq.com/templatehash": hash,
			},
		},
		Spec: v1.PodSpec{
			NodeName: nodeName,
		},
		Status: status,
	}
}

func newTestCanaryPod(name, hash string, status v1.PodStatus) *v1.Pod {
	return newTestPodOnNode(name, "", hash, status)
}

func podTerminatedStatus(restartCount int32, reason string, time time.Time) v1.PodStatus {
	return v1.PodStatus{
		ContainerStatuses: []v1.ContainerStatus{
			{
				Name:         "restarting",
				RestartCount: restartCount,
				LastTerminationState: v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{
						Reason:     reason,
						FinishedAt: metav1.NewTime(time),
					},
				},
			},
		},
	}
}

func podWaitingStatus(reason, message string, time time.Time) v1.PodStatus {
	start := metav1.NewTime(time)

	return v1.PodStatus{
		ContainerStatuses: []v1.ContainerStatus{
			{
				Name: "waiting",
				State: v1.ContainerState{
					Waiting: &v1.ContainerStateWaiting{

						Reason:  reason,
						Message: message,
					},
				},
			},
		},
		StartTime: &start,
	}
}

func withDeletionTimestamp(pod *v1.Pod) *v1.Pod {
	ts := metav1.Now()
	pod.DeletionTimestamp = &ts

	return pod
}

func withHostIP(pod *v1.Pod, ip string) *v1.Pod {
	pod.Status.HostIP = ip

	return pod
}

type canaryStatusTest struct {
	annotations map[string]string
	params      *Parameters
	result      *Result
	now         time.Time
}

func (test *canaryStatusTest) Run(t *testing.T) {
	if test.now.IsZero() {
		test.now = time.Now()
	}
	result := manageCanaryStatus(test.annotations, test.params, test.now)
	assert.Equal(t, test.result, result)
}

func TestManageCanaryStatus_NoRestartsAndPodsToCreate(t *testing.T) {
	test := canaryStatusTest{
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", readyPodStatus),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			PodsToCreate: []*NodeItem{
				testCanaryNodes["b"],
				testCanaryNodes["c"],
			},
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   1,
				Ready:     1,
				Available: 1,
			},
			Result: requeuePromptly(),
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_NoRestartsAndNoPodsToCreate(t *testing.T) {
	test := canaryStatusTest{
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(3),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", readyPodStatus),
				testCanaryNodes["b"]: newTestCanaryPod("foo-b", "v1", readyPodStatus),
				testCanaryNodes["c"]: newTestCanaryPod("foo-c", "v1", readyPodStatus),
			},
			Logger: testLogger,
		},
		result: &Result{
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   3,
				Ready:     3,
				Available: 3,
			},
			Result: reconcile.Result{},
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_NoRestartsAndPodWithDeletionTimestamp(t *testing.T) {
	test := canaryStatusTest{
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: withHostIP(withDeletionTimestamp(newTestCanaryPod("foo-a", "v1", readyPodStatus)), "1.2.3.4"),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			PodsToCreate: []*NodeItem{
				testCanaryNodes["b"],
				testCanaryNodes["c"],
			},
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   0,
				Ready:     0,
				Available: 0,
			},
			Result: requeuePromptly(),
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_NoRestartsAndPodsToDelete(t *testing.T) {
	test := canaryStatusTest{
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v0", readyPodStatus),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			PodsToCreate: []*NodeItem{
				testCanaryNodes["b"],
				testCanaryNodes["c"],
			},
			PodsToDelete: []*NodeItem{
				testCanaryNodes["a"],
			},
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   0,
				Ready:     0,
				Available: 0,
			},
			Result: requeuePromptly(),
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_HighRestartsLeadingToPause(t *testing.T) {
	now := time.Now()
	restartedAt := now.Add(-time.Minute)
	test := canaryStatusTest{
		now: now,
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", podTerminatedStatus(3, "CrashLoopBackOff", restartedAt)),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   1,
				Ready:     0,
				Available: 0,
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypeCanaryPaused,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
						Reason:             "CrashLoopBackOff",
						Message:            "",
					},
					{
						Type:               v1alpha1.ConditionTypePodRestarting,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(restartedAt),
						LastUpdateTime:     metav1.NewTime(restartedAt),
						Message:            "Pod foo-a restarting with reason: CrashLoopBackOff",
					},
				},
			},
			IsPaused:     true,
			PausedReason: v1alpha1.ExtendedDaemonSetStatusReasonCLB,
			Result:       reconcile.Result{},
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_CanaryPausedAlready(t *testing.T) {
	now := time.Now()
	test := canaryStatusTest{
		now: now,
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(false),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
				Status: v1alpha1.ExtendedDaemonSetReplicaSetStatus{
					Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
						{
							Type:               v1alpha1.ConditionTypeCanaryPaused,
							Status:             v1.ConditionTrue,
							LastTransitionTime: metav1.NewTime(now),
							LastUpdateTime:     metav1.NewTime(now),
							Reason:             "CrashLoopBackOff",
						},
					},
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: nil,
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   0,
				Ready:     0,
				Available: 0,
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypeCanaryPaused,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
						Reason:             "CrashLoopBackOff",
					},
				},
			},
			IsPaused:     true,
			PausedReason: v1alpha1.ExtendedDaemonSetStatusReasonCLB,
			Result:       reconcile.Result{},
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_HighRestartsLeadingToFail(t *testing.T) {
	now := time.Now()
	restartedAt := now.Add(-time.Minute)
	test := canaryStatusTest{
		now: now,
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", podTerminatedStatus(6, "CrashLoopBackOff", restartedAt)),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary-failed",
				Desired:   3,
				Current:   1,
				Ready:     0,
				Available: 0,
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypeCanaryFailed,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
						Reason:             "CrashLoopBackOff",
						Message:            "",
					},
					{
						Type:               v1alpha1.ConditionTypePodRestarting,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(restartedAt),
						LastUpdateTime:     metav1.NewTime(restartedAt),
						Message:            "Pod foo-a restarting with reason: CrashLoopBackOff",
					},
				},
			},
			IsFailed:     true,
			FailedReason: v1alpha1.ExtendedDaemonSetStatusReasonCLB,
			Result:       reconcile.Result{},
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_LongRestartsDurationLeadingToFail(t *testing.T) {
	now := time.Now()
	restartsStartedAt := now.Add(-time.Hour)
	restartsUpdatedAt := now.Add(-10 * time.Minute)
	restartedAt := now.Add(-time.Minute)

	test := canaryStatusTest{
		now: now,
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:             v1alpha1.NewBool(true),
						MaxRestarts:         v1alpha1.NewInt32(5),
						MaxRestartsDuration: &metav1.Duration{Duration: 20 * time.Minute},
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypePodRestarting,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(restartsStartedAt),
						LastUpdateTime:     metav1.NewTime(restartsUpdatedAt),
						Message:            "Pod foo-b restarting with reason: CrashLoopBackOff",
					},
				},
			},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", podTerminatedStatus(1, "CrashLoopBackOff", restartedAt)),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary-failed",
				Desired:   3,
				Current:   1,
				Ready:     0,
				Available: 0,
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypePodRestarting,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(restartsStartedAt),
						LastUpdateTime:     metav1.NewTime(restartedAt),
						Message:            "Pod foo-a restarting with reason: CrashLoopBackOff",
					},
					{
						Type:               v1alpha1.ConditionTypeCanaryFailed,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
						Reason:             string(v1alpha1.ExtendedDaemonSetStatusRestartsTimeoutExceeded),
						Message:            "",
					},
				},
			},
			IsFailed:     true,
			FailedReason: v1alpha1.ExtendedDaemonSetStatusRestartsTimeoutExceeded,
			Result:       reconcile.Result{},
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_LongDurationLeadingToFail(t *testing.T) {
	now := time.Now()
	imageErrorStartedAt := now.Add(-time.Hour)

	test := canaryStatusTest{
		now: now,
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					Duration: &metav1.Duration{Duration: 10 * time.Minute},
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:       v1alpha1.NewBool(true),
						MaxRestarts:   v1alpha1.NewInt32(5),
						CanaryTimeout: &metav1.Duration{Duration: 20 * time.Minute},
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypeCanary,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(imageErrorStartedAt),
						LastUpdateTime:     metav1.NewTime(imageErrorStartedAt),
						Message:            "",
					},
					{
						Type:               v1alpha1.ConditionTypeCanaryPaused,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(imageErrorStartedAt),
						LastUpdateTime:     metav1.NewTime(imageErrorStartedAt),
						Message:            "ImagePullBackOff",
					},
				},
			},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", podWaitingStatus("ImagePullBackOff", `Back-off pulling image "gcr.io/missing"`, imageErrorStartedAt)),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary-failed",
				Desired:   3,
				Current:   1,
				Ready:     0,
				Available: 0,
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypeCanary,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(imageErrorStartedAt),
						LastUpdateTime:     metav1.NewTime(imageErrorStartedAt),
						Message:            "",
					},
					{
						Type:               v1alpha1.ConditionTypeCanaryPaused,
						Status:             v1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
						Reason:             "",
						Message:            "ImagePullBackOff",
					},
					{
						Type:               v1alpha1.ConditionTypeCanaryFailed,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
						Reason:             string(v1alpha1.ExtendedDaemonSetStatusTimeoutExceeded),
						Message:            "",
					},
					{
						Type:               v1alpha1.ConditionTypePodCannotStart,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
						Reason:             "ImagePullBackOff",
						Message:            "Pod foo-a cannot start with reason: ImagePullBackOff",
					},
				},
			},
			IsFailed:     true,
			FailedReason: v1alpha1.ExtendedDaemonSetStatusTimeoutExceeded,
			Result:       reconcile.Result{},
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_ImagePullErrorLeadingToPause(t *testing.T) {
	now := time.Now()
	test := canaryStatusTest{
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", podWaitingStatus("ImagePullBackOff", `Back-off pulling image "gcr.io/missing"`, now)),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   1,
				Ready:     0,
				Available: 0,
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypeCanaryPaused,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
						Reason:             "ImagePullBackOff",
					},
					{
						Type:               v1alpha1.ConditionTypePodCannotStart,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
						Reason:             "ImagePullBackOff",
						Message:            "Pod foo-a cannot start with reason: ImagePullBackOff",
					},
				},
			},
			IsPaused:     true,
			PausedReason: v1alpha1.ExtendedDaemonSetStatusReason("ImagePullBackOff"),
			Result:       reconcile.Result{},
		},
		now: now,
	}
	test.Run(t)
}

func TestManageCanaryStatus_AutoPausePendingCreate(t *testing.T) {
	now := time.Now()
	test := canaryStatusTest{
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:              v1alpha1.NewBool(true),
						MaxRestarts:          v1alpha1.NewInt32(2),
						MaxSlowStartDuration: &metav1.Duration{Duration: 1 * time.Minute},
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", readyPodStatus),
				testCanaryNodes["b"]: newTestCanaryPod("foo-b", "v1", podWaitingStatus("ContainerCreating", "Creating...", now)),
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			PodsToCreate: []*NodeItem{
				testCanaryNodes["c"],
			},
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   2,
				Ready:     1,
				Available: 1,
			},
			Result: requeuePromptly(),
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_AutoPauseWithMaxSlowStart(t *testing.T) {
	now := time.Now()
	afterNow := now.Add(2 * time.Minute)
	test := canaryStatusTest{
		now: afterNow,
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:              v1alpha1.NewBool(true),
						MaxRestarts:          v1alpha1.NewInt32(2),
						MaxSlowStartDuration: &metav1.Duration{Duration: 1 * time.Minute},
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", readyPodStatus),
				testCanaryNodes["b"]: newTestCanaryPod("foo-b", "v1", podWaitingStatus("ContainerCreating", "Creating...", now)),
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   2,
				Ready:     1,
				Available: 1,
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypeCanaryPaused,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(afterNow),
						LastUpdateTime:     metav1.NewTime(afterNow),
						Reason:             "SlowStartTimeoutExceeded",
						Message:            "",
					},
					{
						Type:               v1alpha1.ConditionTypePodCannotStart,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(afterNow),
						LastUpdateTime:     metav1.NewTime(afterNow),
						Reason:             "SlowStartTimeoutExceeded",
						Message:            "Pod foo-b cannot start with reason: SlowStartTimeoutExceeded",
					},
				},
			},
			IsPaused:     true,
			PausedReason: "SlowStartTimeoutExceeded",
			Result:       reconcile.Result{},
		},
	}
	test.Run(t)
}

func TestManageCanaryStatus_Unpaused(t *testing.T) {
	test := canaryStatusTest{
		annotations: map[string]string{
			v1alpha1.ExtendedDaemonSetCanaryUnpausedAnnotationKey: v1alpha1.ValueStringTrue,
		},
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(2),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", readyPodStatus),
				testCanaryNodes["b"]: newTestCanaryPod("foo-b", "v1", readyPodStatus),
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			PodsToCreate: []*NodeItem{
				testCanaryNodes["c"],
			},
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   2,
				Ready:     2,
				Available: 2,
			},
			IsUnpaused: true,
			Result:     requeuePromptly(),
		},
	}
	test.Run(t)
}

func Test_ensureCanaryPodLabels(t *testing.T) {
	node1 := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
		},
	}
	nodeItem1 := NodeItem{
		Node: &node1,
	}

	pod1 := newTestCanaryPod("foo-a", "v1", readyPodStatus)
	pod1.Labels = labels.Set{
		"foo": "bar",
		v1alpha1.ExtendedDaemonSetReplicaSetNameLabelKey: "baz",
	}

	nodeByName := make(map[string]*NodeItem)
	nodeByName["node1"] = &nodeItem1
	podByNodeName := make(map[*NodeItem]*v1.Pod)
	podByNodeName[&nodeItem1] = pod1
	params := Parameters{
		CanaryNodes:   []string{"node1"},
		NodeByName:    nodeByName,
		PodByNodeName: podByNodeName,
		Replicaset:    datadoghqv1alpha1test.NewExtendedDaemonSetReplicaSet("foo", "baz", nil),
		Logger:        testLogger,
	}

	// Test case 1, pod exists
	client := fake.NewClientBuilder().WithStatusSubresource(&v1.Pod{}).WithObjects(pod1).Build()
	t.Run("Pod exists", func(t *testing.T) {
		err := ensureCanaryPodLabels(client, &params)
		if err != nil {
			t.Errorf("Error should be nil, but it is: %v", err)
		}
	})

	// Test case 2, pod does not exist
	client = fake.NewClientBuilder().Build()
	t.Run("Pod does not exist", func(t *testing.T) {
		err := ensureCanaryPodLabels(client, &params)
		if err == nil {
			t.Errorf("Error should not be nil, but it is")
		}
	})
}

func TestCreateContainerConfigError_ExceedsMaxSlowStartCondition(t *testing.T) {
	now := time.Now()
	afterNow := now.Add(2 * time.Minute)
	test := canaryStatusTest{
		now: afterNow,
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:              v1alpha1.NewBool(true),
						MaxRestarts:          v1alpha1.NewInt32(2),
						MaxSlowStartDuration: &metav1.Duration{Duration: 1 * time.Minute},
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", podWaitingStatus("CreateContainerConfigError", `Error creating container CreateContainerConfigError"`, now)),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   1,
				Ready:     0,
				Available: 0,
				Conditions: []v1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               v1alpha1.ConditionTypeCanaryPaused,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(afterNow),
						LastUpdateTime:     metav1.NewTime(afterNow),
						Reason:             "CreateContainerConfigError",
						Message:            "",
					},
					{
						Type:               v1alpha1.ConditionTypePodCannotStart,
						Status:             v1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(afterNow),
						LastUpdateTime:     metav1.NewTime(afterNow),
						Reason:             "CreateContainerConfigError",
						Message:            "Pod foo-a cannot start with reason: CreateContainerConfigError",
					},
				},
			},
			IsPaused:     true,
			PausedReason: v1alpha1.ExtendedDaemonSetStatusReason("CreateContainerConfigError"),
			Result:       reconcile.Result{},
		},
	}
	test.Run(t)
}

func TestCreateContainerConfigError_WithinMaxSlowStartDuration(t *testing.T) {
	now := time.Now()
	afterNow := now.Add(2 * time.Minute)
	test := canaryStatusTest{
		now: afterNow,
		params: &Parameters{
			EDSName: "foo",
			Strategy: &v1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary: &v1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					AutoPause: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:              v1alpha1.NewBool(true),
						MaxRestarts:          v1alpha1.NewInt32(2),
						MaxSlowStartDuration: &metav1.Duration{Duration: 5 * time.Minute},
					},
					AutoFail: &v1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     v1alpha1.NewBool(true),
						MaxRestarts: v1alpha1.NewInt32(5),
					},
				},
			},
			Replicaset: &v1alpha1.ExtendedDaemonSetReplicaSet{
				Spec: v1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "v1",
				},
			},
			NewStatus:   &v1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			CanaryNodes: testCanaryNodeNames,
			NodeByName:  testCanaryNodes,
			PodByNodeName: map[*NodeItem]*v1.Pod{
				testCanaryNodes["a"]: newTestCanaryPod("foo-a", "v1", podWaitingStatus("CreateContainerConfigError", `Error creating container CreateContainerConfigError"`, now)),
				testCanaryNodes["b"]: nil,
				testCanaryNodes["c"]: nil,
			},
			Logger: testLogger,
		},
		result: &Result{
			PodsToCreate: []*NodeItem{
				testCanaryNodes["b"],
				testCanaryNodes["c"],
			},
			NewStatus: &v1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Status:    "canary",
				Desired:   3,
				Current:   1,
				Ready:     0,
				Available: 0,
			},
			Result: requeuePromptly(),
		},
	}
	test.Run(t)
}
