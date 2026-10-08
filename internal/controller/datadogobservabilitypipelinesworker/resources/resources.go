// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resources builds the Kubernetes resources managed by a
// DatadogObservabilityPipelinesWorker controller.
package resources

import (
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// Resources is the complete set of Kubernetes resources rendered for a Worker.
type Resources struct {
	Service             *corev1.Service
	ServiceAccount      *corev1.ServiceAccount
	StatefulSet         *appsv1.StatefulSet
	HPA                 *autoscalingv2.HorizontalPodAutoscaler
	PodDisruptionBudget *policyv1.PodDisruptionBudget
	obsoleteObjects     []client.Object
}

// Objects returns the resources in apply order.
func (r *Resources) Objects() []client.Object {
	var objects []client.Object
	if r.ServiceAccount != nil {
		objects = append(objects, r.ServiceAccount)
	}
	if r.Service != nil {
		objects = append(objects, r.Service)
	}
	objects = append(objects, r.StatefulSet)
	if r.HPA != nil {
		objects = append(objects, r.HPA)
	}
	if r.PodDisruptionBudget != nil {
		objects = append(objects, r.PodDisruptionBudget)
	}
	return objects
}

// ObsoleteObjects returns optional resources that are no longer desired.
func (r *Resources) ObsoleteObjects() []client.Object {
	return r.obsoleteObjects
}

// BuildResources builds deterministic Kubernetes resources for a Worker.
func BuildResources(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) (*Resources, error) {
	worker = worker.DeepCopy()

	image, err := resolveImage(worker.Spec.Image)
	if err != nil {
		return nil, err
	}
	if err = checkReservedNames(&worker.Spec); err != nil {
		return nil, err
	}
	podDisruptionBudget, err := newPodDisruptionBudget(objectMeta(worker), selectorLabels(worker), worker.Spec.PodDisruptionBudget)
	if err != nil {
		return nil, err
	}
	r := &Resources{
		StatefulSet:         newStatefulSet(objectMeta(worker), selectorLabels(worker), newPodTemplate(worker, image), worker.Name, &worker.Spec.DatadogBYOCClusterStatefulComponentSpec),
		PodDisruptionBudget: podDisruptionBudget,
	}
	if autoscaling := worker.Spec.Autoscaling; autoscaling != nil {
		if r.HPA, err = newHPA(objectMeta(worker), autoscaling); err != nil {
			return nil, err
		}
	}
	if ports := servicePorts(worker.Spec.Ports); len(ports) > 0 {
		r.Service = newService(objectMeta(worker), selectorLabels(worker), ports, serviceType(worker))
	}

	metadata := obsoleteObjectMeta(worker)
	if r.Service == nil {
		r.obsoleteObjects = append(r.obsoleteObjects, &corev1.Service{ObjectMeta: metadata})
	}
	switch name, managed := serviceAccountName(worker); {
	case managed:
		r.ServiceAccount = newServiceAccount(worker)
	case name != worker.Name:
		// Keep the ServiceAccount when the user adopts the one previously created by the controller.
		r.obsoleteObjects = append(r.obsoleteObjects, &corev1.ServiceAccount{ObjectMeta: metadata})
	}
	if r.HPA == nil {
		r.obsoleteObjects = append(r.obsoleteObjects, &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metadata})
	}
	if r.PodDisruptionBudget == nil {
		r.obsoleteObjects = append(r.obsoleteObjects, &policyv1.PodDisruptionBudget{ObjectMeta: metadata})
	}
	return r, nil
}

// checkReservedNames rejects user-provided names that collide with the names the Worker workload uses.
func checkReservedNames(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) error {
	if slices.ContainsFunc(spec.Ports, func(port datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort) bool {
		return port.Name == workerAPIPortName
	}) {
		return fmt.Errorf("worker port name %q is reserved", workerAPIPortName)
	}
	if slices.ContainsFunc(spec.InitContainers, func(container corev1.Container) bool { return container.Name == workerContainerName }) {
		return fmt.Errorf("worker init container name %q is reserved", workerContainerName)
	}
	if slices.ContainsFunc(spec.Volumes, func(volume corev1.Volume) bool { return volume.Name == dataVolumeName }) {
		return fmt.Errorf("worker volume name %q is reserved", dataVolumeName)
	}
	return nil
}
