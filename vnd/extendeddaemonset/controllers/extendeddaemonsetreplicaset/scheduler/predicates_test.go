// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package scheduler

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	ctrltest "github.com/DataDog/extendeddaemonset/pkg/controller/test"
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils/pod"
)

func TestCheckNodeFitness(t *testing.T) {
	now := time.Now()
	logf.SetLogger(zap.New())
	log := logf.Log.WithName("TestCheckNodeFitness")

	nodeReadyOptions := &ctrltest.NewNodeOptions{
		Labels: map[string]string{"app": "foo"},
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	nodeKOOptions := &ctrltest.NewNodeOptions{
		Labels: map[string]string{"app": "foo"},
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			},
		},
		Taints: []corev1.Taint{
			{
				Key:    "node.kubernetes.io/not-ready",
				Effect: corev1.TaintEffectNoExecute,
			},
		},
	}
	nodeUnscheduledOptions := &ctrltest.NewNodeOptions{
		Labels:        map[string]string{"app": "foo"},
		Unschedulable: true,
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
		Taints: []corev1.Taint{
			{
				Key:    "node.kubernetes.io/unschedulable",
				Effect: corev1.TaintEffectNoSchedule,
			},
		},
	}
	nodeTaintedOptions := &ctrltest.NewNodeOptions{
		Labels: map[string]string{"app": "foo"},
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
		Taints: []corev1.Taint{
			{
				Key:    "mytaint",
				Effect: corev1.TaintEffectNoSchedule,
			},
		},
	}
	node1 := ctrltest.NewNode("node1", nodeReadyOptions)
	node2 := ctrltest.NewNode("node2", nodeKOOptions)
	node3 := ctrltest.NewNode("node3", nodeUnscheduledOptions)
	node4 := ctrltest.NewNode("node4", nodeTaintedOptions)

	pod1 := ctrltest.NewPod("foo", "pod1", "", &ctrltest.NewPodOptions{
		CreationTimestamp: metav1.NewTime(now),
		NodeSelector:      map[string]string{"app": "foo"},
		Tolerations:       pod.StandardDaemonSetTolerations,
	})

	// Non-matching NodeSelectorRequirement MatchExpressions key
	pod2 := ctrltest.NewPod("foo", "pod2", "", &ctrltest.NewPodOptions{
		CreationTimestamp: metav1.NewTime(now),
		NodeSelector:      map[string]string{"app": "foo"},
		Tolerations:       pod.StandardDaemonSetTolerations,
		Affinity: corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key: "string",
								},
							},
						},
					},
				},
			},
		},
	})

	// RequiredDuringSchedulingIgnoredDuringExecution is nil
	pod3 := ctrltest.NewPod("foo", "pod3", "", &ctrltest.NewPodOptions{
		CreationTimestamp: metav1.NewTime(now),
		NodeSelector:      map[string]string{"app": "foo"},
		Tolerations:       pod.StandardDaemonSetTolerations,
		Affinity: corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: nil,
			},
		},
	})

	// Non-matching NodeSelectorRequirement MatchFields key
	pod4 := ctrltest.NewPod("foo", "pod4", "", &ctrltest.NewPodOptions{
		CreationTimestamp: metav1.NewTime(now),
		NodeSelector:      map[string]string{"app": "foo"},
		Tolerations:       pod.StandardDaemonSetTolerations,
		Affinity: corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchFields: []corev1.NodeSelectorRequirement{
								{
									Key: "string",
								},
							},
						},
					},
				},
			},
		},
	})

	// Empty MatchFields and MatchExpressions
	pod5 := ctrltest.NewPod("foo", "pod5", "", &ctrltest.NewPodOptions{
		CreationTimestamp: metav1.NewTime(now),
		NodeSelector:      map[string]string{"app": "foo"},
		Tolerations:       pod.StandardDaemonSetTolerations,
		Affinity: corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchFields:      []corev1.NodeSelectorRequirement{},
							MatchExpressions: []corev1.NodeSelectorRequirement{},
						},
					},
				},
			},
		},
	})

	type args struct {
		pod  *corev1.Pod
		node *corev1.Node
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "node ready",
			args: args{
				pod:  pod1,
				node: node1,
			},
			want: true,
		},
		{
			name: "node not ready",
			args: args{
				pod:  pod1,
				node: node2,
			},
			want: true,
		},
		{
			name: "node unschedulable",
			args: args{
				pod:  pod1,
				node: node3,
			},
			want: true,
		},
		{
			name: "node tainted",
			args: args{
				pod:  pod1,
				node: node4,
			},
			want: false,
		},
		{
			name: "pod with match expression",
			args: args{
				pod:  pod2,
				node: node1,
			},
			want: false,
		},
		{
			name: "pod with nil RequiredDuringSchedulingIgnoredDuringExecution",
			args: args{
				pod:  pod3,
				node: node1,
			},
			want: true,
		},
		{
			name: "pod with match field",
			args: args{
				pod:  pod4,
				node: node1,
			},
			want: false,
		},
		{
			name: "pod with empty match expression and field",
			args: args{
				pod:  pod5,
				node: node1,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckNodeFitness(log.WithName(tt.name), tt.args.pod, tt.args.node); got != tt.want {
				t.Errorf("CheckNodeFitness() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeSelectorRequirementsAsSelector(t *testing.T) {
	foobarReq, _ := labels.NewRequirement("foo", selection.In, []string{"bar"})
	bazquxReq, _ := labels.NewRequirement("baz", selection.NotIn, []string{"qux"})

	foobarReq2, _ := labels.NewRequirement("foo", selection.Exists, []string{})
	bazquxReq2, _ := labels.NewRequirement("baz", selection.DoesNotExist, []string{})

	foobarReq3, _ := labels.NewRequirement("foo", selection.GreaterThan, []string{"1"})
	bazquxReq3, _ := labels.NewRequirement("baz", selection.LessThan, []string{"2"})

	tests := []struct {
		name    string
		nsm     []corev1.NodeSelectorRequirement
		want    labels.Selector
		wantErr bool
	}{
		{
			name: "NodeSelectorOpIn",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpIn,
					Key:      "foo",
					Values:   []string{"bar"},
				},
			},
			want:    labels.NewSelector().Add(*foobarReq),
			wantErr: false,
		},
		{
			name: "NodeSelectorOpNotIn",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpNotIn,
					Key:      "baz",
					Values:   []string{"qux"},
				},
			},
			want:    labels.NewSelector().Add(*bazquxReq),
			wantErr: false,
		},
		{
			name: "NodeSelectorOpExists",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpExists,
					Key:      "foo",
					Values:   []string{},
				},
			},
			want:    labels.NewSelector().Add(*foobarReq2),
			wantErr: false,
		},
		{
			name: "NodeSelectorOpDoesNotExist",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpDoesNotExist,
					Key:      "baz",
					Values:   []string{},
				},
			},
			want:    labels.NewSelector().Add(*bazquxReq2),
			wantErr: false,
		},
		{
			name: "NodeSelectorOpGt",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpGt,
					Key:      "foo",
					Values:   []string{"1"},
				},
			},
			want:    labels.NewSelector().Add(*foobarReq3),
			wantErr: false,
		},
		{
			name: "NodeSelectorOpLt",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpLt,
					Key:      "baz",
					Values:   []string{"2"},
				},
			},
			want:    labels.NewSelector().Add(*bazquxReq3),
			wantErr: false,
		},
		{
			name: "Invalid",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOperator("Equals"),
					Key:      "foo",
					Values:   []string{"bar"},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := NodeSelectorRequirementsAsSelector(tt.nsm)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeSelectorRequirementsAsSelector() = %v, want %v", got, tt.want)
			}
			if tt.wantErr && gotErr == nil {
				t.Errorf("NodeSelectorRequirementsAsSelector() wanted an error but it is nil")
			}
			if !tt.wantErr && gotErr != nil {
				t.Errorf("NodeSelectorRequirementsAsSelector() error = %v, should be nil", gotErr)
			}
		})
	}
}

