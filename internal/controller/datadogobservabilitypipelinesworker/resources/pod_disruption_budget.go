// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// newPodDisruptionBudget returns nil when no budget is configured.
func newPodDisruptionBudget(metadata metav1.ObjectMeta, selector map[string]string, spec *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec) (*policyv1.PodDisruptionBudget, error) {
	switch {
	case spec == nil || spec.MinAvailable == nil && spec.MaxUnavailable == nil:
		return nil, nil
	case spec.MinAvailable != nil && spec.MaxUnavailable != nil:
		return nil, fmt.Errorf("worker pod disruption budget minAvailable and maxUnavailable are mutually exclusive")
	}
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metadata,
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   spec.MinAvailable,
			MaxUnavailable: spec.MaxUnavailable,
			Selector:       &metav1.LabelSelector{MatchLabels: selector},
		},
	}, nil
}
