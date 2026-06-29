// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// dynamicFields lists metadata sub-fields that are non-deterministic at render time
// (set by the API server or fake client) and should be stripped for stable golden files.
// Add or remove entries here to tune what appears in the output.
var dynamicFields = []string{
	"resourceVersion",
	"uid",
	"generation",
	"creationTimestamp",
	"managedFields",
}

// Serialize converts resources to YAML or JSON with consistent, sorted key ordering.
// Key ordering is guaranteed because we round-trip through map[string]interface{} first;
// encoding/json sorts map keys alphabetically, and sigs.k8s.io/yaml converts via JSON.
// Supported formats: "yaml" (default), "json".
func Serialize(objects []client.Object, scheme *runtime.Scheme, format string) ([]byte, error) {
	sorted := SortResources(objects, scheme)

	docs := make([][]byte, 0, len(sorted))
	for _, obj := range sorted {
		m, err := toSortedMap(obj, scheme)
		if err != nil {
			return nil, fmt.Errorf("converting %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
		stripDynamicFields(m)

		var doc []byte
		switch format {
		case "json":
			doc, err = json.MarshalIndent(m, "", "  ")
		default: // yaml
			doc, err = yaml.Marshal(m)
		}
		if err != nil {
			return nil, fmt.Errorf("marshaling %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
		docs = append(docs, doc)
	}

	return joinDocs(docs, format), nil
}

// toSortedMap converts a client.Object to map[string]any via the unstructured
// converter so that encoding/json produces alphabetically sorted keys.
// The fake client strips TypeMeta when storing objects, so we look up the GVK
// from the scheme and restore apiVersion/kind in the output.
func toSortedMap(obj client.Object, scheme *runtime.Scheme) (map[string]any, error) {
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: raw}

	// Restore GVK: fake client strips TypeMeta from stored objects.
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" && scheme != nil {
		gvks, _, err := scheme.ObjectKinds(obj)
		if err == nil && len(gvks) > 0 {
			gvk = gvks[0]
		}
	}
	if gvk.Kind != "" {
		u.SetGroupVersionKind(gvk)
	}
	return u.Object, nil
}

// stripDynamicFields removes non-deterministic metadata fields and the status
// stanza in-place. Status is always server-side runtime state and is not
// meaningful in rendered manifests.
func stripDynamicFields(m map[string]any) {
	delete(m, "status")
	meta, ok := m["metadata"].(map[string]any)
	if !ok {
		return
	}
	for _, field := range dynamicFields {
		delete(meta, field)
	}
}

func joinDocs(docs [][]byte, format string) []byte {
	if format == "json" {
		return joinJSON(docs)
	}
	return joinYAML(docs)
}

func joinYAML(docs [][]byte) []byte {
	var buf bytes.Buffer
	for i, doc := range docs {
		if i > 0 {
			buf.WriteString("---\n")
		}
		buf.Write(bytes.TrimRight(doc, "\n"))
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func joinJSON(docs [][]byte) []byte {
	// Emit a JSON array of objects so the output is valid JSON.
	// Keys within each object are alphabetically sorted by encoding/json.
	var buf bytes.Buffer
	buf.WriteString("[\n")
	for i, doc := range docs {
		if i > 0 {
			buf.WriteString(",\n")
		}
		// re-indent each document with 2 extra spaces for array context; trim
		// the trailing newline that json.MarshalIndent always appends before re-indenting
		indented := "  " + strings.ReplaceAll(strings.TrimRight(string(doc), "\n"), "\n", "\n  ")
		buf.WriteString(indented)
		buf.WriteByte('\n')
	}
	buf.WriteString("]\n")
	return buf.Bytes()
}
