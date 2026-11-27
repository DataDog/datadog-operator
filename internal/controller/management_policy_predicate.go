// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// managementPolicyChangedPredicate triggers a reconcile when the
// datadoghq.com/management-policy annotation is added, changed or removed.
// Annotation changes don't bump metadata.generation, so the
// GenerationChangedPredicate used by the Datadog resource controllers misses
// them. Other events are left to the other predicates.
func managementPolicyChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}
			return e.ObjectOld.GetAnnotations()[v1alpha1.ManagementPolicyAnnotationKey] != e.ObjectNew.GetAnnotations()[v1alpha1.ManagementPolicyAnnotationKey]
		},
	}
}

// datadogResourcePredicate is the event filter of the Datadog resource
// controllers (DatadogGenericResource, DatadogMonitor, DatadogDashboard,
// DatadogSLO).
func datadogResourcePredicate() predicate.Predicate {
	return predicate.Or(predicate.GenerationChangedPredicate{}, managementPolicyChangedPredicate())
}
