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

func newHeadlessService(cluster *datadoghqv1alpha1.DatadogBYOCCluster) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        headlessServiceName(cluster.Name),
			Namespace:   cluster.Namespace,
			Labels:      labels(cluster),
			Annotations: annotations(cluster),
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Selector:                 instanceSelectorLabels(cluster),
			Ports: []corev1.ServicePort{
				{Name: "tcp-http", Port: restPort, Protocol: corev1.ProtocolTCP},
				{Name: "tcp-grpc", Port: grpcPort, Protocol: corev1.ProtocolTCP},
				{Name: "udp", Port: gossipPort, Protocol: corev1.ProtocolUDP},
				{Name: "tcp-cloudprem", Port: cloudpremPort, Protocol: corev1.ProtocolTCP},
				{Name: "tcp-health", Port: healthPort, Protocol: corev1.ProtocolTCP},
			},
		},
	}
}

func headlessServiceName(clusterName string) string {
	return clusterName + "-headless"
}
