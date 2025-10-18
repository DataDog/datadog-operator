// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package object

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	partOfSplitChar        string = "-"
	partOfEscapedSplitChar string = "--"
)

// PartOfLabelValue is helpful to work with the "app.kubernetes.io/part-of"
// label. We use that label to track the of an object when we can't use owner
// references (cannot be used across namespaces).
// In order to identify an owner, we encode its namespace and name in the value
// of the label separated with a "-". Because names and namespaces can contain
// "-" too, we escape them. There are not any characters that are allowed in
// labels but not in names and namespaces so escaping is needed.
type PartOfLabelValue struct {
	Value string
}

// NewPartOfLabelValue creates an instance of PartOfLabelValue from a
// DatadogAgent.
func NewPartOfLabelValue(obj metav1.Object) *PartOfLabelValue {
	value := strings.ReplaceAll(obj.GetNamespace(), partOfSplitChar, partOfEscapedSplitChar) +
		partOfSplitChar +
		strings.ReplaceAll(constants.GetDDAName(obj), partOfSplitChar, partOfEscapedSplitChar)

	return &PartOfLabelValue{Value: value}
}

// NamespacedName returns the NamespaceName that corresponds to the value of a
// part-of label.
func (partOfLabelValue *PartOfLabelValue) NamespacedName() types.NamespacedName {
	l := len(partOfLabelValue.Value)
	for i, c := range partOfLabelValue.Value {
		if string(c) == partOfSplitChar {
			// Tt's split index if it's just one "-"
			isSplitIndex := (i != 0 && string(partOfLabelValue.Value[i-1]) != partOfSplitChar) &&
				(i != l-1 && string(partOfLabelValue.Value[i+1]) != partOfSplitChar)

			if isSplitIndex {
				return types.NamespacedName{
					// The ReplaceAll calls undoes the escaping done in the constructor
					Namespace: strings.ReplaceAll(partOfLabelValue.Value[:i], partOfEscapedSplitChar, partOfSplitChar),
					Name:      strings.ReplaceAll(partOfLabelValue.Value[i+1:], partOfEscapedSplitChar, partOfSplitChar),
				}
			}
		}
	}
	return types.NamespacedName{}
}

func (partOfLabelValue *PartOfLabelValue) String() string {
	return partOfLabelValue.Value
}
