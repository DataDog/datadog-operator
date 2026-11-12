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

// serviceAccountName returns the ServiceAccount used by the workloads and whether the controller manages it.
func serviceAccountName(cluster *datadoghqv1alpha1.DatadogBYOCCluster) (string, bool) {
	if identity := cluster.Spec.Identity; identity != nil && identity.ServiceAccountName != nil {
		return *identity.ServiceAccountName, false
	}
	return cluster.Name, true
}

func newServiceAccount(cluster *datadoghqv1alpha1.DatadogBYOCCluster) *corev1.ServiceAccount {
	var irsaAnnotations map[string]string
	if provider := cluster.Spec.Provider; provider != nil && provider.AWS != nil && provider.AWS.IRSARoleARN != nil && *provider.AWS.IRSARoleARN != "" {
		irsaAnnotations = map[string]string{
			"eks.amazonaws.com/role-arn":               *provider.AWS.IRSARoleARN,
			"eks.amazonaws.com/sts-regional-endpoints": "true",
		}
	}
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cluster.Name,
			Namespace:   cluster.Namespace,
			Labels:      labels(cluster),
			Annotations: annotations(cluster, irsaAnnotations),
		},
		AutomountServiceAccountToken: new(false),
	}
}
