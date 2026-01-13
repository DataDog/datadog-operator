// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/controller/openapi/builder"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
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
		// Backward-compatibility: if the installed CRD schema is older than the operator code,
		// managedfields apply fails with "field not declared in schema".
		//
		// This merge is used to compute a desired object (not directly applied); when
		// the apiserver CRD is outdated, these unknown fields are pruned anyway.
		// So we can safely strip them from the merge inputs and retry.
		paths := extractMissingSchemaPaths(err)
		if len(paths) == 0 {
			return nil, fmt.Errorf("failed to apply merge: %w", err)
		}

		orig := original.DeepCopyObject()
		mod := modified.DeepCopyObject()

		for _, p := range paths {
			_ = stripDottedFieldPath(orig, p)
			_ = stripDottedFieldPath(mod, p)
		}

		newObj, retryErr := r.fieldManager.Apply(orig, mod, defaultOperatorManager, true)
		if retryErr != nil {
			// If we still fail, return the original error to avoid masking other issues.
			return nil, fmt.Errorf("failed to apply merge: %w", err)
		}

		r.log.V(1).Info("SSA merge failed due to missing CRD schema fields; retried after stripping unknown fields", "paths", paths)
		return newObj, nil
	}

	return newObj, nil
}

var missingSchemaPathRe = regexp.MustCompile(`(\.[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)+): field not declared in schema`)

// extractMissingSchemaPaths finds dotted field paths from managedfields errors, e.g.
// ".spec.features.cws.enforcement: field not declared in schema".
func extractMissingSchemaPaths(err error) []string {
	if err == nil {
		return nil
	}
	matches := missingSchemaPathRe.FindAllStringSubmatch(err.Error(), -1)
	if len(matches) == 0 {
		return nil
	}
	seen := sets.NewString()
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		p := strings.TrimSpace(m[1])
		if p == "" || seen.Has(p) {
			continue
		}
		seen.Insert(p)
		out = append(out, p)
	}
	return out
}

// stripDottedFieldPath removes a dotted path (starting with ".") from a runtime.Object by converting it
// to unstructured and removing the nested field. Best-effort; returns an error only for invalid paths
// or conversion failures.
func stripDottedFieldPath(obj runtime.Object, dottedPath string) error {
	if obj == nil {
		return nil
	}
	p := strings.TrimSpace(dottedPath)
	if p == "" {
		return nil
	}
	if !strings.HasPrefix(p, ".") {
		return field.Invalid(field.NewPath("path"), dottedPath, "expected dotted path starting with '.'")
	}
	parts := strings.Split(strings.TrimPrefix(p, "."), ".")
	if len(parts) == 0 {
		return nil
	}
	if slices.Contains(parts, "") {
		return field.Invalid(field.NewPath("path"), dottedPath, "invalid empty path segment")
	}

	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}
	unstructured.RemoveNestedField(m, parts...)
	return runtime.DefaultUnstructuredConverter.FromUnstructured(m, obj)
}
