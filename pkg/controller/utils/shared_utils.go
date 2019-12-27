// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
)

func GetAPIKeySecretName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	if dad.Spec.Credentials.APIKeyExistingSecret != "" {
		return dad.Spec.Credentials.APIKeyExistingSecret
	}
	return dad.Name
}

func GetAppKeySecretName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	if dad.Spec.Credentials.AppKeyExistingSecret != "" {
		return dad.Spec.Credentials.AppKeyExistingSecret
	}
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}
