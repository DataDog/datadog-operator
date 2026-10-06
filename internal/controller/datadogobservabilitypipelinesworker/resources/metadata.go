// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	controllerutils "github.com/DataDog/datadog-operator/internal/controller/utils"
)

// objectMeta returns the metadata shared by every resource of a Worker.
func objectMeta(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        worker.Name,
		Namespace:   worker.Namespace,
		Labels:      podLabels(worker),
		Annotations: maps.Clone(worker.Spec.Annotations),
	}
}

func obsoleteObjectMeta(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}
}

// podLabels lets Worker labels add to the selector without changing it.
func podLabels(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) map[string]string {
	return controllerutils.MergeStringMaps(worker.Spec.Labels, selectorLabels(worker))
}

func selectorLabels(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "observability-pipelines-worker",
		"app.kubernetes.io/instance": worker.Name,
	}
}
