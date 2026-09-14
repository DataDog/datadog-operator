// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"maps"
	"slices"

	"github.com/imdario/mergo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func mergeStringMaps(values ...map[string]string) map[string]string {
	var result map[string]string
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		if result == nil {
			result = make(map[string]string)
		}
		maps.Copy(result, value)
	}
	return result
}

func mergeEnv(base, override []corev1.EnvVar) []corev1.EnvVar {
	return mergeSlicesByKey(base, override, func(env corev1.EnvVar) string {
		return env.Name
	})
}

func mergeVolumes(base, override []corev1.Volume) []corev1.Volume {
	return mergeSlicesByKey(base, override, func(volume corev1.Volume) string {
		return volume.Name
	})
}

func mergeVolumeMounts(base, override []corev1.VolumeMount) []corev1.VolumeMount {
	return mergeSlicesByKey(base, override, func(mount corev1.VolumeMount) string {
		return mount.MountPath
	})
}

func mergeTopologySpreadConstraints(base, override []corev1.TopologySpreadConstraint) []corev1.TopologySpreadConstraint {
	type topologySpreadConstraintKey struct {
		topologyKey       string
		whenUnsatisfiable corev1.UnsatisfiableConstraintAction
	}
	return mergeSlicesByKey(base, override, func(constraint corev1.TopologySpreadConstraint) topologySpreadConstraintKey {
		return topologySpreadConstraintKey{
			topologyKey:       constraint.TopologyKey,
			whenUnsatisfiable: constraint.WhenUnsatisfiable,
		}
	})
}

func mergeAffinity(base, override *corev1.Affinity) (*corev1.Affinity, error) {
	if base == nil {
		return override.DeepCopy(), nil
	}
	if override == nil {
		return base.DeepCopy(), nil
	}
	baseMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(base)
	if err != nil {
		return nil, err
	}
	overrideMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(override)
	if err != nil {
		return nil, err
	}
	if err := mergo.Merge(&baseMap, overrideMap, mergo.WithOverride); err != nil {
		return nil, err
	}
	result := &corev1.Affinity{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(baseMap, result); err != nil {
		return nil, err
	}
	return result, nil
}

func mergeSlicesByKey[T any, K comparable](base, override []T, keyFor func(T) K) []T {
	result := slices.Clone(base)
	positions := make(map[K]int, len(result))
	for index, value := range result {
		positions[keyFor(value)] = index
	}
	for _, value := range override {
		key := keyFor(value)
		if index, found := positions[key]; found {
			result[index] = value
			continue
		}
		positions[key] = len(result)
		result = append(result, value)
	}
	return result
}
