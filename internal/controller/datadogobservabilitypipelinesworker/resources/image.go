// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type workerImage struct {
	reference   string
	pullPolicy  corev1.PullPolicy
	pullSecrets []corev1.LocalObjectReference
}

func resolveImage(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec) (workerImage, error) {
	if spec == nil {
		return workerImage{}, fmt.Errorf("worker image is required")
	}
	if spec.Repository == nil || *spec.Repository == "" {
		return workerImage{}, fmt.Errorf("worker image repository is required")
	}
	if (spec.Tag == nil) == (spec.Digest == nil) {
		return workerImage{}, fmt.Errorf("worker image must specify exactly one of tag or digest")
	}

	reference := *spec.Repository
	if spec.Digest != nil {
		reference += "@" + *spec.Digest
	} else {
		reference += ":" + *spec.Tag
	}
	return workerImage{
		reference:   reference,
		pullPolicy:  ptr.Deref(spec.PullPolicy, corev1.PullIfNotPresent),
		pullSecrets: spec.ImagePullSecrets,
	}, nil
}
