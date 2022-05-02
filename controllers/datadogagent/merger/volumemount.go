// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	corev1 "k8s.io/api/core/v1"
)

// VolumeMountMergeFunction signature for corev1.VolumeMount merge function
type VolumeMountMergeFunction func(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error)

// DefaultVolumeMountMergeFunction default corev1.VolumeMount merge function
// default correspond to OverrideCurrentVolumeMountMergeOption
func DefaultVolumeMountMergeFunction(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error) {
	return OverrideCurrentVolumeMountMergeFunction(current, newVolumeMount)
}

// OverrideCurrentVolumeMountMergeFunction used when the existing corev1.VolumeMount new to be replace by the new one.
func OverrideCurrentVolumeMountMergeFunction(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error) {
	return newVolumeMount.DeepCopy(), nil
}

// IgnoreNewVolumeMountMergeFunction used when the existing corev1.VolumeMount needs to be kept.
func IgnoreNewVolumeMountMergeFunction(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error) {
	return current.DeepCopy(), nil
}

// ErrorOnMergeAttemptdVolumeMountMergeFunction used to avoid replacing an existing VolumeMount
func ErrorOnMergeAttemptdVolumeMountMergeFunction(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error) {
	return nil, errMergeAttempted
}

// AddVolumeMountToContainer use to add a corev1.VolumeMount to a container
// the mergeFunc can be provided to change the default merge behavior
func AddVolumeMountToContainer(container *corev1.Container, volumeMount *corev1.VolumeMount, mergeFunc VolumeMountMergeFunction) ([]corev1.VolumeMount, error) {
	var found bool
	for id, cVolumeMount := range container.VolumeMounts {
		if volumeMount.Name == cVolumeMount.Name {
			if mergeFunc == nil {
				mergeFunc = DefaultVolumeMountMergeFunction
			}
			newVolumeMount, err := mergeFunc(&cVolumeMount, volumeMount)
			if err != nil {
				return nil, err
			}
			container.VolumeMounts[id] = *newVolumeMount
			found = true
		}
	}
	if !found {
		container.VolumeMounts = append(container.VolumeMounts, *volumeMount)
	}
	return container.VolumeMounts, nil
}