func TestNodeSelectorRequirementsAsFieldSelector(t *testing.T) {
	tests := []struct {
		name    string
		nsm     []corev1.NodeSelectorRequirement
		want    fields.Selector
		wantErr bool
	}{
		{
			name: "NodeSelectorOpIn",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpIn,
					Key:      "foo",
					Values:   []string{"bar"},
				},
			},
			want:    fields.AndSelectors([]fields.Selector{fields.OneTermEqualSelector("foo", "bar")}...),
			wantErr: false,
		},
		{
			name: "NodeSelectorOpNotIn",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpNotIn,
					Key:      "foo",
					Values:   []string{"bar"},
				},
			},
			want:    fields.AndSelectors([]fields.Selector{fields.OneTermNotEqualSelector("foo", "bar")}...),
			wantErr: false,
		},
		{
			name: "NodeSelectorOpIn",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpIn,
					Key:      "foo",
					Values:   []string{"bar", "baz"},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "NodeSelectorOpNotIn",
			nsm: []corev1.NodeSelectorRequirement{
				{
					Operator: corev1.NodeSelectorOpNotIn,
					Key:      "foo",
					Values:   []string{"bar", "baz"},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := NodeSelectorRequirementsAsFieldSelector(tt.nsm)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeSelectorRequirementsAsFieldSelector() = %v, want %v", got, tt.want)
			}
			if !tt.wantErr && gotErr != nil {
				t.Errorf("NodeSelectorRequirementsAsFieldSelector() error = %v, should be nil", gotErr)
			}
			if tt.wantErr && gotErr == nil {
				t.Errorf("NodeSelectorRequirementsAsFieldSelector() error = nil, should be %v", tt.wantErr)
			}
		})
	}
}
