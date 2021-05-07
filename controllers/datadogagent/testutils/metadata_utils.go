// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckObjectMeta define the function signature of a check that runs again a metav1.ObjectMeta object.
type CheckObjectMeta func(t *testing.T, obj *metav1.ObjectMeta)

// CheckNamespaceName used to check if the namespace and name are the expected value on a metav1.ObjectMeta.
func CheckNamespaceName(ns, name string) CheckObjectMeta {
	check := func(t *testing.T, obj *metav1.ObjectMeta) {
		if obj.Namespace != ns || obj.Name != name {
			t.Errorf("Wrong object NS/Name, want [%s/%s], got [%s/%s]", ns, name, obj.Namespace, obj.Name)
		}
	}
	return check
}

// CheckLabelIsPresent used to check if a label (key,value) is present on a metav1.ObjectMeta object.
func CheckLabelIsPresent(key, value string) CheckObjectMeta {
	check := func(t *testing.T, obj *metav1.ObjectMeta) {
		checkIsPresentInMap(t, key, value, obj.Labels, "obj.Labels")
	}

	return check
}

// CheckAnnotationIsPresent used to check if an annotation (key,value) is present on a metav1.ObjectMeta object.
func CheckAnnotationIsPresent(key, value string) CheckObjectMeta {
	check := func(t *testing.T, obj *metav1.ObjectMeta) {
		checkIsPresentInMap(t, key, value, obj.Annotations, "obj.Annotations")
	}

	return check
}

// CheckLabelIsNotPresent used to check if a label (key,value) is not present on a metav1.ObjectMeta object.
func CheckLabelIsNotPresent(key string) CheckObjectMeta {
	check := func(t *testing.T, obj *metav1.ObjectMeta) {
		checkLabelIsNotPresent(t, key, obj.Labels, "obj.Labels")
	}

	return check
}

// CheckAnnotationsIsNotPresent used to check if an annotation (key,value) is not present on a metav1.ObjectMeta object.
func CheckAnnotationsIsNotPresent(key string) CheckObjectMeta {
	check := func(t *testing.T, obj *metav1.ObjectMeta) {
		checkLabelIsNotPresent(t, key, obj.Annotations, "obj.Annotations")
	}

	return check
}

func checkIsPresentInMap(t *testing.T, key, value string, entries map[string]string, msgPrefix string) {
	if val, found := entries[key]; found {
		if val == value {
			t.Logf("[%s] key [%s] founded with value value [%s]", msgPrefix, key, value)
			return
		}
		t.Errorf("[%s] key %s founded, but wrong value, got [%s], want [%s]", msgPrefix, key, val, value)
	}
	t.Errorf("[%s] key %s not founded", msgPrefix, key)
}

func checkLabelIsNotPresent(t *testing.T, key string, entries map[string]string, msgPrefix string) {
	if value, found := entries[key]; found {
		t.Errorf("[%s] key [%s] founded with value value [%s]", msgPrefix, key, value)
		return
	}
}
