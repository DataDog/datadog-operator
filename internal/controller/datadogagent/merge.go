// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"strings"

	apiextensionshelpers "k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apiextensions-apiserver/pkg/controller/openapi/builder"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultOperatorManager = "datadog-operator"
	ddaiCRDName            = "datadogagentinternals.datadoghq.com"
)

func newFieldManager(client client.Client, scheme *runtime.Scheme, objGVK schema.GroupVersionKind) (*managedfields.FieldManager, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: ddaiCRDName}, crd); err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %w", ddaiCRDName, err)
	}

	s, err := builder.BuildOpenAPIV3(crd, objGVK.Version, builder.Options{})
	if err != nil {
		return nil, err
	}

	typeConverter, err := managedfields.NewTypeConverter(s.Components.Schemas, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create type converter: %w", err)
	}

	fm, err := managedfields.NewDefaultCRDFieldManager(
		typeConverter,         // typeConverter
		scheme,                // objectConverter
		scheme,                // objectDefaulter
		scheme,                // objectCreater
		objGVK,                // kind
		objGVK.GroupVersion(), // hub
		"",                    // subresource
		nil,                   // resetFields
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create field manager: %w", err)
	}

	return fm, nil
}

// buildStructuralSchema constructs a structural schema from the CRD for the given version
func buildStructuralSchema(crd *apiextensionsv1.CustomResourceDefinition, version string) (*structuralschema.Structural, error) {
	val, err := apiextensionshelpers.GetSchemaForVersion(crd, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema for version %s: %w", version, err)
	}
	if val == nil || val.OpenAPIV3Schema == nil {
		return nil, fmt.Errorf("schema not found for version %s", version)
	}

	// Convert v1.CustomResourceValidation to internal apiextensions.CustomResourceValidation
	internalValidation := &apiextensions.CustomResourceValidation{}
	if convertErr := apiextensionsv1.Convert_v1_CustomResourceValidation_To_apiextensions_CustomResourceValidation(
		val, internalValidation, nil); convertErr != nil {
		return nil, fmt.Errorf("failed converting CRD validation to internal version: %w", convertErr)
	}

	// Build the structural schema from the internal representation
	structural, convertErr := structuralschema.NewStructural(internalValidation.OpenAPIV3Schema)
	if convertErr != nil {
		return nil, fmt.Errorf("failed to convert schema to structural: %w", convertErr)
	}

	return structural, nil
}

// pruneObjectsAgainstSchema prunes unknown fields from runtime objects based on the CRD schema
// This is used for backward compatibility when the operator code is newer than the installed CRD
func pruneObjectsAgainstSchema(original, modified runtime.Object, crd *apiextensionsv1.CustomResourceDefinition) (runtime.Object, runtime.Object, error) {
	if original == nil || modified == nil {
		return nil, nil, fmt.Errorf("original and modified objects must not be nil")
	}

	gvk := original.GetObjectKind().GroupVersionKind()
	structural, err := buildStructuralSchema(crd, gvk.Version)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build structural schema: %w", err)
	}

	// Prune unknown fields from both objects
	orig := original.DeepCopyObject()
	mod := modified.DeepCopyObject()

	origUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(orig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert original to unstructured: %w", err)
	}
	modUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(mod)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert modified to unstructured: %w", err)
	}

	pruning.Prune(origUnstructured, structural, true)
	pruning.Prune(modUnstructured, structural, true)

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(origUnstructured, orig); err != nil {
		return nil, nil, fmt.Errorf("failed to convert original from unstructured: %w", err)
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(modUnstructured, mod); err != nil {
		return nil, nil, fmt.Errorf("failed to convert modified from unstructured: %w", err)
	}

	return orig, mod, nil
}

// ssaMergeCRD merges two custom resource objects using server-side apply without applying the result in k8s
func (r *Reconciler) ssaMergeCRD(original, modified runtime.Object) (runtime.Object, error) {
	// Server side apply
	// `datadog-operator` is the manager for managed fields
	// Set force=true to override conflicts
	newObj, err := r.fieldManager.Apply(original, modified, defaultOperatorManager, true)
	if err != nil {
		// Backward-compatibility: if the installed CRD schema is older than the operator code,
		// managedfields apply fails with "field not declared in schema".
		//
		// This merge is used to compute a desired object (not directly applied); when
		// the apiserver CRD is outdated, these unknown fields are pruned anyway.
		// So we can safely prune them from the merge inputs and retry.
		if !isSchemaError(err) {
			return nil, fmt.Errorf("failed to apply merge: %w", err)
		}

		r.log.V(1).Info("SSA merge failed due to schema mismatch; retrying after pruning unknown fields")

		crd := &apiextensionsv1.CustomResourceDefinition{}
		if getErr := r.client.Get(context.TODO(), types.NamespacedName{Name: ddaiCRDName}, crd); getErr != nil {
			return nil, fmt.Errorf("failed to apply merge, could not fetch CRD: %w", err)
		}

		// Prune unknown fields from both objects based on CRD schema
		orig, mod, pruneErr := pruneObjectsAgainstSchema(original, modified, crd)
		if pruneErr != nil {
			return nil, fmt.Errorf("failed to prune objects: %w", pruneErr)
		}

		// Retry merge with pruned objects
		retryObj, retryErr := r.fieldManager.Apply(orig, mod, defaultOperatorManager, true)
		if retryErr != nil {
			// If we still fail, return both errors to help diagnose the issue
			return nil, fmt.Errorf("failed to retry apply merge; original error: %w, retry error: %w", err, retryErr)
		}

		return retryObj, nil
	}

	return newObj, nil
}

// isSchemaError checks if the error is related to schema validation failures
func isSchemaError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "field not declared in schema")
}
