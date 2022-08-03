// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

// PodTemplateSpec use to override a corev1.PodTemplateSpec with a 2alpha1.DatadogAgentPodTemplateOverride.
func PodTemplateSpec(manager feature.PodTemplateManagers, override *v2alpha1.DatadogAgentComponentOverride) (*corev1.PodTemplateSpec, error) {
	// TODO(operator-ga): implement OverridePodTemplate

	var errs []error
	// Loop over container
	for agentContainerName, containerOverride := range override.Containers {
		Container(agentContainerName, manager, containerOverride)
	}

	return nil, errors.NewAggregate(errs)
}
