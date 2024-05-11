// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package test contains test helpers.
package test

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	utilaffinity "github.com/DataDog/extendeddaemonset/pkg/controller/utils/affinity"
)

// NewNodeOptions store NewNode options.
type NewNodeOptions struct {
	Annotations   map[string]string
	Labels        map[string]string
	Conditions    []corev1.NodeCondition
	Taints        []corev1.Taint
	Unschedulable bool
}

// NewNode returns new node instance.
func NewNode(name string, opts *NewNodeOptions) *corev1.Node {
	node := &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}

	if opts != nil {
		if opts.Annotations != nil {
			for key, value := range opts.Annotations {
				node.Annotations[key] = value
			}
		}
		if opts.Labels != nil {
			for key, value := range opts.Labels {
				node.Labels[key] = value
			}
		}

		node.Spec.Unschedulable = opts.Unschedulable
		node.Status.Conditions = append(node.Status.Conditions, opts.Conditions...)
		node.Spec.Taints = append(node.Spec.Taints, opts.Taints...)
	}

	return node
}

// NewPodOptions store NewPod options.
type NewPodOptions struct {
	CreationTimestamp metav1.Time
	Annotations       map[string]string
	Labels            map[string]string
	Phase             corev1.PodPhase
	Reason            string
	ContainerStatuses []corev1.ContainerStatus
	Resources         corev1.ResourceRequirements
	NodeSelector      map[string]string
	Tolerations       []corev1.Toleration
	Affinity          corev1.Affinity
}

// NewPod used to return new pod instance.
func NewPod(namespace, name, nodeName string, opts *NewPodOptions) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Affinity: &corev1.Affinity{},
			Containers: []corev1.Container{
				{
					Name: name,
				},
			},
		},
	}
	if opts != nil {
		pod.CreationTimestamp = opts.CreationTimestamp

		pod.Spec.Containers[0].Resources = opts.Resources

		if opts.Annotations != nil {
			for key, value := range opts.Annotations {
				pod.Annotations[key] = value
			}
		}
		if opts.Labels != nil {
			for key, value := range opts.Labels {
				pod.Labels[key] = value
			}
		}
		if opts.NodeSelector != nil {
			pod.Spec.NodeSelector = map[string]string{}
			for key, value := range opts.NodeSelector {
				pod.Spec.NodeSelector[key] = value
			}
		}
		pod.Status.Phase = opts.Phase
		pod.Status.Reason = opts.Reason
		pod.Status.ContainerStatuses = opts.ContainerStatuses
		pod.Spec.Tolerations = append(pod.Spec.Tolerations, opts.Tolerations...)
		pod.Spec.Affinity = &opts.Affinity
	}

	if nodeName != "" {
		utilaffinity.ReplaceNodeNameNodeAffinity(pod.Spec.Affinity, nodeName)
	}

	return pod
}
