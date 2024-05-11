// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonset

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	cmp "github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	test "github.com/DataDog/extendeddaemonset/api/v1alpha1/test"
	commontest "github.com/DataDog/extendeddaemonset/pkg/controller/test"
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils/comparison"
)

var testLogger logr.Logger = logf.Log.WithName("test")

func TestReconciler_selectNodes(t *testing.T) {
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSet{})

	nodeOptions := &commontest.NewNodeOptions{
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	node1 := commontest.NewNode("node1", nodeOptions)
	node2 := commontest.NewNode("node2", nodeOptions)
	node3 := commontest.NewNode("node3", nodeOptions)
	intString3 := intstr.FromInt(3)

	node2.Labels = map[string]string{
		"canary": "true",
	}

	options1 := &test.NewExtendedDaemonSetOptions{
		Canary: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
			Replicas: &intString3,
		},
		Status: &datadoghqv1alpha1.ExtendedDaemonSetStatus{
			ActiveReplicaSet: "foo-1",
			Canary: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
				ReplicaSet: "foo-2",
				Nodes:      []string{},
			},
		},
	}
	extendeddaemonset1 := test.NewExtendedDaemonSet("bar", "foo", options1)

	intString1 := intstr.FromInt(1)

	options2 := &test.NewExtendedDaemonSetOptions{
		Canary: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
			Replicas: &intString1,
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"canary": "true",
				},
			},
		},
		Status: &datadoghqv1alpha1.ExtendedDaemonSetStatus{
			ActiveReplicaSet: "foo-1",
			Canary: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
				ReplicaSet: "foo-2",
				Nodes:      []string{},
			},
		},
	}
	extendeddaemonset2 := test.NewExtendedDaemonSet("bar", "foo", options2)

	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}
	type args struct {
		spec         *datadoghqv1alpha1.ExtendedDaemonSetSpec
		replicaset   *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		canaryStatus *datadoghqv1alpha1.ExtendedDaemonSetStatusCanary
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		wantErr  bool
		wantFunc func(*datadoghqv1alpha1.ExtendedDaemonSetStatusCanary) bool
	}{
		{
			name: "enough nodes",
			fields: fields{
				scheme: s,
				client: fake.NewClientBuilder().WithStatusSubresource(&corev1.Node{}).WithObjects(node1, node2, node3).Build(),
			},
			args: args{
				spec:       &extendeddaemonset1.Spec,
				replicaset: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{},
				canaryStatus: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
					ReplicaSet: "foo",
					Nodes:      []string{},
				},
			},
			wantErr: false,
		},
		{
			name: "missing nodes",
			fields: fields{
				scheme: s,
				client: fake.NewClientBuilder().WithStatusSubresource(&corev1.Node{}).WithObjects(node1, node2).Build(),
			},
			args: args{
				spec:       &extendeddaemonset1.Spec,
				replicaset: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{},
				canaryStatus: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
					ReplicaSet: "foo",
					Nodes:      []string{},
				},
			},
			wantErr: true,
		},
		{
			name: "enough nodes",
			fields: fields{
				scheme: s,
				client: fake.NewClientBuilder().WithStatusSubresource(node1, node2, node3).WithObjects(node1, node2, node3).Build(),
			},
			args: args{
				spec:       &extendeddaemonset1.Spec,
				replicaset: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{},
				canaryStatus: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
					ReplicaSet: "foo",
					Nodes:      []string{node1.Name},
				},
			},
			wantErr: false,
		},
		{
			name: "dedicated canary nodes",
			fields: fields{
				scheme: s,
				client: fake.NewClientBuilder().WithStatusSubresource(&corev1.Node{}).WithObjects(node1, node2, node3).Build(),
			},
			args: args{
				spec:       &extendeddaemonset2.Spec,
				replicaset: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{},
				canaryStatus: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
					ReplicaSet: "foo",
					Nodes:      []string{},
				},
			},
			wantErr: false,
			wantFunc: func(canaryStatus *datadoghqv1alpha1.ExtendedDaemonSetStatusCanary) bool {
				return len(canaryStatus.Nodes) == 1 && canaryStatus.Nodes[0] == "node2"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqLogger := testLogger.WithValues("test:", tt.name)
			r := &Reconciler{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
				log:    testLogger,
			}
			if err := r.selectNodes(reqLogger, tt.args.spec, tt.args.replicaset, tt.args.canaryStatus); (err != nil) != tt.wantErr {
				t.Errorf("Reconciler.selectNodes() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantFunc != nil && !tt.wantFunc(tt.args.canaryStatus) {
				t.Errorf("ReconcileExtendedDaemonSet.selectNodes() didn’t pass the post-run checks")
			}
		})
	}
}

