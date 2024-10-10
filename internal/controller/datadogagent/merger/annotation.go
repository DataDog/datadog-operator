// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	corev1 "k8s.io/api/core/v1"
)

// AnnotationManager is used to manage adding annotations in a PodTemplateSpec
type AnnotationManager interface {
	// AddAnnotation use to add an annotation to a Pod.
	AddAnnotation(key, value string)
}

// NewAnnotationManager returns a new instance of the AnnotationManager
func NewAnnotationManager(podTmpl *corev1.PodTemplateSpec) AnnotationManager {
	return &annotationManagerImpl{
		podTmpl: podTmpl,
	}
}

type annotationManagerImpl struct {
	podTmpl *corev1.PodTemplateSpec
}

func (impl *annotationManagerImpl) AddAnnotation(key, value string) {
	impl.podTmpl.Annotations[key] = value
}
