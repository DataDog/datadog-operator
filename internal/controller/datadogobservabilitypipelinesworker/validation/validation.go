// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package validation enforces Worker cross-field rules at reconciliation time.
// CEL/XValidation requires Kubernetes 1.25+, but the Operator's minimum supported
// Kubernetes version is not being raised. OperatorHub/Red Hat installations install
// all CRDs in the OLM bundle; they cannot omit the Worker CRD on older clusters.
// Keep these rules in the reconciler so the Worker does not impose a newer
// Kubernetes requirement on every Operator installation.
// See https://github.com/DataDog/datadog-operator/pull/3501#discussion_r4080491818.
package validation

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocvalidation "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/validation"
)

// ValidateWorkerSpec checks image and component rules before defaults are applied.
// Single-field constraints, such as minimum lengths, remain in the CRD schema.
func ValidateWorkerSpec(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) field.ErrorList {
	path := field.NewPath("spec")
	errs := byocvalidation.ValidateStatefulComponent(&spec.DatadogBYOCClusterStatefulComponentSpec, path)
	imagePath := path.Child("image")
	if spec.Image == nil {
		return append(errs, field.Required(imagePath, "image must be specified"))
	}
	if spec.Image.Repository == nil {
		errs = append(errs, field.Required(imagePath.Child("repository"), "repository must be specified"))
	}
	if (spec.Image.Tag == nil) == (spec.Image.Digest == nil) {
		errs = append(errs, field.Invalid(imagePath, nil, "exactly one of tag or digest must be specified"))
	}
	return errs
}
