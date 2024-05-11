// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonsetsetting

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/api/v1alpha1/test"
	commontest "github.com/DataDog/extendeddaemonset/pkg/controller/test"
)

var testLogger logr.Logger = logf.Log.WithName("test")

func newRequest(ns, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ns,
			Name:      name,
		},
	}
}

func TestReconcile(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcile"})

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonsetSetting{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonsetSettingList{})

	now := time.Now()
	commonLabels := map[string]string{
		"test": "bigmemory",
	}

	// Define EDSS
	edsOptions1 := &test.NewExtendedDaemonsetSettingOptions{
		CreationTime: now,
		Selector:     commonLabels,
	}
	edsNode1 := test.NewExtendedDaemonsetSetting("foo", "bar", "app", edsOptions1)
	edsOptions2 := &test.NewExtendedDaemonsetSettingOptions{
		CreationTime: now.Add(time.Minute),
		Selector:     commonLabels,
	}
	edsNode2 := test.NewExtendedDaemonsetSetting("foo", "bar2", "app", edsOptions2)
	edsNode3 := edsNode1.DeepCopy()
	edsNode3.Spec.Reference = nil
	edsNode4 := edsNode1.DeepCopy()
	edsNode4.Spec.Reference.Name = ""

	// Define nodes
	nodeOptions := &commontest.NewNodeOptions{
		Labels: commonLabels,
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	node1 := commontest.NewNode("node1", nodeOptions)

	tests := []struct {
		name          string
		client        client.Client
		want          reconcile.Result
		wantStatusErr bool
	}{
		{
			name:          "No object, empty result",
			client:        fake.NewClientBuilder().WithObjects().Build(),
			want:          reconcile.Result{},
			wantStatusErr: false,
		},
		{
			name:          "Found object, empty result",
			client:        fake.NewClientBuilder().WithStatusSubresource(edsNode1).WithObjects(edsNode1).Build(),
			want:          reconcile.Result{},
			wantStatusErr: false,
		},
		{
			name:          "With a node",
			client:        fake.NewClientBuilder().WithStatusSubresource(node1, edsNode1).WithObjects(node1, edsNode1).Build(),
			want:          reconcile.Result{},
			wantStatusErr: false,
		},
		{
			name:          "Nil reference",
			client:        fake.NewClientBuilder().WithStatusSubresource(node1, edsNode3).WithObjects(node1, edsNode3).Build(),
			want:          reconcile.Result{},
			wantStatusErr: true,
		},
		{
			name:          "Empty reference name",
			client:        fake.NewClientBuilder().WithStatusSubresource(node1, edsNode4).WithObjects(node1, edsNode4).Build(),
			want:          reconcile.Result{},
			wantStatusErr: true,
		},
		{
			name:          "Conflict",
			client:        fake.NewClientBuilder().WithStatusSubresource(node1, edsNode1, edsNode2).WithObjects(node1, edsNode1, edsNode2).Build(),
			want:          reconcile.Result{},
			wantStatusErr: true,
		},
	}
	request := newRequest("foo", "bar")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := NewReconciler(ReconcilerOptions{}, tt.client, s, testLogger, recorder)

			got, _ := r.Reconcile(context.TODO(), request)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reconcile() = %v, but want %v", got, tt.want)
			}

			instance := &datadoghqv1alpha1.ExtendedDaemonsetSetting{}
			_ = r.client.Get(context.TODO(), request.NamespacedName, instance)

			if tt.wantStatusErr && instance.Status.Error == "" {
				t.Errorf("Reconcile err is nil, but want an error")
			}
			if !tt.wantStatusErr && instance.Status.Error != "" {
				t.Errorf("Reconcile err is %v, but want nil", instance.Status.Error)
			}
		})
	}
}

