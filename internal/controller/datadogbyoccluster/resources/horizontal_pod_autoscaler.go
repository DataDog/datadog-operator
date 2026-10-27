// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func newHPA(metadata metav1.ObjectMeta, autoscaling *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metadata,
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: metadata.Name},
			MinReplicas:    autoscaling.MinReplicas,
			MaxReplicas:    *autoscaling.MaxReplicas,
			Metrics:        autoscaling.Metrics,
			Behavior:       autoscaling.Behavior,
		},
	}
}
