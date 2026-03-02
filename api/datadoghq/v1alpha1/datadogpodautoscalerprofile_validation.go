// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"

	utilserrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
)

func IsValidDatadogPodAutoscalerProfile(spec *DatadogPodAutoscalerProfileSpec) error {
	var errs []error

	errs = append(errs, validateProfileTemplate(&spec.Template)...)

	return utilserrors.NewAggregate(errs)
}

func validateProfileTemplate(template *common.DatadogPodAutoscalerTemplate) []error {
	var errs []error

	if len(template.Objectives) == 0 {
		errs = append(errs, fmt.Errorf("spec.template.objectives must contain at least one objective"))
	}

	for i, obj := range template.Objectives {
		errs = append(errs, validateObjective(i, &obj)...)
	}

	if template.Constraints != nil {
		errs = append(errs, validateConstraints(template.Constraints)...)
	}

	if template.ApplyPolicy != nil {
		errs = append(errs, validateApplyPolicy(template.ApplyPolicy)...)
	}

	return errs
}

func validateObjective(index int, obj *common.DatadogPodAutoscalerObjective) []error {
	var errs []error
	prefix := fmt.Sprintf("spec.template.objectives[%d]", index)

	switch obj.Type {
	case common.DatadogPodAutoscalerPodResourceObjectiveType:
		if obj.PodResource == nil {
			errs = append(errs, fmt.Errorf("%s.podResource must be defined when type is PodResource", prefix))
		}
	case common.DatadogPodAutoscalerContainerResourceObjectiveType:
		if obj.ContainerResource == nil {
			errs = append(errs, fmt.Errorf("%s.containerResource must be defined when type is ContainerResource", prefix))
		}
	case common.DatadogPodAutoscalerCustomQueryObjectiveType:
		if obj.CustomQuery == nil {
			errs = append(errs, fmt.Errorf("%s.customQuery must be defined when type is CustomQuery", prefix))
		}
	default:
		errs = append(errs, fmt.Errorf("%s.type must be one of PodResource, ContainerResource, CustomQuery", prefix))
	}

	return errs
}

func validateConstraints(constraints *common.DatadogPodAutoscalerConstraints) []error {
	var errs []error

	if constraints.MinReplicas != nil && constraints.MaxReplicas != nil {
		if *constraints.MinReplicas > *constraints.MaxReplicas {
			errs = append(errs, fmt.Errorf("spec.template.constraints.minReplicas must be less than or equal to maxReplicas"))
		}
	}

	return errs
}

func validateApplyPolicy(policy *common.DatadogPodAutoscalerApplyPolicy) []error {
	var errs []error

	switch policy.Mode {
	case common.DatadogPodAutoscalerApplyModeV2Apply,
		common.DatadogPodAutoscalerApplyModeV2Preview:
	default:
		errs = append(errs, fmt.Errorf("spec.template.applyPolicy.mode must be one of Apply, Preview"))
	}

	return errs
}
