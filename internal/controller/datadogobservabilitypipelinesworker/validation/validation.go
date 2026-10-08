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
	"cmp"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocvalidation "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/validation"
	workerresources "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/resources"
)

// ValidateWorkerSpec checks image, component and reserved name rules after defaults are applied.
// Single-field constraints, such as minimum lengths, remain in the CRD schema.
func ValidateWorkerSpec(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) field.ErrorList {
	path := field.NewPath("spec")
	errs := byocvalidation.ValidateStatefulComponent(&spec.DatadogBYOCClusterStatefulComponentSpec, path)
	errs = append(errs, validatePorts(spec.Ports, path.Child("ports"))...)
	errs = append(errs, validateReservedNames(spec, path)...)
	imagePath := path.Child("image")
	if spec.Image == nil {
		return append(errs, field.Required(imagePath, "image must be specified"))
	}
	if spec.Image.Repository == nil {
		errs = append(errs, field.Required(imagePath.Child("repository"), "repository must be specified"))
	}
	if (spec.Image.Tag == nil) == (spec.Image.Digest == nil) {
		errs = append(errs, field.Invalid(imagePath, nil, "exactly one of tag or digest must be specified"))
	} else if spec.Image.Digest != nil && *spec.Image.Digest == "" {
		errs = append(errs, field.Required(imagePath.Child("digest"), "digest must be non-empty"))
	}
	return errs
}

// validatePorts rejects ports that collide with the Worker API port or with each other,
// since the Worker cannot bind two listeners to the same address.
func validatePorts(ports []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort, fieldPath *field.Path) field.ErrorList {
	type portKey struct {
		port     int32
		protocol corev1.Protocol
	}
	var errs field.ErrorList
	seen := map[portKey]bool{}
	for i, port := range ports {
		if port.Name == workerresources.APIPortName {
			errs = append(errs, field.Invalid(fieldPath.Index(i).Child("name"), port.Name, "is reserved for the Worker API port"))
		}
		key := portKey{port: port.Port, protocol: cmp.Or(port.Protocol, corev1.ProtocolTCP)}
		if key == (portKey{port: workerresources.APIPort, protocol: corev1.ProtocolTCP}) {
			errs = append(errs, field.Invalid(fieldPath.Index(i).Child("port"), port.Port, "is reserved for the Worker API port"))
		}
		if seen[key] {
			errs = append(errs, field.Invalid(fieldPath.Index(i).Child("port"), port.Port, "duplicates the port and protocol of another port"))
		}
		seen[key] = true
	}
	return errs
}

func validateReservedNames(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec, fieldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	for i, container := range spec.InitContainers {
		if container.Name == workerresources.ContainerName {
			errs = append(errs, field.Invalid(fieldPath.Child("initContainers").Index(i).Child("name"), container.Name, "is reserved for the Worker container"))
		}
	}
	for i, volume := range spec.Volumes {
		if volume.Name == workerresources.DataVolumeName {
			errs = append(errs, field.Invalid(fieldPath.Child("volumes").Index(i).Child("name"), volume.Name, "is reserved for a built-in volume"))
		}
	}
	for i, volumeMount := range spec.VolumeMounts {
		if path.Clean(volumeMount.MountPath) == workerresources.DataDirectory {
			errs = append(errs, field.Invalid(fieldPath.Child("volumeMounts").Index(i).Child("mountPath"), volumeMount.MountPath, "is reserved for a built-in volume mount"))
		}
	}
	return errs
}
