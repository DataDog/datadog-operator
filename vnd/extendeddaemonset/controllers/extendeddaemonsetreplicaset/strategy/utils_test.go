// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package strategy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/api/v1alpha1/test"
	commontest "github.com/DataDog/extendeddaemonset/pkg/controller/test"
)

func Test_compareWithExtendedDaemonsetSettingOverwrite(t *testing.T) {
	nodeName1 := "node1"
	nodeOptions := &commontest.NewNodeOptions{}
	node1 := commontest.NewNode(nodeName1, nodeOptions)

	resource1 := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			"cpu": resource.MustParse("1"),
		},
	}
	pod1Option := &commontest.NewPodOptions{Resources: *resource1}
	pod1 := commontest.NewPod("bar", "pod1", nodeName1, pod1Option)
	pod1.Spec.Containers[0].Resources = *resource1

	edsNode1Options := &test.NewExtendedDaemonsetSettingOptions{
		Resources: map[string]corev1.ResourceRequirements{
			"pod1": *resource1,
		},
	}
	extendedDaemonsetSetting1 := test.NewExtendedDaemonsetSetting("bar", "foo", "foo", edsNode1Options)

	edsNode2Options := &test.NewExtendedDaemonsetSettingOptions{
		Resources: map[string]corev1.ResourceRequirements{
			"pod1": {
				Requests: corev1.ResourceList{
					"cpu":    resource.MustParse("2"),
					"memory": resource.MustParse("1G"),
				},
			},
		},
	}
	extendedDaemonsetSetting2 := test.NewExtendedDaemonsetSetting("bar", "foo", "foo", edsNode2Options)

	type args struct {
		pod  *corev1.Pod
		node *NodeItem
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "empty ExtendedDaemonsetSetting",
			args: args{
				pod: pod1,
				node: &NodeItem{
					Node: node1,
				},
			},
			want: true,
		},
		{
			name: "ExtendedDaemonsetSetting that match",
			args: args{
				pod: pod1,
				node: &NodeItem{
					Node:                     node1,
					ExtendedDaemonsetSetting: extendedDaemonsetSetting1,
				},
			},
			want: true,
		},
		{
			name: "ExtendedDaemonsetSetting doesn't match",
			args: args{
				pod: pod1,
				node: &NodeItem{
					Node:                     node1,
					ExtendedDaemonsetSetting: extendedDaemonsetSetting2,
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareWithExtendedDaemonsetSettingOverwrite(tt.args.pod, tt.args.node); got != tt.want {
				t.Errorf("compareWithExtendedDaemonsetSettingOverwrite() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_addPodLabel(t *testing.T) {
	logf.SetLogger(zap.New())
	testLogger := logf.Log.WithName("test")
	key := "key1"
	val := "val1"
	podNoLabel := commontest.NewPod("foo", "pod1", "node1", nil)
	podLabeled := podNoLabel.DeepCopy()
	podLabeled.Labels = make(map[string]string)
	podLabeled.Labels[key] = val

	validatationLabelFunc := func(t *testing.T, c client.Client, pod *corev1.Pod) {
		wantPod := &corev1.Pod{}
		nNs := types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}
		err := c.Get(context.TODO(), nNs, wantPod)
		require.NoErrorf(t, err, "error must be nil, err: %v", err)

		if gotVal, ok := wantPod.Labels[key]; ok {
			assert.Equal(t, val, gotVal)
		} else {
			t.Fatalf("Label not present, pod: %#v", wantPod.Labels)
		}
	}

	type args struct {
		c   client.Client
		pod *corev1.Pod
		k   string
		v   string
	}
	tests := []struct {
		name           string
		args           args
		validationFunc func(*testing.T, client.Client, *corev1.Pod)
		wantErr        bool
	}{
		{
			name: "add label",
			args: args{
				c:   fake.NewClientBuilder().WithObjects(podNoLabel).Build(),
				pod: podNoLabel,
				k:   key,
				v:   val,
			},
			validationFunc: validatationLabelFunc,
			wantErr:        false,
		},
		{
			name: "label already present",
			args: args{
				c:   fake.NewClientBuilder().WithObjects(podLabeled).Build(),
				pod: podLabeled,
				k:   key,
				v:   val,
			},
			wantErr:        false,
			validationFunc: validatationLabelFunc,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testLogger.WithName(tt.name)
			if err := addPodLabel(logger, tt.args.c, tt.args.pod, tt.args.k, tt.args.v); (err != nil) != tt.wantErr {
				t.Errorf("addPodLabel() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validationFunc != nil {
				tt.validationFunc(t, tt.args.c, tt.args.pod)
			}
		})
	}
}

func Test_deletePodLabel(t *testing.T) {
	logf.SetLogger(zap.New())
	testLogger := logf.Log.WithName("test")
	key := "key1"
	val := "val1"
	podNoLabel := commontest.NewPod("foo", "pod1", "node1", nil)
	podLabeled := podNoLabel.DeepCopy()
	podLabeled.Labels = make(map[string]string)
	podLabeled.Labels[key] = val

	validatationNoLabelFunc := func(t *testing.T, c client.Client, pod *corev1.Pod) {
		wantPod := &corev1.Pod{}
		nNs := types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}
		err := c.Get(context.TODO(), nNs, wantPod)
		require.NoErrorf(t, err, "error must be nil, err: %v", err)
		if _, ok := wantPod.Labels[key]; ok {
			t.Fatalf("Label is present, pod: %#v", wantPod.Labels)
		}
	}

	type args struct {
		c   client.Client
		pod *corev1.Pod
		k   string
	}
	tests := []struct {
		name           string
		args           args
		validationFunc func(*testing.T, client.Client, *corev1.Pod)
		wantErr        bool
	}{
		{
			name: "delete label",
			args: args{
				c:   fake.NewClientBuilder().WithObjects(podLabeled).Build(),
				pod: podLabeled,
				k:   key,
			},
			validationFunc: validatationNoLabelFunc,
			wantErr:        false,
		},
		{
			name: "label not present",
			args: args{
				c:   fake.NewClientBuilder().WithObjects(podNoLabel).Build(),
				pod: podNoLabel,
				k:   key,
			},
			wantErr:        false,
			validationFunc: validatationNoLabelFunc,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testLogger.WithName(tt.name)
			if err := deletePodLabel(logger, tt.args.c, tt.args.pod, tt.args.k); (err != nil) != tt.wantErr {
				t.Errorf("deletePodLabel() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validationFunc != nil {
				tt.validationFunc(t, tt.args.c, tt.args.pod)
			}
		})
	}
}

func Test_cleanupPods(t *testing.T) {
	logf.SetLogger(zap.New())
	logger := logf.Log.WithName("test")

	status := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus{
		Status:    "canary",
		Desired:   3,
		Current:   1,
		Ready:     1,
		Available: 1,
	}

	pod1 := newTestCanaryPod("foo-a", "v1", readyPodStatus)
	pods := []*corev1.Pod{
		pod1,
	}
	client := fake.NewClientBuilder().WithObjects(pod1).Build()

	err := cleanupPods(client, logger, status, pods)
	require.NoErrorf(t, err, "error must be nil, err: %v", err)
}

func Test_manageUnscheduledPodNodes(t *testing.T) {
	podStatus1 := corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionFalse,
				Reason: corev1.PodReasonUnschedulable,
			},
		},
	}
	pod1 := newTestCanaryPod("foo-a", "v1", podStatus1)
	pod1.Spec.NodeName = "test-node1"

	podStatus2 := corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionFalse,
				Reason: "",
			},
		},
	}
	pod2 := newTestCanaryPod("foo-b", "v1", podStatus2)
	pod2.Spec.NodeName = "test-node2"

	podStatus3 := corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	pod3 := newTestCanaryPod("foo-c", "v1", podStatus3)
	pod3.Spec.NodeName = "test-node3"

	pods := []*corev1.Pod{
		pod1,
		pod2,
		pod3,
	}

	nodes := manageUnscheduledPodNodes(pods)
	assert.Len(t, nodes, 1)
}
