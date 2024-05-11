// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonsetreplicaset

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	clock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/api/v1alpha1/test"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/strategy"
	ctrltest "github.com/DataDog/extendeddaemonset/pkg/controller/test"
)

var testLogger logr.Logger = logf.Log.WithName("controller-test")

func TestReconcileExtendedDaemonSetReplicaSet_Reconcile(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileExtendedDaemonSet_Reconcile"})

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetList{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSet{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonsetSettingList{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonsetSetting{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.PodList{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Pod{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.NodeList{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Node{})

	maxUnavailable := intstr.FromInt(1)
	maxPodSchedulerFailure := intstr.FromInt(0)
	slowStartAdditiveIncrease := intstr.FromInt(1)
	slowStartIntervalDuration := metav1.Duration{Duration: time.Minute}
	rollingUpdate := &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
		MaxUnavailable:            &maxUnavailable,
		MaxPodSchedulerFailure:    &maxPodSchedulerFailure,
		SlowStartAdditiveIncrease: &slowStartAdditiveIncrease,
		SlowStartIntervalDuration: &slowStartIntervalDuration,
		MaxParallelPodCreation:    datadoghqv1alpha1.NewInt32(1),
	}
	options := &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}, RollingUpdate: rollingUpdate}
	daemonset := test.NewExtendedDaemonSet("but", "foo", options)

	status := &datadoghqv1alpha1.ExtendedDaemonSetStatus{
		ActiveReplicaSet: "foo-1",
		Canary: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
			ReplicaSet: "foo-2",
		},
	}
	daemonsetWithStatus := test.NewExtendedDaemonSet("but", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}, RollingUpdate: rollingUpdate, Status: status})
	replicaset := test.NewExtendedDaemonSetReplicaSet("but", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels:       map[string]string{"foo-key": "bar-value"},
		OwnerRefName: "foo",
	})

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		request reconcile.Request
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "ReplicaSet does not exist in client",
			fields: fields{
				client:   fake.NewClientBuilder().Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo-bar"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "ReplicaSet exist but not Daemonset, it should trigger an error",
			fields: fields{
				client:   fake.NewClientBuilder().WithObjects(replicaset).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("but", "foo-1"),
			},
			want:    reconcile.Result{},
			wantErr: true,
		},
		{
			name: "ReplicaSet, Daemonset exists but not defaulted => should requeue in 1sec",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).WithObjects(daemonset, replicaset).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("but", "foo-1"),
			},
			want:    reconcile.Result{RequeueAfter: time.Second},
			wantErr: false,
		},
		{
			name: "ReplicaSet, Daemonset exists, defaulted but without a status => should requeue in 1sec",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).
					WithObjects(datadoghqv1alpha1.DefaultExtendedDaemonSet(daemonset, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto), replicaset).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("but", "foo-1"),
			},
			want:    reconcile.Result{RequeueAfter: time.Second},
			wantErr: false,
		},
		{
			name: "ReplicaSet, Daemonset exists, defaulted and with a status",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).
					WithObjects(datadoghqv1alpha1.DefaultExtendedDaemonSet(daemonsetWithStatus, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto), replicaset).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("but", "foo-1"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:            tt.fields.client,
				scheme:            tt.fields.scheme,
				recorder:          tt.fields.recorder,
				failedPodsBackOff: flowcontrol.NewFakeBackOff(30*time.Second, 15*time.Minute, clock.NewFakeClock(time.Now())),
				log:               testLogger,
			}
			got, err := r.Reconcile(context.TODO(), tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileExtendedDaemonSetReplicaSet.Reconcile() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileExtendedDaemonSetReplicaSet.Reconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_retrieveReplicaSetStatus(t *testing.T) {
	status := &datadoghqv1alpha1.ExtendedDaemonSetStatus{
		ActiveReplicaSet: "rs-active",
		Canary: &datadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
			ReplicaSet: "rs-canary",
		},
	}
	daemonset := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}, Status: status})

	type args struct {
		daemonset       *datadoghqv1alpha1.ExtendedDaemonSet
		replicassetName string
	}
	tests := []struct {
		name string
		args args
		want strategy.ReplicaSetStatus
	}{
		{
			name: "status unknown",
			args: args{
				daemonset:       daemonset,
				replicassetName: "rs-unknown",
			},
			want: strategy.ReplicaSetStatusUnknown,
		},
		{
			name: "status unknown",
			args: args{
				daemonset:       daemonset,
				replicassetName: "rs-active",
			},
			want: strategy.ReplicaSetStatusActive,
		},
		{
			name: "status unknown",
			args: args{
				daemonset:       daemonset,
				replicassetName: "rs-canary",
			},
			want: strategy.ReplicaSetStatusCanary,
		},
		{
			name: "activeRS not set => unknown",
			args: args{
				daemonset:       test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}}),
				replicassetName: "rs-active",
			},
			want: strategy.ReplicaSetStatusUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retrieveReplicaSetStatus(tt.args.daemonset, tt.args.replicassetName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("retrieveReplicaSetStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_retrieveOwnerReference(t *testing.T) {
	type args struct {
		obj *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "ownerref not found",
			args: args{
				obj: test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
					Labels: map[string]string{"foo-key": "bar-value"},
				}),
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "ownerref found",
			args: args{
				obj: test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
					Labels:       map[string]string{"foo-key": "bar-value"},
					OwnerRefName: "foo",
				}),
			},
			want:    "foo",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := retrieveOwnerReference(tt.args.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("retrieveOwnerReference() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if got != tt.want {
				t.Errorf("retrieveOwnerReference() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileExtendedDaemonSetReplicaSet_getPodList(t *testing.T) {
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Pod{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.PodList{})

	ns := "bar"
	daemonset := test.NewExtendedDaemonSet(ns, "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}})
	podOptions := &ctrltest.NewPodOptions{
		Labels: map[string]string{
			datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: "foo",
		},
	}
	pod1 := ctrltest.NewPod(ns, "foo-pod1", ns, podOptions)
	pod2 := ctrltest.NewPod(ns, "foo-pod2", ns, podOptions)

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		ds *datadoghqv1alpha1.ExtendedDaemonSet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *corev1.PodList
		wantErr bool
	}{
		{
			name: "no pods",
			fields: fields{
				client: fake.NewClientBuilder().Build(),
				scheme: s,
			},
			args: args{
				ds: daemonset,
			},
			want: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PodList",
					APIVersion: "v1",
				},
			},
			wantErr: false,
		},
		{
			name: "two pods",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&corev1.Pod{}).WithObjects(pod1, pod2).Build(),
				scheme: s,
			},
			args: args{
				ds: daemonset,
			},
			want: &corev1.PodList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PodList",
					APIVersion: "v1",
				},
				Items: []corev1.Pod{*pod1, *pod2},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:            tt.fields.client,
				scheme:            tt.fields.scheme,
				recorder:          tt.fields.recorder,
				failedPodsBackOff: flowcontrol.NewFakeBackOff(30*time.Second, 15*time.Minute, clock.NewFakeClock(time.Now())),
			}
			got, err := r.getPodList(tt.args.ds)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileExtendedDaemonSetReplicaSet.getPodList() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !apiequality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileExtendedDaemonSetReplicaSet.getPodList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileExtendedDaemonSetReplicaSet_getNodeList(t *testing.T) {
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Node{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.NodeList{})
	eds := test.NewExtendedDaemonSet("bar", "foo", nil)
	replicasset := test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels: map[string]string{"foo-key": "bar-value"},
	})

	nodeOptions := &ctrltest.NewNodeOptions{
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	node1 := ctrltest.NewNode("node1", nodeOptions)
	node2 := ctrltest.NewNode("node2", nodeOptions)

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		eds        *datadoghqv1alpha1.ExtendedDaemonSet
		replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *strategy.NodeList
		wantErr bool
	}{
		{
			name: "no nodes",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&corev1.Node{}).WithObjects(node1, node2).Build(),
				scheme: s,
			},
			args: args{
				eds:        eds,
				replicaset: replicasset,
			},
			want: &strategy.NodeList{
				Items: []*strategy.NodeItem{
					{Node: node1},
					{Node: node2},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:            tt.fields.client,
				scheme:            tt.fields.scheme,
				recorder:          tt.fields.recorder,
				failedPodsBackOff: flowcontrol.NewFakeBackOff(30*time.Second, 15*time.Minute, clock.NewFakeClock(time.Now())),
				log:               testLogger,
			}
			got, err := r.getNodeList(eds, tt.args.replicaset)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileExtendedDaemonSetReplicaSet.getNodeList() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !apiequality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileExtendedDaemonSetReplicaSet.getNodeList() = %#v \nwant %#v", got, tt.want)
			}
		})
	}
}

