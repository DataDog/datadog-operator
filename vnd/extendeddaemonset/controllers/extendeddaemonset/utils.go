// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonset

import (
	"time"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/conditions"
)

// IsRollingUpdatePaused checks if a rolling update has been paused.
func IsRollingUpdatePaused(dsAnnotations map[string]string) bool {
	return dsAnnotations[datadoghqv1alpha1.ExtendedDaemonSetRollingUpdatePausedAnnotationKey] == datadoghqv1alpha1.ValueStringTrue
}

// IsRolloutFrozen checks if a rollout has been freezed.
func IsRolloutFrozen(dsAnnotations map[string]string) bool {
	return dsAnnotations[datadoghqv1alpha1.ExtendedDaemonSetRolloutFrozenAnnotationKey] == datadoghqv1alpha1.ValueStringTrue
}

// IsCanaryDeploymentEnded used to know if the Canary duration has finished.
// If the duration is completed: return true
// If the duration is not completed: return false and the remaining duration.
func IsCanaryDeploymentEnded(specCanary *datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary, rs *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, now time.Time) (bool, time.Duration) {
	var pendingDuration time.Duration
	if specCanary == nil {
		return true, pendingDuration
	}
	if specCanary.Duration == nil {
		// in this case, it means the canary never ends
		return false, pendingDuration
	}

	var lastRestartTime time.Time

	restartCondition := conditions.GetExtendedDaemonSetReplicaSetStatusCondition(&rs.Status, datadoghqv1alpha1.ConditionTypePodRestarting)
	if restartCondition != nil {
		lastRestartTime = restartCondition.LastUpdateTime.Time
	}

	pendingNoRestartDuration := -specCanary.Duration.Duration
	if specCanary.NoRestartsDuration != nil && !lastRestartTime.IsZero() {
		pendingNoRestartDuration = lastRestartTime.Add(specCanary.NoRestartsDuration.Duration).Sub(now)
	}

	pendingDuration = rs.CreationTimestamp.Add(specCanary.Duration.Duration).Sub(now)

	if pendingNoRestartDuration > pendingDuration {
		pendingDuration = pendingNoRestartDuration
	}

	if pendingDuration >= 0 {
		return false, pendingDuration
	}

	return true, pendingDuration
}

// IsCanaryDeploymentPaused checks if the Canary deployment has been paused.
func IsCanaryDeploymentPaused(dsAnnotations map[string]string, ers *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet) (bool, datadoghqv1alpha1.ExtendedDaemonSetStatusReason) {
	// check ERS status to detect if a Canary paused
	if ers != nil && conditions.IsConditionTrue(&ers.Status, datadoghqv1alpha1.ConditionTypeCanaryPaused) {
		cond := conditions.GetExtendedDaemonSetReplicaSetStatusCondition(&ers.Status, datadoghqv1alpha1.ConditionTypeCanaryPaused)

		return true, datadoghqv1alpha1.ExtendedDaemonSetStatusReason(cond.Reason)
	}

	// Check annotations is a user have added the pause annotation.
	isPaused, found := dsAnnotations[datadoghqv1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey]
	if found && isPaused == datadoghqv1alpha1.ValueStringTrue {
		if reason, found := dsAnnotations[datadoghqv1alpha1.ExtendedDaemonSetCanaryPausedReasonAnnotationKey]; found {
			return true, datadoghqv1alpha1.ExtendedDaemonSetStatusReason(reason)
		}

		return true, datadoghqv1alpha1.ExtendedDaemonSetStatusReasonUnknown
	}

	return false, ""
}

// IsCanaryDeploymentUnpaused checks if the Canary deployment has been manually unpaused.
func IsCanaryDeploymentUnpaused(dsAnnotations map[string]string) bool {
	isUnpaused, found := dsAnnotations[datadoghqv1alpha1.ExtendedDaemonSetCanaryUnpausedAnnotationKey]
	if found {
		return isUnpaused == datadoghqv1alpha1.ValueStringTrue
	}

	return false
}

// IsCanaryDeploymentValid used to know if the Canary deployment has been declared
// valid even if its duration has not finished yet.
// If the ExtendedDaemonSet has the corresponding annotation: return true.
func IsCanaryDeploymentValid(dsAnnotations map[string]string, rsName string) bool {
	if value, found := dsAnnotations[datadoghqv1alpha1.ExtendedDaemonSetCanaryValidAnnotationKey]; found {
		return value == rsName
	}

	return false
}

// IsCanaryDeploymentFailed checks if the Canary deployment has been failed.
func IsCanaryDeploymentFailed(ers *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet) bool {
	// Check ERS status to detect if a Canary failed
	if ers != nil && conditions.IsConditionTrue(&ers.Status, datadoghqv1alpha1.ConditionTypeCanaryFailed) {
		return true
	}

	return false
}
