// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type serviceValues struct {
	Metadata metav1.ObjectMeta
	Selector map[string]string
	Ports    []corev1.ServicePort
}

func createService(values serviceValues) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: *values.Metadata.DeepCopy(),
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: maps.Clone(values.Selector),
			Ports:    slices.Clone(values.Ports),
		},
	}
}