func Test_newReplicaSetFromInstance(t *testing.T) {
	tests := []struct {
		name      string
		daemonset *datadoghqv1alpha1.ExtendedDaemonSet
		want      *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		wantErr   bool
	}{
		{
			name:      "default test",
			daemonset: test.NewExtendedDaemonSet("bar", "foo", nil),
			wantErr:   false,
			want: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "bar",
					GenerateName: "foo-",
					Labels:       map[string]string{"extendeddaemonset.datadoghq.com/name": "foo"},
					Annotations:  map[string]string{"extendeddaemonset.datadoghq.com/templatehash": "a2bb34618483323482d9a56ae2515eed"},
				},
				Spec: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "a2bb34618483323482d9a56ae2515eed",
				},
			},
		},
		{
			name:      "with label",
			daemonset: test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}}),
			wantErr:   false,
			want: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "bar",
					GenerateName: "foo-",
					Labels:       map[string]string{"foo-key": "bar-value", "extendeddaemonset.datadoghq.com/name": "foo"},
					Annotations:  map[string]string{"extendeddaemonset.datadoghq.com/templatehash": "a2bb34618483323482d9a56ae2515eed"},
				},
				Spec: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetSpec{
					TemplateGeneration: "a2bb34618483323482d9a56ae2515eed",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newReplicaSetFromInstance(tt.daemonset)
			if (err != nil) != tt.wantErr {
				t.Errorf("newReplicaSetFromInstance() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !apiequality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("newReplicaSetFromInstance() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func Test_selectCurrentReplicaSet(t *testing.T) {
	now := time.Now()
	t.Logf("now: %v", now)
	creationTimeDaemonset := now.Add(-10 * time.Minute)
	creationTimeRSDone := now.Add(-6 * time.Minute)

	replicassetUpToDate := test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
		CreationTime: &now,
		Labels:       map[string]string{"foo-key": "bar-value"},
	})

	replicassetUpToDateDone := test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
		CreationTime: &creationTimeRSDone,
		Labels:       map[string]string{"foo-key": "bar-value"},
	})
	replicassetOld := test.NewExtendedDaemonSetReplicaSet("bar", "foo-old", &test.NewExtendedDaemonSetReplicaSetOptions{
		CreationTime: &creationTimeDaemonset,
		Labels:       map[string]string{"foo-key": "old-value"},
	})

	daemonset := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}})
	intString1 := intstr.FromInt(1)
	daemonsetWithCanary := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{
		CreationTime: &creationTimeDaemonset,
		Labels:       map[string]string{"foo-key": "bar-value"},
		Canary: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
			Replicas: &intString1,
			Duration: &metav1.Duration{Duration: 5 * time.Minute},
		},
		Status: &datadoghqv1alpha1.ExtendedDaemonSetStatus{
			ActiveReplicaSet: replicassetOld.Name,
		},
	})
	daemonsetWithCanaryValid := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{
		CreationTime: &creationTimeDaemonset,
		Labels:       map[string]string{"foo-key": "bar-value"},
		Annotations:  map[string]string{datadoghqv1alpha1.ExtendedDaemonSetCanaryValidAnnotationKey: "foo-1"},
		Canary: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
			Replicas: &intString1,
			Duration: &metav1.Duration{Duration: 5 * time.Minute},
		},
		Status: &datadoghqv1alpha1.ExtendedDaemonSetStatus{
			ActiveReplicaSet: replicassetOld.Name,
		},
	})
	daemonsetWithCanaryPaused := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{
		CreationTime: &creationTimeDaemonset,
		Labels:       map[string]string{"foo-key": "bar-value"},
		Annotations:  map[string]string{datadoghqv1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey: "true"},
		Canary: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
			Replicas: &intString1,
			Duration: &metav1.Duration{Duration: 5 * time.Minute},
		},
		Status: &datadoghqv1alpha1.ExtendedDaemonSetStatus{
			ActiveReplicaSet: replicassetOld.Name,
		},
	})

	type args struct {
		daemonset  *datadoghqv1alpha1.ExtendedDaemonSet
		upToDateRS *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		activeRS   *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		now        time.Time
	}
	tests := []struct {
		name  string
		args  args
		want  *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		want1 time.Duration
	}{
		{
			name: "one RS, update to date",
			args: args{
				daemonset:  daemonset,
				upToDateRS: replicassetUpToDate,
				activeRS:   replicassetUpToDate,
				now:        now,
			},
			want:  replicassetUpToDate,
			want1: 0,
		},
		{
			name: "two RS, update to date, canary not set",
			args: args{
				daemonset:  daemonset,
				upToDateRS: replicassetUpToDate,
				activeRS:   replicassetOld,
				now:        now,
			},
			want:  replicassetUpToDate,
			want1: 0,
		},
		{
			name: "two RS, update to date, canary set not done",
			args: args{
				daemonset:  daemonsetWithCanary,
				upToDateRS: replicassetUpToDate,
				activeRS:   replicassetOld,
				now:        now,
			},

			want:  replicassetOld,
			want1: 5 * time.Minute,
		},
		{
			name: "two RS, update to date, canary set and done",
			args: args{
				daemonset:  daemonsetWithCanary,
				upToDateRS: replicassetUpToDateDone,
				activeRS:   replicassetOld,
				now:        now,
			},
			want:  replicassetUpToDateDone,
			want1: -time.Minute,
		},
		{
			name: "two RS, update to date, canary set, canary duration not done, canary valid",
			args: args{
				daemonset:  daemonsetWithCanaryValid,
				upToDateRS: replicassetUpToDate,
				activeRS:   replicassetOld,
				now:        now,
			},
			want:  replicassetUpToDate,
			want1: 5 * time.Minute,
		},
		{
			name: "two RS, update to date, canary set, canary duration done, canary valid",
			args: args{
				daemonset:  daemonsetWithCanaryValid,
				upToDateRS: replicassetUpToDateDone,
				activeRS:   replicassetOld,
				now:        now,
			},
			want:  replicassetUpToDateDone,
			want1: -time.Minute,
		},
		{
			name: "two RS, update to date, canary set, canary duration not done, canary paused",
			args: args{
				daemonset:  daemonsetWithCanaryPaused,
				upToDateRS: replicassetUpToDate,
				activeRS:   replicassetOld,
				now:        now,
			},
			want:  replicassetOld,
			want1: 5 * time.Minute,
		},
		{
			name: "two RS, update to date, canary set, canary duration done, canary paused",
			args: args{
				daemonset:  daemonsetWithCanaryPaused,
				upToDateRS: replicassetUpToDateDone,
				activeRS:   replicassetOld,
				now:        now,
			},
			want:  replicassetOld,
			want1: -time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("daemonset: %v", tt.args.daemonset)
			got, got1 := selectCurrentReplicaSet(tt.args.daemonset, tt.args.activeRS, tt.args.upToDateRS, tt.args.now)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("selectCurrentReplicaSet() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("selectCurrentReplicaSet() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestReconciler_cleanupReplicaSet(t *testing.T) {
	now := time.Now()
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(t.Logf)

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSet{})

	replicassetUpToDate := test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels: map[string]string{"foo-key": "bar-value"},
	})
	replicassetCurrent := test.NewExtendedDaemonSetReplicaSet("bar", "current", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels: map[string]string{"foo-key": "bar-value"},
	})

	replicassetOld := test.NewExtendedDaemonSetReplicaSet("bar", "old", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels: map[string]string{"foo-key": "bar-value"},
	})

	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}
	type args struct {
		rsList       *datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList
		current      *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		updatetodate *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "nothing to delete",
			fields: fields{
				client: fake.NewClientBuilder().Build(),
				scheme: s,
			},
			args: args{
				rsList: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList{
					Items: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{},
				},
			},
			wantErr: false,
		},
		{
			name: "on RS to delete",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&corev1.Node{}).WithObjects(replicassetOld, replicassetUpToDate, replicassetCurrent).Build(),
				scheme: s,
			},
			args: args{
				rsList: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList{
					Items: []datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{*replicassetOld, *replicassetUpToDate, *replicassetCurrent},
				},
				updatetodate: replicassetUpToDate,
				current:      replicassetCurrent,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqLogger := testLogger.WithValues("test:", tt.name)
			r := &Reconciler{
				client:   tt.fields.client,
				scheme:   tt.fields.scheme,
				log:      testLogger,
				recorder: eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: tt.name}),
			}
			if err := r.cleanupReplicaSet(reqLogger, now, tt.args.rsList, tt.args.current, tt.args.updatetodate); (err != nil) != tt.wantErr {
				t.Errorf("Reconciler.cleanupReplicaSet() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReconciler_createNewReplicaSet(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()

	logf.SetLogger(zap.New())
	log := logf.Log.WithName("TestReconcileExtendedDaemonSet_createNewReplicaSet")

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSet{})

	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}
	type args struct {
		logger    logr.Logger
		daemonset *datadoghqv1alpha1.ExtendedDaemonSet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "create new RS",
			fields: fields{
				client: fake.NewClientBuilder().Build(),
				scheme: s,
			},
			args: args{
				logger:    log,
				daemonset: test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}}),
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:   tt.fields.client,
				scheme:   tt.fields.scheme,
				log:      testLogger,
				recorder: eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconciler_cleanupReplicaSet"}),
			}
			got, err := r.createNewReplicaSet(tt.args.logger, tt.args.daemonset, podsCounterType{
				Current:   3,
				Ready:     2,
				Available: 1,
			})
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconciler.createNewReplicaSet() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reconciler.createNewReplicaSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileExtendedDaemonSet_updateInstanceWithCurrentRS(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	now := time.Now()

	logf.SetLogger(zap.New())
	log := logf.Log.WithName("TestReconcileExtendedDaemonSet_updateStatusWithNewRS")

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSet{})

	daemonset := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}})
	replicassetUpToDate := test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels: map[string]string{"foo-key": "bar-value"},
	})
	replicassetCurrent := test.NewExtendedDaemonSetReplicaSet("bar", "current", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels: map[string]string{"foo-key": "current-value"},
		Status: &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
			// Define different values for all the attributes to avoid passing tests by chance.
			// Constraints: desired >= current >= ready >= available
			Desired:   4,
			Current:   3,
			Ready:     2,
			Available: 1,
		},
	})

	replicassetUpToDateWithPauseCondition := replicassetUpToDate.DeepCopy()
	{
		replicassetUpToDateWithPauseCondition.Status.Conditions = []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
			{
				Type:               datadoghqv1alpha1.ConditionTypeCanaryPaused,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
				Reason:             "CrashLoopBackOff",
			},
		}
	}

	daemonsetWithStatus := daemonset.DeepCopy()
	{
		daemonsetWithStatus.ResourceVersion = "1000"
		daemonsetWithStatus.Status = datadoghqv1alpha1.ExtendedDaemonSetStatus{
			ActiveReplicaSet: "current",
			Desired:          4,
			Current:          3,
			Ready:            2,
			Available:        1,
			UpToDate:         3,
			State:            "Running",
		}
	}

	intString1 := intstr.FromInt(1)
	daemonsetWithCanaryWithStatus := daemonsetWithStatus.DeepCopy()
	{
		// replicassetUpToDate defined above has no replicas so UpToDate should be 0
		daemonsetWithCanaryWithStatus.Status.UpToDate = 0
		daemonsetWithCanaryWithStatus.Status.State = datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanary
		daemonsetWithCanaryWithStatus.Spec.Strategy.Canary = &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
			Replicas: &intString1,
			Duration: &metav1.Duration{Duration: 10 * time.Minute},
		}
		daemonsetWithCanaryWithStatus.Status.Canary = &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
			Nodes:      []string{"node1"},
			ReplicaSet: "foo-1",
		}
		daemonsetWithCanaryWithStatus.ResourceVersion = "1"
	}

	daemonsetWithCanaryPaused := test.NewExtendedDaemonSet(
		"bar",
		"foo",
		&test.NewExtendedDaemonSetOptions{
			Labels: map[string]string{"foo-key": "bar-value"},
			Annotations: map[string]string{
				datadoghqv1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey:       "true",
				datadoghqv1alpha1.ExtendedDaemonSetCanaryPausedReasonAnnotationKey: string(datadoghqv1alpha1.ExtendedDaemonSetStatusReasonCLB),
			},
			Canary: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
				Replicas: &intString1,
				Duration: &metav1.Duration{Duration: 10 * time.Minute},
			},
			Status: &datadoghqv1alpha1.ExtendedDaemonSetStatus{
				ActiveReplicaSet: "current",
				Desired:          4,
				Current:          3,
				Ready:            2,
				Available:        1,
				UpToDate:         0, // replicassetUpToDate defined above has no replicas so UpToDate should be 0
				Canary: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
					Nodes:      []string{"node1"},
					ReplicaSet: "foo-1",
				},
				State:  datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanaryPaused,
				Reason: datadoghqv1alpha1.ExtendedDaemonSetStatusReasonCLB,
			},
		},
	)
	daemonsetWithCanaryPaused.ResourceVersion = "999"
	daemonsetWithCanaryPausedWanted := daemonsetWithCanaryPaused.DeepCopy()
	{
		daemonsetWithCanaryPausedWanted.ResourceVersion = "1000"
		daemonsetWithCanaryPausedWanted.Status.Conditions = []datadoghqv1alpha1.ExtendedDaemonSetCondition{
			{
				Type:               datadoghqv1alpha1.ConditionTypeEDSCanaryPaused,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
				Reason:             "CrashLoopBackOff",
				Message:            "canary paused with ers: foo-1",
			},
		}
	}

	daemonsetWithCanaryPausedWithoutAnnotations := daemonsetWithCanaryPaused.DeepCopy()
	{
		daemonsetWithCanaryPausedWithoutAnnotations.Annotations = make(map[string]string)
	}
	daemonsetWithCanaryPausedWithoutAnnotationsWanted := daemonsetWithCanaryPausedWanted.DeepCopy()
	{
		daemonsetWithCanaryPausedWithoutAnnotationsWanted.Annotations = make(map[string]string)
	}

	daemonsetWithCanaryFailedOldWithoutAnnotationsStatus := daemonsetWithCanaryWithStatus.DeepCopy()
	{
		daemonsetWithCanaryFailedOldWithoutAnnotationsStatus.Status.Canary = &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
			Nodes:      []string{"node1"},
			ReplicaSet: "foo-1",
		}
		daemonsetWithCanaryFailedOldWithoutAnnotationsStatus.Status.Conditions = []datadoghqv1alpha1.ExtendedDaemonSetCondition{
			{
				Type:               datadoghqv1alpha1.ConditionTypeEDSCanaryPaused,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
				Reason:             "CrashLoopBackOff",
				Message:            "canary paused with ers: foo-1",
			},
		}
	}
	daemonsetWithCanaryFailedOldStatus := daemonsetWithCanaryFailedOldWithoutAnnotationsStatus.DeepCopy()
	daemonsetWithCanaryFailedWithoutAnnotationsWanted := daemonsetWithCanaryFailedOldStatus.DeepCopy()
	{
		// When the canary fails, the number of "Updated" replicas should equal
		// the number of current ones.
		daemonsetWithCanaryFailedWithoutAnnotationsWanted.Status.UpToDate = replicassetCurrent.Status.Current
		daemonsetWithCanaryFailedWithoutAnnotationsWanted.ResourceVersion = "3"
		daemonsetWithCanaryFailedWithoutAnnotationsWanted.Status.Canary = nil
		daemonsetWithCanaryFailedWithoutAnnotationsWanted.Status.State = datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanaryFailed
		daemonsetWithCanaryFailedWithoutAnnotationsWanted.Status.Conditions = []datadoghqv1alpha1.ExtendedDaemonSetCondition{
			{
				Type:               datadoghqv1alpha1.ConditionTypeEDSCanaryPaused,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
				Reason:             "CrashLoopBackOff",
				Message:            "canary paused with ers: foo-1",
			},
			{
				Type:               datadoghqv1alpha1.ConditionTypeEDSCanaryFailed,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
				Reason:             "CanaryFailed",
				Message:            "canary failed with ers: foo-1",
			},
		}
	}

	daemonsetWithStatusAndAvailable := daemonsetWithStatus.DeepCopy()
	daemonsetWithStatusAndAvailable.Status.Available = 5

	replicassetUpToDateWithFailedCondition := replicassetUpToDate.DeepCopy()
	{
		replicassetUpToDateWithFailedCondition.Status.Conditions = []datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition{
			{
				Type:               datadoghqv1alpha1.ConditionTypeCanaryFailed,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(now),
				LastUpdateTime:     metav1.NewTime(now),
				Reason:             "CrashLoopBackOff",
			},
		}
	}

	type fields struct {
		client client.Client
		scheme *runtime.Scheme
	}
	type args struct {
		logger      logr.Logger
		daemonset   *datadoghqv1alpha1.ExtendedDaemonSet
		current     *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		upToDate    *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		podsCounter podsCounterType
	}
	tests := []struct {
		now        time.Time
		name       string
		fields     fields
		args       args
		want       *datadoghqv1alpha1.ExtendedDaemonSet
		wantResult reconcile.Result
		wantErr    bool
	}{
		{
			now:  now,
			name: "no replicaset == no update",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}).WithObjects(daemonset).Build(),
				scheme: s,
			},
			args: args{
				logger:    testLogger,
				daemonset: daemonset,
				current:   nil,
				upToDate:  nil,
			},
			want:       daemonset,
			wantResult: reconcile.Result{Requeue: false},
			wantErr:    false,
		},
		{
			now:  now,
			name: "current == upToDate; status empty => update",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).
					WithObjects(daemonset, replicassetCurrent, replicassetUpToDate).Build(),
				scheme: s,
			},
			args: args{
				logger:    testLogger,
				daemonset: daemonset,
				current:   replicassetCurrent,
				upToDate:  replicassetCurrent,
				podsCounter: podsCounterType{
					Current:   3,
					Ready:     2,
					Available: 1,
				},
			},
			want:       daemonsetWithStatus,
			wantResult: reconcile.Result{Requeue: false},
			wantErr:    false,
		},
		{
			now:  now,
			name: "current != upToDate; canary active => update",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).
					WithObjects(daemonset, replicassetCurrent, replicassetUpToDate).Build(),
				scheme: s,
			},
			args: args{
				logger:    testLogger,
				daemonset: daemonsetWithCanaryWithStatus,
				current:   replicassetCurrent,
				upToDate:  replicassetUpToDate,
				podsCounter: podsCounterType{
					Current:   3,
					Ready:     2,
					Available: 1,
				},
			},
			want:       daemonsetWithCanaryWithStatus,
			wantResult: reconcile.Result{Requeue: false},
			wantErr:    false,
		},
		{
			now:  now,
			name: "current != upToDate; canary paused => update",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).
					WithObjects(daemonset, replicassetCurrent, replicassetUpToDate).Build(),
				scheme: s,
			},
			args: args{
				logger:    log,
				daemonset: daemonsetWithCanaryPaused,
				current:   replicassetCurrent,
				upToDate:  replicassetUpToDate,
				podsCounter: podsCounterType{
					Current:   3,
					Ready:     2,
					Available: 1,
				},
			},
			want:       daemonsetWithCanaryPausedWanted,
			wantResult: reconcile.Result{Requeue: false},
			wantErr:    false,
		},
		{
			now:  now,
			name: "current != upToDate; ers-condition-pause, canary paused => update",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).
					WithObjects(daemonset, replicassetCurrent, replicassetUpToDate).Build(),
				scheme: s,
			},
			args: args{
				logger:    log,
				daemonset: daemonsetWithCanaryPausedWithoutAnnotations,
				current:   replicassetCurrent,
				upToDate:  replicassetUpToDateWithPauseCondition,
				podsCounter: podsCounterType{
					Current:   3,
					Ready:     2,
					Available: 1,
				},
			},
			want:       daemonsetWithCanaryPausedWithoutAnnotationsWanted,
			wantResult: reconcile.Result{Requeue: false},
			wantErr:    false,
		},
		{
			now:  now,
			name: "canary failed, ers-condition-failed => update",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).
					WithObjects(daemonsetWithCanaryFailedOldStatus, replicassetCurrent, replicassetUpToDate).Build(),
				scheme: s,
			},
			args: args{
				logger:    log,
				daemonset: daemonsetWithCanaryFailedOldWithoutAnnotationsStatus,
				current:   replicassetCurrent,
				upToDate:  replicassetUpToDateWithFailedCondition,
				podsCounter: podsCounterType{
					Current:   3,
					Ready:     2,
					Available: 1,
				},
			},
			want:       daemonsetWithCanaryFailedWithoutAnnotationsWanted,
			wantResult: reconcile.Result{Requeue: false},
			wantErr:    false,
		},
		{
			now:  now,
			name: "\"available\" correct when current == upToDate",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).WithObjects(daemonset, replicassetCurrent, replicassetUpToDate).Build(),
				scheme: s,
			},
			args: args{
				logger:    testLogger,
				daemonset: daemonset,
				current:   replicassetCurrent,
				upToDate:  replicassetCurrent,
				podsCounter: podsCounterType{
					Current: 3,
					Ready:   2,
					// The purpose of this test is to check that the number of
					// available replicas in the result is taken from
					// podsCounter.Available (not from
					// daemonset.Status.Available or other field)
					Available: 5,
				},
			},
			want:       daemonsetWithStatusAndAvailable, // Its "Available" field equals podsCounter.Available
			wantResult: reconcile.Result{Requeue: false},
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:   tt.fields.client,
				scheme:   tt.fields.scheme,
				recorder: eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconciler_cleanupReplicaSet"}),
			}
			got, got1, err := r.updateInstanceWithCurrentRS(tt.args.logger, tt.now, tt.args.daemonset, tt.args.current, tt.args.upToDate, tt.args.podsCounter)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileExtendedDaemonSet.updateInstanceWithCurrentRS() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if len(tt.want.Status.Conditions) > 0 {
				// https://github.com/kubernetes-sigs/controller-runtime/blob/735b6073bb253c0449bfcf6641855dcf2118bb15/pkg/client/fake/client.go#L1037-L1053
				// Some of time.Time info is lost here due to marshaling to json and unmarshaling.
				for i := range tt.want.Status.Conditions {
					assert.Equal(t, tt.want.Status.Conditions[i].LastTransitionTime.Truncate(time.Second), got.Status.Conditions[i].LastTransitionTime.Truncate(time.Second))
					assert.Equal(t, tt.want.Status.Conditions[i].LastUpdateTime.Truncate(time.Second), got.Status.Conditions[i].LastUpdateTime.Truncate(time.Second))

					got.Status.Conditions[i].LastTransitionTime = tt.want.Status.Conditions[i].LastTransitionTime
					got.Status.Conditions[i].LastUpdateTime = tt.want.Status.Conditions[i].LastUpdateTime
				}
			}
			assert.Equal(t, tt.want, got, "ReconcileExtendedDaemonSet.updateInstanceWithCurrentRS()")
			assert.Equal(t, tt.wantResult, got1, "ReconcileExtendedDaemonSet.updateInstanceWithCurrentRS().result")
		})
	}
}

