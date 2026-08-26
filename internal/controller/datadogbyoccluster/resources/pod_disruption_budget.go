// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"
	"maps"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type podDisruptionBudgetBuilder struct {
	metadata      metav1.ObjectMeta
	selector      map[string]string
	globalSpec    *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec
	componentSpec *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec
}

func newPodDisruptionBudgetBuilder(
	metadata metav1.ObjectMeta,
	selector map[string]string,
	globalSpec, componentSpec *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec,
) podDisruptionBudgetBuilder {
	return podDisruptionBudgetBuilder{
		metadata:      metadata,
		selector:      selector,
		globalSpec:    globalSpec,
		componentSpec: componentSpec,
	}
}

func (b podDisruptionBudgetBuilder) build() (*policyv1.PodDisruptionBudget, error) {
	spec := b.componentSpec
	if spec == nil {
		spec = b.globalSpec
	}
	if spec != nil && spec.MinAvailable == nil && spec.MaxUnavailable == nil {
		return nil, nil
	}

	var minAvailable, maxUnavailable *intstr.IntOrString
	switch {
	case spec == nil:
		maxUnavailable = ptr.To(intstr.FromInt32(1))
	case spec.MinAvailable != nil && spec.MaxUnavailable != nil:
		return nil, fmt.Errorf("%s pod disruption budget: minAvailable and maxUnavailable are mutually exclusive", b.metadata.Name)
	case spec.MinAvailable != nil:
		minAvailable = ptr.To(*spec.MinAvailable)
	default:
		maxUnavailable = ptr.To(*spec.MaxUnavailable)
	}

	metadata := b.metadata.DeepCopy()
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: *metadata,
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   minAvailable,
			MaxUnavailable: maxUnavailable,
			Selector:       &metav1.LabelSelector{MatchLabels: maps.Clone(b.selector)},
		},
	}, nil
}
