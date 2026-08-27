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

type serviceAccountBuilder struct {
	name        string
	namespace   string
	labels      map[string]string
	annotations map[string]string
}

func newServiceAccountBuilder(cluster *datadoghqv1alpha1.DatadogBYOCCluster) serviceAccountBuilder {
	name := cluster.Name
	if identity := cluster.Spec.Identity; identity != nil && identity.ServiceAccountName != nil && *identity.ServiceAccountName != "" {
		name = *identity.ServiceAccountName
	}

	var additionalAnnotations map[string]string
	if provider := cluster.Spec.Provider; provider != nil && provider.AWS != nil && provider.AWS.IRSARoleARN != nil && *provider.AWS.IRSARoleARN != "" {
		additionalAnnotations = map[string]string{
			"eks.amazonaws.com/role-arn":               *provider.AWS.IRSARoleARN,
			"eks.amazonaws.com/sts-regional-endpoints": "true",
		}
	}
	return serviceAccountBuilder{
		name:        name,
		namespace:   cluster.Namespace,
		labels:      labels(cluster),
		annotations: annotations(cluster, additionalAnnotations),
	}
}

func (b serviceAccountBuilder) build() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.name,
			Namespace:   b.namespace,
			Labels:      b.labels,
			Annotations: b.annotations,
		},
		AutomountServiceAccountToken: new(false),
	}
}
