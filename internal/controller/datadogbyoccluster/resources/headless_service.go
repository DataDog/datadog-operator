// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type headlessServiceBuilder struct {
	name        string
	namespace   string
	labels      map[string]string
	annotations map[string]string
	selector    map[string]string
}

func newHeadlessServiceBuilder(cluster *datadoghqv1alpha1.DatadogBYOCCluster) headlessServiceBuilder {
	return headlessServiceBuilder{
		name:        headlessServiceName(cluster.Name),
		namespace:   cluster.Namespace,
		labels:      labels(cluster),
		annotations: annotations(cluster),
		selector: map[string]string{
			"app.kubernetes.io/name":     appName,
			"app.kubernetes.io/instance": cluster.Name,
		},
	}
}

func (b headlessServiceBuilder) build() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.name,
			Namespace:   b.namespace,
			Labels:      b.labels,
			Annotations: b.annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Selector:                 b.selector,
			Ports: []corev1.ServicePort{
				{Name: "tcp-http", Port: 7280, Protocol: corev1.ProtocolTCP},
				{Name: "tcp-grpc", Port: 7281, Protocol: corev1.ProtocolTCP},
				{Name: "udp", Port: 7282, Protocol: corev1.ProtocolUDP},
				{Name: "tcp-cloudprem", Port: 7283, Protocol: corev1.ProtocolTCP},
				{Name: "tcp-health", Port: 7284, Protocol: corev1.ProtocolTCP},
			},
		},
	}
}

func headlessServiceName(clusterName string) string {
	return clusterName + "-headless"
}
