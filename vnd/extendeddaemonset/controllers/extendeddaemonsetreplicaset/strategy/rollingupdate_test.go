// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package strategy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

func TestManageDeployment(t *testing.T) {
	now := time.Now()
	metaNow := metav1.NewTime(now)

	defaultRollingUpdate := &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{}
	defaultRollingUpdate = datadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyRollingUpdate(defaultRollingUpdate)

	logf.SetLogger(zap.New())
	testLogger := logf.Log.WithName("test")

	tests := []struct {
		name      string
		params    *Parameters
		daemonset *datadoghqv1alpha1.ExtendedDaemonSet
		want      *Result
		wantErr   bool
	}{
		{
			name: "default, no pods",
			params: &Parameters{
				Logger:    testLogger,
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{},
				Strategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
					RollingUpdate: *defaultRollingUpdate,
				},
				Replicaset: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							datadoghqv1alpha1.ExtendedDaemonSetReplicaSetUnreadyPodsAnnotationKey: "0",
						},
					},
					Status: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
						Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
							{
								Type:               datadoghqv1alpha1.ConditionTypeActive,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metaNow,
							},
						},
					},
				},
			},
			daemonset: &datadoghqv1alpha1.ExtendedDaemonSet{},
			want: &Result{
				PodsToCreate: []*NodeItem{},
				PodsToDelete: []*NodeItem{},
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
					Status: "active",
					Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
						{
							Type:               datadoghqv1alpha1.ConditionTypeActive,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metaNow,
							LastUpdateTime:     metaNow,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "default, with one pod to create",
			params: &Parameters{
				Logger:    testLogger,
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{},
				Strategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
					RollingUpdate: *defaultRollingUpdate,
				},
				Replicaset: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							datadoghqv1alpha1.ExtendedDaemonSetReplicaSetUnreadyPodsAnnotationKey: "0",
						},
					},
					Status: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
						Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
							{
								Type:               datadoghqv1alpha1.ConditionTypeActive,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metaNow,
							},
						},
					},
				},
				PodByNodeName: map[*NodeItem]*corev1.Pod{
					testCanaryNodes["a"]: newTestPodOnNode("foo-a", "a", "v1", readyPodStatus),
					testCanaryNodes["b"]: nil,
				},
			},
			daemonset: &datadoghqv1alpha1.ExtendedDaemonSet{},
			want: &Result{
				PodsToCreate: []*NodeItem{
					testCanaryNodes["b"],
				},
				PodsToDelete: []*NodeItem{},
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
					Status:  "active",
					Desired: 2,
					Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
						{
							Type:               datadoghqv1alpha1.ConditionTypeActive,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metaNow,
							LastUpdateTime:     metaNow,
						},
					},
				},
				Result: reconcile.Result{
					Requeue: true,
				},
			},
			wantErr: false,
		},
		{
			name: "default, with one pod present and one pod to create",
			params: &Parameters{
				Logger:    testLogger,
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{},
				Strategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
					RollingUpdate: *defaultRollingUpdate,
				},
				Replicaset: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							datadoghqv1alpha1.ExtendedDaemonSetReplicaSetUnreadyPodsAnnotationKey: "0",
						},
					},
					Status: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
						Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
							{
								Type:               datadoghqv1alpha1.ConditionTypeActive,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metaNow,
							},
						},
					},
					Spec: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetSpec{
						TemplateGeneration: "v1",
					},
				},
				PodByNodeName: map[*NodeItem]*corev1.Pod{
					testCanaryNodes["a"]: newTestPodOnNode("foo-a", "a", "v1", readyPodStatus),
					testCanaryNodes["b"]: nil,
				},
			},
			daemonset: &datadoghqv1alpha1.ExtendedDaemonSet{},
			want: &Result{
				PodsToCreate: []*NodeItem{
					testCanaryNodes["b"],
				},
				PodsToDelete: []*NodeItem{},
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
					Status:    "active",
					Desired:   2,
					Current:   1,
					Ready:     1,
					Available: 1,
					Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
						{
							Type:               datadoghqv1alpha1.ConditionTypeActive,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metaNow,
							LastUpdateTime:     metaNow,
						},
					},
				},
				Result: reconcile.Result{
					Requeue: true,
				},
			},
			wantErr: false,
		},
		{
			name: "default, with one pod that changed and one pod to create",
			params: &Parameters{
				Logger:    testLogger,
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{},
				Strategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
					RollingUpdate: *defaultRollingUpdate,
				},
				Replicaset: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							datadoghqv1alpha1.ExtendedDaemonSetReplicaSetUnreadyPodsAnnotationKey: "0",
						},
					},
					Status: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
						Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
							{
								Type:               datadoghqv1alpha1.ConditionTypeActive,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metaNow,
							},
						},
					},
					Spec: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetSpec{
						TemplateGeneration: "v1",
					},
				},
				PodByNodeName: map[*NodeItem]*corev1.Pod{
					testCanaryNodes["a"]: newTestPodOnNode("foo-a", "a", "v2", readyPodStatus),
					testCanaryNodes["b"]: nil,
				},
			},
			daemonset: &datadoghqv1alpha1.ExtendedDaemonSet{},
			want: &Result{
				PodsToCreate: []*NodeItem{
					testCanaryNodes["b"],
				},
				PodsToDelete: []*NodeItem{},
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
					Status:    "active",
					Desired:   2,
					Current:   0,
					Ready:     0,
					Available: 0,
					Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
						{
							Type:               datadoghqv1alpha1.ConditionTypeActive,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metaNow,
							LastUpdateTime:     metaNow,
						},
					},
				},
				Result: reconcile.Result{
					Requeue: true,
				},
			},
			wantErr: false,
		},
		{
			name: "default, with one pod that changed and one pod to delete....",
			params: &Parameters{
				Logger:    testLogger,
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{},
				Strategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
					RollingUpdate: *defaultRollingUpdate,
				},
				Replicaset: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							datadoghqv1alpha1.ExtendedDaemonSetReplicaSetUnreadyPodsAnnotationKey: "0",
						},
					},
					Status: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
						Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
							{
								Type:               datadoghqv1alpha1.ConditionTypeActive,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: metaNow,
							},
						},
					},
					Spec: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetSpec{
						TemplateGeneration: "v1",
					},
				},
				PodByNodeName: map[*NodeItem]*corev1.Pod{
					testCanaryNodes["a"]: withDeletionTimestamp(newTestPodOnNode("foo-a", "a", "v2", readyPodStatus)),
					testCanaryNodes["b"]: nil,
				},
			},
			daemonset: &datadoghqv1alpha1.ExtendedDaemonSet{},
			want: &Result{
				PodsToCreate: []*NodeItem{
					testCanaryNodes["b"],
				},
				PodsToDelete: []*NodeItem{},
				NewStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
					Status:    "active",
					Desired:   2,
					Current:   0,
					Ready:     0,
					Available: 0,
					Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
						{
							Type:               datadoghqv1alpha1.ConditionTypeActive,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metaNow,
							LastUpdateTime:     metaNow,
						},
					},
				},
				Result: reconcile.Result{
					Requeue: true,
				},
			},
			wantErr: false,
		},
	}
	client := fake.NewClientBuilder().Build()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ManageDeployment(client, tt.daemonset, tt.params, metaNow)
			if !tt.wantErr {
				require.NoError(t, err, "ManageDeployment() error = %v", err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_calculateMaxCreation(t *testing.T) {
	now := time.Now()

	defaultParams := &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{}
	defaultParams = datadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyRollingUpdate(defaultParams)
	type args struct {
		params      *datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate
		nbNodes     int
		rsStartTime time.Time
		now         time.Time
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "startTime",
			args: args{
				nbNodes:     100,
				now:         now,
				params:      defaultParams,
				rsStartTime: now,
			},
			want:    1,
			wantErr: false,
		},
		{
			name: "2min later, with default strategy",
			args: args{
				nbNodes:     100,
				now:         now,
				params:      defaultParams,
				rsStartTime: now.Add(-2 * time.Minute),
			},
			want:    3,
			wantErr: false,
		},
		{
			name: "2min later, with default strategy",
			args: args{
				nbNodes: 100,
				now:     now,
				params: datadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyRollingUpdate(
					&datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
						SlowStartAdditiveIncrease: intstr.ValueOrDefault(nil, intstr.FromInt(2)),
					},
				),
				rsStartTime: now.Add(-2 * time.Minute),
			},
			want:    6,
			wantErr: false,
		},
		{
			name: "5min later, with default strategy",
			args: args{
				nbNodes: 100,
				now:     now,
				params: datadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyRollingUpdate(
					&datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
						SlowStartAdditiveIncrease: intstr.ValueOrDefault(nil, intstr.FromInt(10)),
					},
				),
				rsStartTime: now.Add(-5 * time.Minute),
			},
			want:    60,
			wantErr: false,
		},
		{
			name: "value parse error",
			args: args{
				nbNodes: 100,
				now:     now,
				params: datadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyRollingUpdate(
					&datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
						SlowStartAdditiveIncrease: intstr.ValueOrDefault(nil, intstr.FromString("10$")),
					},
				),
				rsStartTime: now.Add(-5 * time.Minute),
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "10min later, max // pods",
			args: args{
				nbNodes: 100,
				now:     now,
				params: datadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyRollingUpdate(
					&datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
						SlowStartAdditiveIncrease: intstr.ValueOrDefault(nil, intstr.FromInt(30)),
					},
				),
				rsStartTime: now.Add(-10 * time.Minute),
			},
			want:    250,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateMaxCreation(tt.args.params, tt.args.nbNodes, tt.args.rsStartTime, tt.args.now)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateMaxCreation() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if got != tt.want {
				t.Errorf("calculateMaxCreation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getRollingUpdateStartTime(t *testing.T) {
	now := time.Now()
	oneMinuteAgo := now.Add(-time.Minute)

	tests := []struct {
		name      string
		ersStatus *datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus
		time      time.Time
		want      time.Time
	}{
		{
			name:      "nil case",
			ersStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{},
			time:      now,
			want:      now,
		},
		{
			name: "non-active condition only",
			ersStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:   datadoghqv1alpha1.ConditionTypeRollingUpdatePaused,
						Status: corev1.ConditionTrue,
					},
				},
			},
			time: now,
			want: now,
		},
		{
			name: "active condition is false",
			ersStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:   datadoghqv1alpha1.ConditionTypeActive,
						Status: corev1.ConditionFalse,
					},
				},
			},
			time: now,
			want: now,
		},
		{
			name: "active condition is true",
			ersStatus: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
				Conditions: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
					{
						Type:               datadoghqv1alpha1.ConditionTypeActive,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.Time{Time: oneMinuteAgo},
					},
				},
			},
			time: now,
			want: oneMinuteAgo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRollingUpdateStartTime(tt.ersStatus, tt.time)
			assert.Equal(t, tt.want, got)
		})
	}
}
