// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	v1 "k8s.io/api/apps/v1"
)

// Deployment overrides a v1.Deployment according to the given override options
func Deployment(deployment *v1.Deployment, override *v2alpha1.DatadogAgentComponentOverride) {
	if override.Replicas != nil {
		deployment.Spec.Replicas = override.Replicas
	}

	if override.Name != nil {
		deployment.Name = *override.Name
	}
}
