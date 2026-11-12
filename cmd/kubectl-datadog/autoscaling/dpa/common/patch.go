// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// patchAnnotation sets or removes the annotation.
func patchAnnotation(ctx context.Context, c client.Client, obj client.Object, key string, value AnnotationValue) error {
	var v any // null removes the key
	if !value.Remove {
		v = value.Value
	}
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{key: v},
		},
	})
	if err != nil {
		return err
	}
	return c.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch))
}
