// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package validation enforces BYOC cross-field rules at reconciliation time.
// CEL/XValidation requires Kubernetes 1.25+, but the Operator's minimum supported
// Kubernetes version is not being raised. OperatorHub/Red Hat installations install
// all CRDs in the OLM bundle; they cannot omit the BYOC CRD on older clusters.
// Keep these rules in the reconciler so BYOC does not impose a newer Kubernetes
// requirement on every Operator installation.
// See https://github.com/DataDog/datadog-operator/pull/3501#discussion_r4080491818.
package validation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// ValidateClusterSpec checks the cross-field rules after defaults and before image resolution.
// Validating the defaulted spec catches conflicts between explicit and default values.
// Required fields and single-field constraints are enforced by the CRD schema.
func ValidateClusterSpec(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) field.ErrorList {
	path := field.NewPath("spec")
	var errs field.ErrorList
	if spec.Release == nil && spec.ImageOverrides == nil {
		errs = append(errs, field.Required(path, "release or imageOverrides must be specified"))
	}
	if release := spec.Release; release != nil && (release.Tag == nil) == (release.Digest == nil) {
		errs = append(errs, field.Invalid(path.Child("release"), nil, "exactly one of tag or digest must be specified"))
	}
	if overrides := spec.ImageOverrides; overrides != nil {
		// Without a release, neither image can inherit a repository or version.
		requireComplete := spec.Release == nil
		errs = append(errs, validateImageOverride(overrides.BYOC, requireComplete, path.Child("imageOverrides", "byoc"))...)
		errs = append(errs, validateImageOverride(overrides.ObservabilityPipelinesWorker, requireComplete, path.Child("imageOverrides", "observabilityPipelinesWorker"))...)
	}
	if spec.Provider == nil || spec.Provider.AWS == nil {
		errs = append(errs, field.Required(path.Child("provider", "aws"), "at least one provider must be specified"))
	}
	// Validate the global setting even if every component overrides it.
	errs = append(errs, validatePodDisruptionBudget(spec.Global.PodDisruptionBudget, path.Child("global", "podDisruptionBudget"))...)
	if components := spec.Components; components != nil {
		componentPath := path.Child("components")
		errs = append(errs, validateQuickwitStatefulComponent(components.Indexer, componentPath.Child("indexer"))...)
		errs = append(errs, validateQuickwitStatefulComponent(components.Searcher, componentPath.Child("searcher"))...)
		for i := range components.Pipelines {
			errs = append(errs, ValidateStatefulComponent(&components.Pipelines[i].DatadogBYOCClusterStatefulComponentSpec, componentPath.Child("pipelines").Index(i))...)
		}
		if components.Metastore != nil {
			errs = append(errs, validateComponent(&components.Metastore.DatadogBYOCClusterComponentSpec, componentPath.Child("metastore"))...)
		}
		if components.ReadOnlyMetastore != nil {
			errs = append(errs, validateComponent(&components.ReadOnlyMetastore.DatadogBYOCClusterComponentSpec, componentPath.Child("readOnlyMetastore"))...)
		}
		errs = append(errs, validateComponent(components.ControlPlane, componentPath.Child("controlPlane"))...)
		errs = append(errs, validateComponent(components.Compactor, componentPath.Child("compactor"))...)
		errs = append(errs, validateComponent(components.Janitor, componentPath.Child("janitor"))...)
	}
	return errs
}

func validateImageOverride(image *datadoghqv1alpha1.DatadogBYOCImageSpec, requireComplete bool, path *field.Path) field.ErrorList {
	if image == nil {
		if requireComplete {
			return field.ErrorList{field.Required(path, "image must be fully specified when release is omitted")}
		}
		return nil
	}
	var errs field.ErrorList
	if image.Tag != nil && image.Digest != nil {
		errs = append(errs, field.Forbidden(path, "tag and digest are mutually exclusive"))
	}
	if requireComplete {
		if image.Repository == nil || *image.Repository == "" {
			errs = append(errs, field.Required(path.Child("repository"), "repository must be non-empty when release is omitted"))
		}
		if image.Tag == nil && image.Digest == nil {
			errs = append(errs, field.Required(path, "tag or digest must be specified when release is omitted"))
		}
		if image.Tag != nil && *image.Tag == "" {
			errs = append(errs, field.Required(path.Child("tag"), "tag must be non-empty when release is omitted"))
		}
		if image.Digest != nil && *image.Digest == "" {
			errs = append(errs, field.Required(path.Child("digest"), "digest must be non-empty when release is omitted"))
		}
	}
	return errs
}

// ValidateStatefulComponent is shared by BYOC components and standalone Workers.
func ValidateStatefulComponent(component *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec, path *field.Path) field.ErrorList {
	if component == nil {
		return nil
	}
	errs := validateComponent(&component.DatadogBYOCClusterComponentSpec, path)
	errs = append(errs, validateAutoscaling(component.Autoscaling, path.Child("autoscaling"))...)
	if storage := component.Storage; storage != nil && (storage.EmptyDir == nil) == (storage.VolumeClaimTemplate == nil) {
		errs = append(errs, field.Invalid(path.Child("storage"), nil, "exactly one storage type must be specified"))
	}
	return errs
}

// validateQuickwitStatefulComponent also requires the memory limit used to size the Quickwit node configuration.
func validateQuickwitStatefulComponent(component *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec, path *field.Path) field.ErrorList {
	if component == nil {
		return nil
	}
	errs := ValidateStatefulComponent(component, path)
	if component.Resources != nil {
		if _, ok := component.Resources.Limits[corev1.ResourceMemory]; !ok {
			errs = append(errs, field.Required(path.Child("resources", "limits", "memory"), "resources.limits.memory must be specified when resources is set"))
		}
	}
	return errs
}

func validateAutoscaling(autoscaling *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec, path *field.Path) field.ErrorList {
	if autoscaling == nil {
		return nil
	}
	var errs field.ErrorList
	if autoscaling.MinReplicas != nil && *autoscaling.MinReplicas < 1 {
		errs = append(errs, field.Invalid(path.Child("minReplicas"), *autoscaling.MinReplicas, "must be greater than or equal to 1"))
	}
	if autoscaling.MaxReplicas != nil && *autoscaling.MaxReplicas < 1 {
		errs = append(errs, field.Invalid(path.Child("maxReplicas"), *autoscaling.MaxReplicas, "must be greater than or equal to 1"))
	}
	if autoscaling.MinReplicas != nil && autoscaling.MaxReplicas != nil && *autoscaling.MinReplicas > *autoscaling.MaxReplicas {
		errs = append(errs, field.Invalid(path.Child("maxReplicas"), *autoscaling.MaxReplicas, fmt.Sprintf("must be greater than or equal to minReplicas (%d)", *autoscaling.MinReplicas)))
	}
	return errs
}

func validateComponent(component *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec, path *field.Path) field.ErrorList {
	if component == nil {
		return nil
	}
	return validatePodDisruptionBudget(component.PodDisruptionBudget, path.Child("podDisruptionBudget"))
}

func validatePodDisruptionBudget(budget *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec, path *field.Path) field.ErrorList {
	if budget != nil && budget.MinAvailable != nil && budget.MaxUnavailable != nil {
		return field.ErrorList{field.Forbidden(path, "minAvailable and maxUnavailable are mutually exclusive")}
	}
	return nil
}
