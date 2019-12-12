// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

const (
	// ExtendedDaemonSetNameLabelKey label key use to link a ExtendedDaemonSetReplicaSet to a ExtendedDaemonSet
	ExtendedDaemonSetNameLabelKey = "extendeddaemonset.datadoghq.com/name"
	// ExtendedDaemonSetReplicaSetNameLabelKey label key use to link a Pod to a ExtendedDaemonSetReplicaSet
	ExtendedDaemonSetReplicaSetNameLabelKey = "extendeddaemonsetreplicaset.datadoghq.com/name"
	// MD5ExtendedDaemonSetAnnotationKey annotation key use on Pods in order to identify which PodTemplateSpec have been used to generate it.
	MD5ExtendedDaemonSetAnnotationKey = "extendeddaemonset.datadoghq.com/templatehash"
	// ExtendedDaemonSetCanaryValidAnnotationKey annotation key used on Pods in order to detect if a canary deployment is considered valid.
	ExtendedDaemonSetCanaryValidAnnotationKey = "extendeddaemonset.datadoghq.com/canary-valid"
	// ExtendedDaemonSetOldDaemonsetAnnotationKey annotation key used on ExtendedDaemonset in order to inform the controller that old Daemonset's pod.
	// should be taken into consideration during the initial rolling-update.
	ExtendedDaemonSetOldDaemonsetAnnotationKey = "extendeddaemonset.datadoghq.com/old-daemonset"
)
