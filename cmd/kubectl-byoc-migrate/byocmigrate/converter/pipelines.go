// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package converter

import (
	"fmt"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const workerAPIPortName = "api"

// pipelineSpec reads a pipeline from the StatefulSet, Service, HPA, and PodDisruptionBudget of a Worker manifest.
func pipelineSpec(name string, objects []client.Object, statefulSet *appsv1.StatefulSet) (datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec, error) {
	template := statefulSet.Spec.Template
	if len(template.Spec.Containers) == 0 {
		return datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{}, fmt.Errorf("StatefulSet %s has no container", statefulSet.Name)
	}
	pod := template.Spec
	container := pod.Containers[0]
	component := datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
		EnvFrom:                       container.EnvFrom,
		Volumes:                       slices.DeleteFunc(slices.Clone(pod.Volumes), func(volume corev1.Volume) bool { return volume.Name == dataVolumeName }),
		VolumeMounts:                  slices.DeleteFunc(slices.Clone(container.VolumeMounts), func(mount corev1.VolumeMount) bool { return mount.Name == dataVolumeName }),
		Annotations:                   withoutChecksums(template.Annotations),
		Labels:                        withoutKeys(template.Labels, chartLabels...),
		NodeSelector:                  pod.NodeSelector,
		Tolerations:                   pod.Tolerations,
		Affinity:                      pod.Affinity,
		TopologySpreadConstraints:     pod.TopologySpreadConstraints,
		InitContainers:                pod.InitContainers,
		TerminationGracePeriodSeconds: pod.TerminationGracePeriodSeconds,
		PodDisruptionBudget:           &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{},
	}
	if len(container.Resources.Limits) > 0 || len(container.Resources.Requests) > 0 {
		component.Resources = container.Resources.DeepCopy()
	}
	if budget, found := findObject(objects, func(*policyv1.PodDisruptionBudget) bool { return true }); found {
		component.PodDisruptionBudget = &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
			MinAvailable:   budget.Spec.MinAvailable,
			MaxUnavailable: budget.Spec.MaxUnavailable,
		}
	}
	stateful := datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
		DatadogBYOCClusterComponentSpec: component,
		Storage:                         storage(statefulSet),
		PodManagementPolicy:             podManagementPolicy(statefulSet),
	}
	if hpa, found := findObject(objects, func(hpa *autoscalingv2.HorizontalPodAutoscaler) bool {
		return hpa.Spec.ScaleTargetRef.Kind == "StatefulSet" && hpa.Spec.ScaleTargetRef.Name == statefulSet.Name
	}); found {
		stateful.Autoscaling = autoscaling(hpa)
	} else {
		stateful.Replicas = statefulSet.Spec.Replicas
	}
	return datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{
		Name: name,
		DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
			DatadogBYOCClusterStatefulComponentSpec: stateful,
			PipelineID:                              envValue(container.Env, "DD_OP_PIPELINE_ID"),
			Ports:                                   pipelinePorts(objects, statefulSet.Name),
		},
	}, nil
}

// pipelinePorts reads the ports of the Service named after the Worker. The Worker adds the API port itself.
func pipelinePorts(objects []client.Object, workerName string) []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort {
	service, found := findObject(objects, func(service *corev1.Service) bool { return service.Name == workerName })
	if !found {
		return nil
	}
	var ports []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort
	for _, port := range service.Spec.Ports {
		if port.Name == workerAPIPortName {
			continue
		}
		ports = append(ports, datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{Name: port.Name, Port: port.Port, Protocol: port.Protocol})
	}
	return ports
}

// withoutChecksums removes the checksum annotations that the charts set to restart Pods when a configuration changes.
func withoutChecksums(annotations map[string]string) map[string]string {
	var checksums []string
	for key := range annotations {
		if strings.HasPrefix(key, "checksum/") {
			checksums = append(checksums, key)
		}
	}
	return withoutKeys(annotations, checksums...)
}