func Test_searchPossibleConflict(t *testing.T) {
	now := time.Now()
	commonLabels := map[string]string{
		"test": "bigmemory",
	}
	edsOptions1 := &test.NewExtendedDaemonsetSettingOptions{
		CreationTime: now,
		Selector:     commonLabels,
	}
	edsNode1 := test.NewExtendedDaemonsetSetting("bar", "foo", "app", edsOptions1)
	edsOptions2 := &test.NewExtendedDaemonsetSettingOptions{
		CreationTime: now.Add(time.Minute),
		Selector:     commonLabels,
	}
	edsNode2 := test.NewExtendedDaemonsetSetting("bar", "foo2", "app", edsOptions2)
	edsOptions3 := &test.NewExtendedDaemonsetSettingOptions{
		CreationTime: now.Add(time.Minute),
		Selector:     commonLabels,
		SelectorRequirement: []metav1.LabelSelectorRequirement{
			{
				Operator: metav1.LabelSelectorOpIn,
				Key:      "test",
				Values:   []string{"bigmemory"},
			},
		},
	}
	edsNode3 := test.NewExtendedDaemonsetSetting("bar", "foo3", "app", edsOptions3)
	edsOptions4 := &test.NewExtendedDaemonsetSettingOptions{
		CreationTime: now.Add(time.Minute),
		Selector:     commonLabels,
		SelectorRequirement: []metav1.LabelSelectorRequirement{
			{
				Operator: metav1.LabelSelectorOperator("invalid operator"),
				Key:      "test",
				Values:   []string{"bigmemory"},
			},
		},
	}
	edsNode4 := test.NewExtendedDaemonsetSetting("bar", "foo3", "app", edsOptions4)
	nodeOptions := &commontest.NewNodeOptions{
		Labels: commonLabels,
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	node1 := commontest.NewNode("node1", nodeOptions)
	type args struct {
		instance    *datadoghqv1alpha1.ExtendedDaemonsetSetting
		nodeList    *corev1.NodeList
		edsNodeList *datadoghqv1alpha1.ExtendedDaemonsetSettingList
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "No Node",
			args: args{
				instance: edsNode1,
				nodeList: &corev1.NodeList{},
				edsNodeList: &datadoghqv1alpha1.ExtendedDaemonsetSettingList{
					Items: []datadoghqv1alpha1.ExtendedDaemonsetSetting{*edsNode1},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "No ExtendedDaemonsetSetting",
			args: args{
				instance: nil,
				nodeList: &corev1.NodeList{
					Items: []corev1.Node{*node1},
				},
				edsNodeList: &datadoghqv1alpha1.ExtendedDaemonsetSettingList{
					Items: []datadoghqv1alpha1.ExtendedDaemonsetSetting{},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "1 ExtendedDaemonsetSetting, no conflict",
			args: args{
				instance: edsNode1,
				nodeList: &corev1.NodeList{
					Items: []corev1.Node{*node1},
				},
				edsNodeList: &datadoghqv1alpha1.ExtendedDaemonsetSettingList{
					Items: []datadoghqv1alpha1.ExtendedDaemonsetSetting{*edsNode1},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "1 ExtendedDaemonsetSetting, conflict between 2 ExtendedDaemonsetSettings",
			args: args{
				instance: edsNode1,
				nodeList: &corev1.NodeList{
					Items: []corev1.Node{*node1},
				},
				edsNodeList: &datadoghqv1alpha1.ExtendedDaemonsetSettingList{
					Items: []datadoghqv1alpha1.ExtendedDaemonsetSetting{*edsNode1, *edsNode2},
				},
			},
			want:    "foo2",
			wantErr: true,
		},
		{
			name: "1 ExtendedDaemonsetSetting, using LabelSelectorRequirement",
			args: args{
				instance: edsNode3,
				nodeList: &corev1.NodeList{
					Items: []corev1.Node{*node1},
				},
				edsNodeList: &datadoghqv1alpha1.ExtendedDaemonsetSettingList{
					Items: []datadoghqv1alpha1.ExtendedDaemonsetSetting{*edsNode1, *edsNode2, *edsNode3},
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "1 ExtendedDaemonsetSetting, using LabelSelectorRequirement with invalid operator",
			args: args{
				instance: edsNode4,
				nodeList: &corev1.NodeList{
					Items: []corev1.Node{*node1},
				},
				edsNodeList: &datadoghqv1alpha1.ExtendedDaemonsetSettingList{
					Items: []datadoghqv1alpha1.ExtendedDaemonsetSetting{*edsNode4},
				},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := searchPossibleConflict(tt.args.instance, tt.args.nodeList, tt.args.edsNodeList)
			if (err != nil) != tt.wantErr {
				t.Errorf("searchPossibleConflict() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if got != tt.want {
				t.Errorf("searchPossibleConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}