func TestReconcileExtendedDaemonSetReplicaSet_getDaemonsetOwner(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSet{})

	replicasset := test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels: map[string]string{"foo-key": "bar-value"},
	})
	replicassetWithOwner := test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels:       map[string]string{"foo-key": "bar-value"},
		OwnerRefName: "foo",
	},
	)
	daemonset := test.NewExtendedDaemonSet("bar", "foo", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"foo-key": "bar-value"}})

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *datadoghqv1alpha1.ExtendedDaemonSet
		wantErr bool
	}{
		{
			name: "owner not define, return errror",
			fields: fields{
				client: fake.NewClientBuilder().Build(),
				scheme: s,
			},
			args: args{
				replicaset: replicasset,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "with owner define, but not exist, return errror",
			fields: fields{
				client: fake.NewClientBuilder().Build(),
				scheme: s,
			},
			args: args{
				replicaset: replicassetWithOwner,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "with owner define, but not exist, return errror",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSet{}).WithObjects(daemonset).Build(),
				scheme: s,
			},
			args: args{
				replicaset: replicassetWithOwner,
			},
			want:    daemonset,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:            tt.fields.client,
				scheme:            tt.fields.scheme,
				recorder:          tt.fields.recorder,
				failedPodsBackOff: flowcontrol.NewFakeBackOff(30*time.Second, 15*time.Minute, clock.NewFakeClock(time.Now())),
				log:               testLogger,
			}
			got, err := r.getDaemonsetOwner(tt.args.replicaset)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileExtendedDaemonSetReplicaSet.getDaemonsetOwner() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !apiequality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileExtendedDaemonSetReplicaSet.getDaemonsetOwner() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileExtendedDaemonSetReplicaSet_updateReplicaSet(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{})

	replicasset := test.NewExtendedDaemonSetReplicaSet("bar", "foo-1", &test.NewExtendedDaemonSetReplicaSetOptions{
		Labels:       map[string]string{"foo-key": "bar-value"},
		OwnerRefName: "foo",
	},
	)

	newStatus := replicasset.Status.DeepCopy()
	newStatus.Desired = 3

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet
		newStatus  *datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "error: replicaset doesn't exist",
			fields: fields{
				client: fake.NewClientBuilder().Build(),
				scheme: s,
			},
			args: args{
				replicaset: replicasset,
				newStatus:  newStatus,
			},
			wantErr: true,
		},
		{
			name: "new status, update should work",
			fields: fields{
				client: fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}).WithObjects(replicasset).Build(),
				scheme: s,
			},
			args: args{
				replicaset: replicasset,
				newStatus:  newStatus,
			},
			wantErr: false,
		},
		{
			name: "same status, we should not update the replicaset",
			fields: fields{
				client: fake.NewClientBuilder().Build(),
				scheme: s,
			},
			args: args{
				replicaset: replicasset,
				newStatus:  replicasset.Status.DeepCopy(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:            tt.fields.client,
				scheme:            tt.fields.scheme,
				recorder:          tt.fields.recorder,
				failedPodsBackOff: flowcontrol.NewFakeBackOff(30*time.Second, 15*time.Minute, clock.NewFakeClock(time.Now())),
				log:               testLogger,
			}
			if err := r.updateReplicaSet(tt.args.replicaset, tt.args.newStatus); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileExtendedDaemonSetReplicaSet.updateReplicaSet() error = %v, wantErr %v", err, tt.wantErr)
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
