// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import "strings"

// JSONPointerEscape escapes a string for use as a segment of an RFC 6901 JSON
// Pointer, e.g. an annotation key used in a JSON Patch "path".
func JSONPointerEscape(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}

// AnnotationJSONPatchPath converts an annotation key to a JSON Patch path
// under /metadata/annotations, escaping "~" and "/" per RFC 6901.
func AnnotationJSONPatchPath(key string) string {
	return "/metadata/annotations/" + JSONPointerEscape(key)
}
