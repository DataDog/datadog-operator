// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package pod

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	// DaemonsetClusterAutoscalerPodAnnotationKey use to inform the cluster-autoscaler that a pod
	// should be considered as a DaemonSet pod.
	DaemonsetClusterAutoscalerPodAnnotationKey = "cluster-autoscaler.kubernetes.io/daemonset-pod"
)

// Should be const but GO doesn't support const structures.
var (
	// StandardDaemonSetTolerations contains the tolerations that the EDS controller should add to
	// all pods it manages.
	// For consistency, this list must be in sync with the tolerations that are automatically added
	// by the regular kubernetes DaemonSet controller:
	// https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/#taints-and-tolerations.
	StandardDaemonSetTolerations = []corev1.Toleration{
		{
			Key:      "node.kubernetes.io/not-ready",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoExecute,
		},
		{
			Key:      "node.kubernetes.io/unreachable",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoExecute,
		},
		{
			Key:      "node.kubernetes.io/disk-pressure",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "node.kubernetes.io/memory-pressure",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "node.kubernetes.io/unschedulable",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "node.kubernetes.io/network-unavailable",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}
)
