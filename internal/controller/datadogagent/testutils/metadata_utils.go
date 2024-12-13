// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils_test

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckObjectMeta define the function signature of a check that runs again a metav1.ObjectMeta object.
type CheckObjectMeta func(t *testing.T, obj *metav1.ObjectMeta)

// ObjetMetaCheckInterface use as ObjectMeta check interface
type ObjetMetaCheckInterface interface {
	Check(t *testing.T, obj *metav1.ObjectMeta) error
}

// CheckNameNamespace used to check if the namespace and name are the expected value on a metav1.ObjectMeta.
type CheckNameNamespace struct {
	Namespace string
	Name      string
}

// Check used to check if the namespace and name are the expected value on a metav1.ObjectMeta.
func (c *CheckNameNamespace) Check(t *testing.T, obj *metav1.ObjectMeta) error {
	if obj.Namespace != c.Namespace || obj.Name != c.Name {
		return fmt.Errorf("wrong object NS/Name, want [%s/%s], got [%s/%s]", c.Namespace, c.Name, obj.Namespace, obj.Name)
	}
	return nil
}

// CheckLabelIsPresent used to check if a label (key,value) is present on a metav1.ObjectMeta object.
type CheckLabelIsPresent struct {
	Key   string
	Value string
}

// Check used to check if a label (key,value) is present on a metav1.ObjectMeta object.
func (c *CheckLabelIsPresent) Check(t *testing.T, obj *metav1.ObjectMeta) error {
	return checkIsPresentInMap(t, c.Key, c.Value, obj.Labels, "obj.Labels")
}

// CheckAnnotationIsPresent used to check if an annotation (key,value) is present on a metav1.ObjectMeta object.
type CheckAnnotationIsPresent struct {
	Key   string
	Value string
}

// Check used to check if an annotation (key,value) is present on a metav1.ObjectMeta object.
func (c *CheckAnnotationIsPresent) Check(t *testing.T, obj *metav1.ObjectMeta) error {
	return checkIsPresentInMap(t, c.Key, c.Value, obj.Annotations, "obj.Annotations")
}

// CheckLabelIsNotPresent used to check if a label key is not present on a metav1.ObjectMeta object.
type CheckLabelIsNotPresent struct {
	Key string
}

// Check used to check if a label key is not present on a metav1.ObjectMeta object.
func (c *CheckLabelIsNotPresent) Check(t *testing.T, obj *metav1.ObjectMeta) error {
	return checkLabelIsNotPresent(t, c.Key, obj.Labels, "obj.Labels")
}

// CheckAnnotationsIsNotPresent used to check if a annotation key is not present on a metav1.ObjectMeta object.
type CheckAnnotationsIsNotPresent struct {
	Key string
}

// Check used to check if an annotation (key,value) is not present on a metav1.ObjectMeta object.
func (c *CheckAnnotationsIsNotPresent) Check(t *testing.T, obj *metav1.ObjectMeta) error {
	return checkLabelIsNotPresent(t, c.Key, obj.Annotations, "obj.Annotations")
}

func checkIsPresentInMap(t *testing.T, key, value string, entries map[string]string, msgPrefix string) error {
	if val, found := entries[key]; found {
		if val == value {
			t.Logf("[%s] key [%s] found with value [%s]", msgPrefix, key, value)
			return nil
		}
		return fmt.Errorf("[%s] key %s found, but wrong value, got [%s], want [%s]", msgPrefix, key, val, value)
	}
	return fmt.Errorf("[%s] key %s not found", msgPrefix, key)
}

func checkLabelIsNotPresent(_ *testing.T, key string, entries map[string]string, msgPrefix string) error {
	if value, found := entries[key]; found {
		return fmt.Errorf("[%s] key [%s] found with value [%s]", msgPrefix, key, value)
	}
	return nil
}
