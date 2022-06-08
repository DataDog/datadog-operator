// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetDefaultCredentialsSecretName returns the default name for credentials secret
func GetDefaultCredentialsSecretName(dda metav1.Object) string {
	return dda.GetName()
}

// GetDefaultDCATokenSecretName returns the default name for cluster-agent secret
func GetDefaultDCATokenSecretName(dda metav1.Object) string {
	return fmt.Sprintf("%s-token", dda.GetName())
}
