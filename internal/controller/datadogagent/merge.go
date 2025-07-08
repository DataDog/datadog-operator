// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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

// ssaMergeCRD merges two custom resource objects using server-side apply without applying the result in k8s
func (r *Reconciler) ssaMergeCRD(original, modified runtime.Object) (runtime.Object, error) {
	// Server side apply
	// `datadog-operator` is the manager for managed fields
	// Set force=true to override conflicts
	newObj, err := r.fieldManager.Apply(original, modified, defaultOperatorManager, true)
	if err != nil {
		return nil, fmt.Errorf("failed to apply merge: %w", err)
	}

	return newObj, nil
}
