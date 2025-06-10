// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"fmt"

	"github.com/go-logr/logr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func getExperimentalAnnotationKey(subkey string) string {
	return fmt.Sprintf("%s/%s", ExperimentalAnnotationPrefix, subkey)
}

func getExperimentalAnnotation(ddai *v1alpha1.DatadogAgentInternal, annotationSubkey string) string {
	annotationKey := getExperimentalAnnotationKey(annotationSubkey)
	if annotationValue, ok := ddai.Annotations[annotationKey]; ok {
		return annotationValue
	}

	return ""
}

// ApplyExperimentalOverrides applies any configured experimental overrides for the the given DatadogAgent resource.
func ApplyExperimentalOverrides(logger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, manager feature.PodTemplateManagers) {
	elogger := logger.WithName("ExperimentalOverrides")
	elogger.V(2).Info("Applying experimental overrides")

	applyExperimentalImageOverrides(elogger, ddai, manager)
}
