// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func newService(metadata metav1.ObjectMeta, selector map[string]string, ports []corev1.ServicePort, serviceType corev1.ServiceType) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metadata,
		Spec: corev1.ServiceSpec{
			Type:     serviceType,
			Selector: selector,
			Ports:    ports,
		},
	}
}

func serviceType(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) corev1.ServiceType {
	if service := worker.Spec.Service; service != nil && service.Type != "" {
		return service.Type
	}
	return corev1.ServiceTypeClusterIP
}

func servicePorts(ports []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort) []corev1.ServicePort {
	result := make([]corev1.ServicePort, 0, len(ports))
	for _, port := range ports {
		result = append(result, corev1.ServicePort{
			Name:       port.Name,
			Port:       port.Port,
			Protocol:   portProtocol(port),
			TargetPort: intstr.FromInt32(port.Port),
		})
	}
	return result
}

func portProtocol(port datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort) corev1.Protocol {
	if port.Protocol == "" {
		return corev1.ProtocolTCP
	}
	return port.Protocol
}
