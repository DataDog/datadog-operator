// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissioncontroller

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getACServiceName return the default service name for the admission controller based on the DatadogAgent name
func getACServiceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), admissionControllerServiceNameSuffix)
}

// getACWebhookName return the default webhook name for the admission controller based on the DatadogAgent name
func getACWebhookName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), admissionControllerWebhookNameSuffix)
}
