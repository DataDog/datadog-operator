// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/controller/openapi/builder"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

const (
	defaultOperatorManager = "datadog-operator"
)

// ssaMergeCRD merges two custom resource objects using server-side apply without applying the result in k8s
func ssaMergeCRD(original, modified runtime.Object, crd *apiextensionsv1.CustomResourceDefinition, scheme *runtime.Scheme) (runtime.Object, error) {
	gvk, err := apiutil.GVKForObject(original, scheme)
	if err != nil {
		return nil, err
	}

	s, err := builder.BuildOpenAPIV3(crd, gvk.Version, builder.Options{})
	if err != nil {
		return nil, err
	}

	typeConverter, err := managedfields.NewTypeConverter(s.Components.Schemas, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create type converter: %w", err)
	}

	fm, err := managedfields.NewDefaultCRDFieldManager(
		typeConverter,      // typeConverter
		scheme,             // objectConverter
		scheme,             // objectDefaulter
		scheme,             // objectCreater
		gvk,                // kind
		gvk.GroupVersion(), // hub
		"",                 // subresource
		nil,                // resetFields
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create field manager: %w", err)
	}

	originalUnstructured, err := convertToUnstructured(original, gvk)
	if err != nil {
		return nil, err
	}

	modifiedUnstructured, err := convertToUnstructured(modified, gvk)
	if err != nil {
		return nil, err
	}

	// Server side apply
	// `datadog-operator` is the manager for managed fields
	// Set force=true to override conflicts
	newObj, err := fm.Apply(originalUnstructured, modifiedUnstructured, defaultOperatorManager, true)
	if err != nil {
		return nil, fmt.Errorf("failed to apply merge: %w", err)
	}

	return newObj, nil
}

func convertToUnstructured(obj runtime.Object, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}
	unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}
	unstructuredObj.SetGroupVersionKind(gvk)
	return unstructuredObj, nil
}
