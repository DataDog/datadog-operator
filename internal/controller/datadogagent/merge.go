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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/structured-merge-diff/v6/typed"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	defaultOperatorManager = "datadog-operator"
	ddaiCRDName            = "datadogagentinternals.datadoghq.com"
)

// crdFieldManager wraps managedfields.FieldManager alongside the CRD's TypeConverter
// so in-memory merges without managed-field pruning can execute TypedValue.Merge directly
// without paying for unused ManagedFields diffing, fieldset construction, or encoding.
type crdFieldManager struct {
	fieldManager  *managedfields.FieldManager
	typeConverter managedfields.TypeConverter
	scheme        *runtime.Scheme
	gvk           schema.GroupVersionKind
}

func newFieldManager(ctx context.Context, client client.Client, scheme *runtime.Scheme, objGVK schema.GroupVersionKind) (*crdFieldManager, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := client.Get(ctx, types.NamespacedName{Name: ddaiCRDName}, crd); err != nil {
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

	return &crdFieldManager{
		fieldManager:  fm,
		typeConverter: typeConverter,
		scheme:        scheme,
		gvk:           objGVK,
	}, nil
}

// hasPopulatedManagedFields reports whether obj carries any ManagedFields entry
// with non-empty FieldsV1 (the only case where FieldManager.Apply's prune step
// can remove fields during merge).
func hasPopulatedManagedFields(obj runtime.Object) bool {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return true
	}
	for _, entry := range accessor.GetManagedFields() {
		if entry.FieldsV1 != nil && len(entry.FieldsV1.Raw) > 0 {
			return true
		}
	}
	return false
}

func (fm *crdFieldManager) objectToTypedLive(obj runtime.Object) (*typed.TypedValue, error) {
	versioned, err := fm.scheme.ConvertToVersion(obj, fm.gvk.GroupVersion())
	if err != nil {
		return nil, err
	}
	return fm.typeConverter.ObjectToTyped(versioned, typed.AllowDuplicates)
}

// applyDirect performs the schema-aware structured merge directly via TypedValue.Merge,
// bypassing FieldManager's unused ManagedFields tracking, Compare, ToFieldSet, and encoding.
// If preTypedOriginal is non-nil, it is reused instead of re-converting original.
func (fm *crdFieldManager) applyDirect(original, modified runtime.Object, preTypedOriginal *typed.TypedValue) (runtime.Object, error) {
	if preTypedOriginal == nil && hasPopulatedManagedFields(original) {
		return fm.fieldManager.Apply(original, modified, defaultOperatorManager, true)
	}

	if patchVersion := modified.GetObjectKind().GroupVersionKind().GroupVersion(); patchVersion != fm.gvk.GroupVersion() {
		return nil, apierrors.NewBadRequest(
			fmt.Sprintf("Incorrect version specified in apply patch. Specified patch version: %s, expected: %s",
				patchVersion, fm.gvk.GroupVersion()))
	}

	patchMeta, err := meta.Accessor(modified)
	if err != nil {
		return nil, fmt.Errorf("couldn't get accessor: %w", err)
	}
	if patchMeta.GetManagedFields() != nil {
		return nil, apierrors.NewBadRequest("metadata.managedFields must be nil")
	}

	patchTyped, err := fm.typeConverter.ObjectToTyped(modified)
	if err != nil {
		return nil, fmt.Errorf("failed to create typed patch object: %w", err)
	}

	liveTyped := preTypedOriginal
	if liveTyped == nil {
		liveTyped, err = fm.objectToTypedLive(original)
		if err != nil {
			return nil, fmt.Errorf("failed to create typed live object: %w", err)
		}
	}

	mergedTyped, err := liveTyped.Merge(patchTyped)
	if err != nil {
		return nil, fmt.Errorf("failed to merge config: %w", err)
	}

	newObj, err := fm.typeConverter.TypedToObject(mergedTyped)
	if err != nil {
		return nil, fmt.Errorf("failed to convert new typed object to object: %w", err)
	}

	newObjVersioned, err := fm.scheme.ConvertToVersion(newObj, fm.gvk.GroupVersion())
	if err != nil {
		return nil, fmt.Errorf("failed to convert new object to proper version: %w", err)
	}
	fm.scheme.Default(newObjVersioned)
	return newObjVersioned, nil
}

// ssaMergeCRD merges two custom resource objects using server-side apply without applying the result in k8s
func (r *Reconciler) ssaMergeCRD(ctx context.Context, original, modified runtime.Object) (runtime.Object, error) {
	return r.ssaMergeCRDWithTypedOriginal(ctx, original, modified, nil)
}

func (r *Reconciler) ssaMergeCRDWithTypedOriginal(ctx context.Context, original, modified runtime.Object, preTypedOriginal *typed.TypedValue) (runtime.Object, error) {
	newObj, err := r.fieldManager.applyDirect(original, modified, preTypedOriginal)
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

		var retryErr error
		newObj, retryErr = r.fieldManager.applyDirect(orig, mod, nil)
		if retryErr != nil {
			// If we still fail, return the original error to avoid masking other issues.
			return nil, fmt.Errorf("failed to apply merge: %w", err)
		}

		originalKV := objectIdentityKV("original", original)
		modifiedKV := objectIdentityKV("modified", modified)
		kv := make([]any, 0, 2+len(originalKV)+len(modifiedKV))
		kv = append(kv, "paths", paths)
		kv = append(kv, originalKV...)
		kv = append(kv, modifiedKV...)
		ctrl.LoggerFrom(ctx).V(1).Info("SSA merge failed due to missing CRD schema fields; retried after stripping unknown fields", kv...)
		return newObj, nil
	}

	return newObj, nil
}

func objectIdentityKV(prefix string, obj runtime.Object) []any {
	if obj == nil {
		return []any{prefix + "_nil", true}
	}

	kv := []any{prefix + "_gvk", obj.GetObjectKind().GroupVersionKind().String()}

	accessor, err := meta.Accessor(obj)
	if err != nil {
		return append(kv, prefix+"_metaError", err.Error())
	}

	kv = append(kv,
		prefix+"_namespace", accessor.GetNamespace(),
		prefix+"_name", accessor.GetName(),
		prefix+"_uid", string(accessor.GetUID()),
	)

	labels := accessor.GetLabels()
	if labels != nil {
		if val, ok := labels[constants.ProfileLabelKey]; ok && val != "" {
			kv = append(kv, prefix+"_profile", val)
		}
		if val, ok := labels[apicommon.DatadogAgentNameLabelKey]; ok && val != "" {
			kv = append(kv, prefix+"_datadogAgent", val)
		}
	}

	return kv
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
