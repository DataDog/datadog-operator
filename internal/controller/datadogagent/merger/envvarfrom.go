// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	corev1 "k8s.io/api/core/v1"
)

// EnvFromSourceFromMergeFunction signature for corev1.EnvFromSource merge function
type EnvFromSourceFromMergeFunction func(current, newEnv *corev1.EnvFromSource) (*corev1.EnvFromSource, error)

// DefaultEnvFromSourceFromMergeFunction default corev1.EnvFromSource merge function
// default correspond to OverrideCurrentEnvFromSourceMergeOption
func DefaultEnvFromSourceFromMergeFunction(current, newEnv *corev1.EnvFromSource) (*corev1.EnvFromSource, error) {
	return OverrideCurrentEnvFromSourceFromMergeFunction(current, newEnv)
}

// OverrideCurrentEnvFromSourceFromMergeFunction used when the existing corev1.EnvFromSource new to be replace by the new one.
func OverrideCurrentEnvFromSourceFromMergeFunction(current, newEnv *corev1.EnvFromSource) (*corev1.EnvFromSource, error) {
	return newEnv.DeepCopy(), nil
}

// IgnoreNewEnvFromSourceFromMergeFunction used when the existing corev1.EnvFromSource needs to be kept.
func IgnoreNewEnvFromSourceFromMergeFunction(current, newEnv *corev1.EnvFromSource) (*corev1.EnvFromSource, error) {
	return current.DeepCopy(), nil
}

// ErrorOnMergeAttemptdEnvFromSourceFromMergeFunction used to avoid replacing an existing corev1.EnvFromSource
func ErrorOnMergeAttemptdEnvFromSourceFromMergeFunction(current, newEnv *corev1.EnvFromSource) (*corev1.EnvFromSource, error) {
	return nil, errMergeAttempted
}

// AddEnvFromSourceFromToContainer use to add an EnvFromSource to container.
func AddEnvFromSourceFromToContainer(container *corev1.Container, envFromSource *corev1.EnvFromSource, mergeFunc EnvFromSourceFromMergeFunction) ([]corev1.EnvFromSource, error) {
	var found bool
	for id, cEnvFromSource := range container.EnvFrom {
		if cEnvFromSource.ConfigMapRef != nil && envFromSource.ConfigMapRef != nil && cEnvFromSource.ConfigMapRef.Name == envFromSource.ConfigMapRef.Name {
			found = true
		} else if cEnvFromSource.SecretRef != nil && envFromSource.SecretRef != nil && cEnvFromSource.SecretRef.Name == envFromSource.SecretRef.Name {
			found = true
		}

		if found {
			if mergeFunc == nil {
				mergeFunc = DefaultEnvFromSourceFromMergeFunction
			}
			newEnvFromSource, err := mergeFunc(&cEnvFromSource, envFromSource)
			if err != nil {
				return nil, err
			}
			container.EnvFrom[id] = *newEnvFromSource
		}
	}
	if !found {
		container.EnvFrom = append(container.EnvFrom, *envFromSource)
	}
	return container.EnvFrom, nil
}