func TestReconciler_Reconcile(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconciler_Reconcile"})

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSet{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetList{})

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		request  reconcile.Request
		loadFunc func(c client.Client)
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		want     reconcile.Result
		wantErr  bool
		wantFunc func(c client.Client) error
	}{
		{
			name: "ExtendedDaemonset not found",
			fields: fields{
				client:   fake.NewClientBuilder().WithObjects().Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("but", "faa"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "ExtendedDaemonset found, but not defaulted",
			fields: fields{
				client:   fake.NewClientBuilder().WithObjects().Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					_ = c.Create(context.TODO(), test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}}))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
		},
		{
			name: "ExtendedDaemonset found and defaulted => create the replicaset",
			fields: fields{
				client:   fake.NewClientBuilder().WithObjects().Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dd := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}})
					dd = datadoghqv1alpha1.DefaultExtendedDaemonSet(dd, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)
					_ = c.Create(context.TODO(), dd)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				replicasetList := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList{}
				listOptions := []client.ListOption{
					client.InNamespace("bar"),
				}
				if err := c.List(context.TODO(), replicasetList, listOptions...); err != nil {
					return err
				}
				if len(replicasetList.Items) != 1 {
					return fmt.Errorf("len(replicasetList.Items) is not equal to 1")
				}
				if replicasetList.Items[0].GenerateName != "foo-" {
					return fmt.Errorf("replicasetList.Items[0] bad generated name, should be: 'foo-', current: %s", replicasetList.Items[0].GenerateName)
				}

				return nil
			},
		},
		{
			name: "ExtendedDaemonset found and defaulted, replicaset already exist",
			fields: fields{
				// https://github.com/kubernetes-sigs/controller-runtime/issues/2362#issuecomment-1837270195
				client:   fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dd := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}})
					dd = datadoghqv1alpha1.DefaultExtendedDaemonSet(dd, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)

					hash, _ := comparison.GenerateMD5PodTemplateSpec(&dd.Spec.Template)
					rsOptions := &test.NewExtendedDaemonSetReplicaSetOptions{
						GenerateName: "foo-",
						Labels:       map[string]string{"foo-key": "bar-value", datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: "foo"},
						Annotations:  map[string]string{string(datadoghqv1alpha1.MD5ExtendedDaemonSetAnnotationKey): hash},
					}
					rs := test.NewExtendedDaemonSetReplicaSet("bar", "", rsOptions)

					_ = c.Create(context.TODO(), dd)
					_ = c.Create(context.TODO(), rs)
				},
			},
			want:    reconcile.Result{Requeue: false},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				replicasetList := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList{}
				listOptions := []client.ListOption{
					client.InNamespace("bar"),
				}
				if err := c.List(context.TODO(), replicasetList, listOptions...); err != nil {
					return err
				}
				if len(replicasetList.Items) != 1 {
					return fmt.Errorf("len(replicasetList.Items) is not equal to 1")
				}
				if replicasetList.Items[0].GenerateName != "foo-" {
					return fmt.Errorf("replicasetList.Items[0] bad generated name, should be: 'foo-', current: %s", replicasetList.Items[0].GenerateName)
				}

				return nil
			},
		},
		{
			name: "ExtendedDaemonset found and defaulted, replicaset already but not uptodate",
			fields: fields{
				client:   fake.NewClientBuilder().Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dd := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}})
					dd = datadoghqv1alpha1.DefaultExtendedDaemonSet(dd, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)

					rsOptions := &test.NewExtendedDaemonSetReplicaSetOptions{
						GenerateName: "foo-",
						Labels:       map[string]string{"foo-key": "old-value"},
						Annotations:  map[string]string{string(datadoghqv1alpha1.MD5ExtendedDaemonSetAnnotationKey): "oldhash"},
					}
					rs := test.NewExtendedDaemonSetReplicaSet("bar", "foo-old", rsOptions)

					_ = c.Create(context.TODO(), dd)
					_ = c.Create(context.TODO(), rs)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				replicasetList := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList{}
				listOptions := []client.ListOption{
					client.InNamespace("bar"),
				}
				if err := c.List(context.TODO(), replicasetList, listOptions...); err != nil {
					return err
				}
				if len(replicasetList.Items) != 2 {
					return fmt.Errorf("len(replicasetList.Items) is not equal to 1")
				}

				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:   tt.fields.client,
				scheme:   tt.fields.scheme,
				recorder: tt.fields.recorder,
				log:      testLogger,
			}
			if tt.args.loadFunc != nil {
				tt.args.loadFunc(r.client)
			}
			got, err := r.Reconcile(context.TODO(), tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reconciler.Reconcile() = %v, want %v", got, tt.want)
			}
			if tt.wantFunc != nil {
				if err := tt.wantFunc(r.client); err != nil {
					t.Errorf("Reconciler.Reconcile() wantFunc validation error: %v", err)
				}
			}
		})
	}
}

