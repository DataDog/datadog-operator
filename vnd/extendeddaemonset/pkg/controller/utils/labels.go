// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"regexp"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ResourceNamePromLabel refers to the resource name label key.
	ResourceNamePromLabel = "name"
	// ResourceNamespacePromLabel refers to the resource namespace label key.
	ResourceNamespacePromLabel = "namespace"
)

var invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// GetLabelsValues returns name and namespace as label values.
func GetLabelsValues(obj *metav1.ObjectMeta) ([]string, []string) {
	return []string{
		ResourceNamespacePromLabel,
		ResourceNamePromLabel,
	}, []string{obj.GetNamespace(), obj.GetName()}
}

// BuildInfoLabels build the lists of label keys and values from the ObjectMeta Labels.
func BuildInfoLabels(obj *metav1.ObjectMeta) ([]string, []string) {
	labelKeys := []string{}
	for key := range obj.Labels {
		labelKeys = append(labelKeys, sanitizeLabelName(key))
	}
	sort.Strings(labelKeys)

	labelValues := make([]string, len(obj.Labels))
	for i, key := range labelKeys {
		labelValues[i] = obj.Labels[key]
	}

	return labelKeys, labelValues
}

func sanitizeLabelName(s string) string {
	return invalidLabelCharRE.ReplaceAllString(s, "_")
}
