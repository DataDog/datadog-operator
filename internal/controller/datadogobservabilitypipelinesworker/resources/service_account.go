// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// serviceAccountName returns the ServiceAccount used by the Worker and whether the controller manages it.
func serviceAccountName(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) (string, bool) {
	if identity := worker.Spec.Identity; identity != nil && identity.ServiceAccountName != nil {
		return *identity.ServiceAccountName, false
	}
	return worker.Name, true
}

func newServiceAccount(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta:                   objectMeta(worker),
		AutomountServiceAccountToken: new(false),
	}
}