func newRequest(ns, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ns,
			Name:      name,
		},
	}
}

func Test_getAntiAffinityKeysValue(t *testing.T) {
	node := commontest.NewNode("node", &commontest.NewNodeOptions{
		Labels: map[string]string{
			"app":     "foo",
			"service": "bar",
			"unused":  "baz",
		},
	})

	tests := []struct {
		name          string
		node          corev1.Node
		daemonsetSpec datadoghqv1alpha1.ExtendedDaemonSetSpec
		want          string
	}{
		{
			name: "basic",
			node: *node,
			daemonsetSpec: datadoghqv1alpha1.ExtendedDaemonSetSpec{
				Strategy: datadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
					Canary: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
						NodeAntiAffinityKeys: []string{
							"app",
							"missing",
							"service",
						},
					},
				},
			},
			want: "foo$$bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAntiAffinityKeysValue(&tt.node, &tt.daemonsetSpec)
			if got != tt.want {
				t.Errorf("getAntiAffinityKeysValue(%#v, %#v) = %s, want %s", tt.node, tt.daemonsetSpec, got, tt.want)
			}
		})
	}
}

func Test_isCanaryActive(t *testing.T) {
	type args struct {
		daemonset       *datadoghqv1alpha1.ExtendedDaemonSet
		activeERSName   string
		upToDateERSName string
		isCanaryFailed  bool
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "canary spec not set",
			args: args{
				daemonset: test.NewExtendedDaemonSet("ns-foo", "foo", &test.NewExtendedDaemonSetOptions{Canary: nil}),
			},
			want: false,
		},
		{
			name: "CanarySpec Enabled, 2 ers, canary not failed",
			args: args{
				daemonset:       test.NewExtendedDaemonSet("ns-foo", "foo", &test.NewExtendedDaemonSetOptions{Canary: datadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(&datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{}, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)}),
				activeERSName:   "foo-old",
				upToDateERSName: "foo-new",
				isCanaryFailed:  false,
			},
			want: true,
		},
		{
			name: "CanarySpec Enabled, But canary failed",
			args: args{
				daemonset:       test.NewExtendedDaemonSet("ns-foo", "foo", &test.NewExtendedDaemonSetOptions{Canary: datadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(&datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{}, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)}),
				activeERSName:   "foo-old",
				upToDateERSName: "foo-new",
				isCanaryFailed:  true,
			},
			want: false,
		},
		{
			name: "CanarySpec Enabled, but ERS active == ERS up-to-date",
			args: args{
				daemonset:       test.NewExtendedDaemonSet("ns-foo", "foo", &test.NewExtendedDaemonSetOptions{Canary: datadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(&datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{}, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)}),
				activeERSName:   "foo-new",
				upToDateERSName: "foo-new",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCanaryActive(tt.args.daemonset, tt.args.activeERSName, tt.args.upToDateERSName, tt.args.isCanaryFailed); got != tt.want {
				t.Errorf("isCanaryActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_manageStatus(t *testing.T) {
	ns := "bar"
	edsName := "foo"
	ersName := fmt.Sprintf("%s-dsdvdv", edsName)

	blankStatus := datadoghqv1alpha1.ExtendedDaemonSetStatus{}

	statusCanaryFailed := datadoghqv1alpha1.ExtendedDaemonSetStatus{
		State: datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanaryFailed,
	}

	statusCanaryActive := datadoghqv1alpha1.ExtendedDaemonSetStatus{
		State: datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanary,
		Canary: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
			ReplicaSet: ersName,
		},
	}

	statusCanaryPaused := datadoghqv1alpha1.ExtendedDaemonSetStatus{
		State:  datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanaryPaused,
		Reason: datadoghqv1alpha1.ExtendedDaemonSetStatusReasonOOM,
		Canary: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
			ReplicaSet: ersName,
		},
	}

	statusEDSRunning := datadoghqv1alpha1.ExtendedDaemonSetStatus{
		State: datadoghqv1alpha1.ExtendedDaemonSetStatusStateRunning,
	}

	statusEDSPaused := datadoghqv1alpha1.ExtendedDaemonSetStatus{
		State: datadoghqv1alpha1.ExtendedDaemonSetStatusStateRollingUpdatePaused,
	}

	statusEDSFrozen := datadoghqv1alpha1.ExtendedDaemonSetStatus{
		State: datadoghqv1alpha1.ExtendedDaemonSetStatusStateRolloutFrozen,
	}

	type args struct {
		status         *datadoghqv1alpha1.ExtendedDaemonSetStatus
		upToDate       *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		isCanaryActive bool
		isCanaryFailed bool
		isCanaryPaused bool
		pausedReason   datadoghqv1alpha1.ExtendedDaemonSetStatusReason
		daemonset      *datadoghqv1alpha1.ExtendedDaemonSet
	}
	tests := []struct {
		name string
		args args
		want *datadoghqv1alpha1.ExtendedDaemonSetStatus
	}{
		{
			name: "CanaryFailed",
			args: args{
				status:         blankStatus.DeepCopy(),
				upToDate:       test.NewExtendedDaemonSetReplicaSet(ns, ersName, nil),
				isCanaryFailed: true,
			},
			want: &statusCanaryFailed,
		},
		{
			name: "CanaryActive",
			args: args{
				status:         blankStatus.DeepCopy(),
				upToDate:       test.NewExtendedDaemonSetReplicaSet(ns, ersName, nil),
				isCanaryActive: true,
			},
			want: &statusCanaryActive,
		},
		{
			name: "CanaryPause",
			args: args{
				status:         blankStatus.DeepCopy(),
				upToDate:       test.NewExtendedDaemonSetReplicaSet(ns, ersName, nil),
				isCanaryPaused: true,
				isCanaryActive: true,
				pausedReason:   datadoghqv1alpha1.ExtendedDaemonSetStatusReasonOOM,
			},
			want: &statusCanaryPaused,
		},
		{
			name: "No canary, EDS running",
			args: args{
				status:         blankStatus.DeepCopy(),
				upToDate:       test.NewExtendedDaemonSetReplicaSet(ns, ersName, nil),
				isCanaryPaused: false,
				isCanaryActive: false,
				daemonset:      test.NewExtendedDaemonSet("bar", "foo", nil),
			},
			want: &statusEDSRunning,
		},
		{
			name: "No canary, EDS paused",
			args: args{
				status:         blankStatus.DeepCopy(),
				upToDate:       test.NewExtendedDaemonSetReplicaSet(ns, ersName, nil),
				isCanaryPaused: false,
				isCanaryActive: false,
				daemonset:      test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Annotations: map[string]string{datadoghqv1alpha1.ExtendedDaemonSetRollingUpdatePausedAnnotationKey: "true"}}),
			},
			want: &statusEDSPaused,
		},
		{
			name: "No canary, EDS frozen",
			args: args{
				status:         blankStatus.DeepCopy(),
				upToDate:       test.NewExtendedDaemonSetReplicaSet(ns, ersName, nil),
				isCanaryPaused: false,
				isCanaryActive: false,
				daemonset:      test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Annotations: map[string]string{datadoghqv1alpha1.ExtendedDaemonSetRolloutFrozenAnnotationKey: "true"}}),
			},
			want: &statusEDSFrozen,
		},
		{
			// A frozen state includes the paused state
			// The EDS must be considered frozen in this case
			name: "No canary, EDS frozen and paused at the same time",
			args: args{
				status:         blankStatus.DeepCopy(),
				upToDate:       test.NewExtendedDaemonSetReplicaSet(ns, ersName, nil),
				isCanaryPaused: false,
				isCanaryActive: false,
				daemonset: test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Annotations: map[string]string{
					datadoghqv1alpha1.ExtendedDaemonSetRolloutFrozenAnnotationKey:       "true",
					datadoghqv1alpha1.ExtendedDaemonSetRollingUpdatePausedAnnotationKey: "true",
				}}),
			},
			want: &statusEDSFrozen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manageStatus(tt.args.status, tt.args.upToDate, tt.args.isCanaryActive, tt.args.isCanaryFailed, tt.args.isCanaryPaused, tt.args.pausedReason, tt.args.daemonset)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("manageCanaryStatus() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
