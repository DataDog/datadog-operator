// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newDeployment(metadata metav1.ObjectMeta, selector map[string]string, template corev1.PodTemplateSpec, replicas int32, strategy appsv1.DeploymentStrategyType) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metadata,
		Spec: appsv1.DeploymentSpec{
			Replicas: new(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: template,
			Strategy: appsv1.DeploymentStrategy{Type: strategy},
		},
	}
}
