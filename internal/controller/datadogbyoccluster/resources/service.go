// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// RESTEndpoint returns the URL of the Pomsky REST API, which every searcher serves behind its Service.
func RESTEndpoint(cluster *datadoghqv1alpha1.DatadogBYOCCluster) string {
	return fmt.Sprintf("http://%s.%s.svc:%d", ComponentResourceName(cluster.Name, SearcherComponentName), cluster.Namespace, restPort)
}

func newService(metadata metav1.ObjectMeta, selector map[string]string, ports []corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metadata,
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selector,
			Ports:    ports,
		},
	}
}
