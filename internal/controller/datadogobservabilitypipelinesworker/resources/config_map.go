// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func pipelineConfigMapName(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) string {
	return worker.Name + "-pipeline"
}

// newPipelineConfigMap holds spec.pipelineConfig for a Worker in local mode.
func newPipelineConfigMap(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) *corev1.ConfigMap {
	meta := objectMeta(worker)
	meta.Name = pipelineConfigMapName(worker)
	return &corev1.ConfigMap{
		ObjectMeta: meta,
		Data:       map[string]string{pipelineConfigFileName: *worker.Spec.PipelineConfig},
	}
}
