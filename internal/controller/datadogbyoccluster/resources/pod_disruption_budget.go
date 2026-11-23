// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// newPodDisruptionBudget returns nil when the effective budget is explicitly empty.
func newPodDisruptionBudget(
	metadata metav1.ObjectMeta,
	selector map[string]string,
	global, component *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec,
) (*policyv1.PodDisruptionBudget, error) {
	spec := component
	if spec == nil {
		spec = global
	}

	var minAvailable, maxUnavailable *intstr.IntOrString
	switch {
	case spec == nil:
		maxUnavailable = new(intstr.FromInt32(1))
	case spec.MinAvailable != nil && spec.MaxUnavailable != nil:
		return nil, fmt.Errorf("%s pod disruption budget: minAvailable and maxUnavailable are mutually exclusive", metadata.Name)
	case spec.MinAvailable != nil:
		minAvailable = spec.MinAvailable
	case spec.MaxUnavailable != nil:
		maxUnavailable = spec.MaxUnavailable
	default:
		return nil, nil
	}

	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metadata,
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   minAvailable,
			MaxUnavailable: maxUnavailable,
			Selector:       &metav1.LabelSelector{MatchLabels: selector},
		},
	}, nil
}
