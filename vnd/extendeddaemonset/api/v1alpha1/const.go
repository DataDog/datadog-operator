// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

const (
	// ExtendedDaemonSetNameLabelKey label key use to link a ExtendedDaemonSetReplicaSet to a ExtendedDaemonSet.
	ExtendedDaemonSetNameLabelKey = "extendeddaemonset.datadoghq.com/name"
	// ExtendedDaemonSetReplicaSetNameLabelKey label key use to link a Pod to a ExtendedDaemonSetReplicaSet.
	ExtendedDaemonSetReplicaSetNameLabelKey = "extendeddaemonsetreplicaset.datadoghq.com/name"
	// ExtendedDaemonSetSettingNameLabelKey label key use to link a Pod to a ExtendedDaemonSetSetting name.
	ExtendedDaemonSetSettingNameLabelKey = "extendeddaemonsetsetting.datadoghq.com/name"
	// ExtendedDaemonSetSettingNamespaceLabelKey label key use to link a Pod to a ExtendedDaemonSetSetting namespace.
	ExtendedDaemonSetSettingNamespaceLabelKey = "extendeddaemonsetsetting.datadoghq.com/namespace"
	// ExtendedDaemonSetReplicaSetCanaryLabelKey label key used to identify canary Pods.
	ExtendedDaemonSetReplicaSetCanaryLabelKey = "extendeddaemonsetreplicaset.datadoghq.com/canary"
	// ExtendedDaemonSetReplicaSetCanaryLabelValue label value used to identify canary Pods.
	ExtendedDaemonSetReplicaSetCanaryLabelValue = "true"
	// ExtendedDaemonSetReplicaSetUnreadyPodsAnnotationKey annotation key used on ExtendedDaemonSetReplicaSet to detect the number of unready pods at the moment of creation of the ExtendedDaemonSetReplicaSet
	ExtendedDaemonSetReplicaSetUnreadyPodsAnnotationKey = "extendeddaemonsetreplicaset.datadoghq.com/unready-pods"
	// MD5ExtendedDaemonSetAnnotationKey annotation key use on Pods in order to identify which PodTemplateSpec have been used to generate it.
	MD5ExtendedDaemonSetAnnotationKey = "extendeddaemonset.datadoghq.com/templatehash"
	// ExtendedDaemonSetCanaryValidAnnotationKey annotation key used on Pods in order to detect if a canary deployment is considered valid.
	ExtendedDaemonSetCanaryValidAnnotationKey = "extendeddaemonset.datadoghq.com/canary-valid"
	// ExtendedDaemonSetCanaryPausedAnnotationKey annotation key used on ExtendedDaemonset in order to detect if a canary deployment is paused.
	ExtendedDaemonSetCanaryPausedAnnotationKey = "extendeddaemonset.datadoghq.com/canary-paused"
	// ExtendedDaemonSetCanaryPausedReasonAnnotationKey annotation key used on ExtendedDaemonset to provide a reason that the a canary deployment is paused.
	ExtendedDaemonSetCanaryPausedReasonAnnotationKey = "extendeddaemonset.datadoghq.com/canary-paused-reason"
	// ExtendedDaemonSetCanaryUnpausedAnnotationKey annotation key used on ExtendedDaemonset in order to detect if a canary deployment is manually unpaused.
	ExtendedDaemonSetCanaryUnpausedAnnotationKey = "extendeddaemonset.datadoghq.com/canary-unpaused"
	// ExtendedDaemonSetOldDaemonsetAnnotationKey annotation key used on ExtendedDaemonset in order to inform the controller that old Daemonset's pod.
	// should be taken into consideration during the initial rolling-update.
	ExtendedDaemonSetOldDaemonsetAnnotationKey = "extendeddaemonset.datadoghq.com/old-daemonset"
	// ExtendedDaemonSetRessourceNodeAnnotationKey annotation key used on Node to overwrite the resource allocated to a specific container linked to an ExtendedDaemonset
	// The value format is: <eds-namespace>.<eds-name>.<container-name> .
	ExtendedDaemonSetRessourceNodeAnnotationKey = "resources.extendeddaemonset.datadoghq.com/%s.%s.%s"
	// MD5NodeExtendedDaemonSetAnnotationKey annotation key use on Pods in order to identify which Node Resources Overwride have been used to generate it.
	MD5NodeExtendedDaemonSetAnnotationKey = "extendeddaemonset.datadoghq.com/nodehash"
	// ExtendedDaemonSetRollingUpdatePausedAnnotationKey annotation key used on ExtendedDaemonset in order to detect if a rolling update is paused.
	ExtendedDaemonSetRollingUpdatePausedAnnotationKey = "extendeddaemonset.datadoghq.com/rolling-update-paused"
	// ExtendedDaemonSetRolloutFrozenAnnotationKey annotation key used on ExtendedDaemonset in order to detect if a rollout is frozen.
	ExtendedDaemonSetRolloutFrozenAnnotationKey = "extendeddaemonset.datadoghq.com/rollout-frozen"

	// ValueStringTrue is the string value of bool `true`.
	ValueStringTrue = "true"
	// ValueStringFalse is the string value of bool `false`.
	ValueStringFalse = "false"
)
