// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testAgentName = "test-agent"
	fooNamespace  = "foo"
)

func TestGetObjKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		obj      func() client.Object
		expected string
	}{
		{
			name: "DatadogAgent with GVK",
			obj: func() client.Object {
				dda := &v2alpha1.DatadogAgent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testAgentName,
						Namespace: fooNamespace,
					},
				}
				dda.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   rbac.DatadogAPIGroup,
					Version: "v2alpha1",
					Kind:    datadogAgentKind,
				})
				return dda
			},
			expected: datadogAgentKind,
		},
		{
			name: "DatadogAgentInternal with GVK",
			obj: func() client.Object {
				ddai := &v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testAgentName,
						Namespace: fooNamespace,
					},
				}
				ddai.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   rbac.DatadogAPIGroup,
					Version: "v1alpha1",
					Kind:    datadogAgentInternalKind,
				})
				return ddai
			},
			expected: datadogAgentInternalKind,
		},
		{
			name: "Generic object with last-applied-configuration annotation - no GVK set",
			obj: func() client.Object {
				obj := &unstructured.Unstructured{}
				obj.SetName(testAgentName)
				obj.SetNamespace(fooNamespace)
				obj.SetAnnotations(map[string]string{
					"kubectl.kubernetes.io/last-applied-configuration": `{"kind":"` + datadogAgentKind + `"}`,
				})
				return obj
			},
			expected: datadogAgentKind,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clientObj := tt.obj()

			result := getObjKind(clientObj)
			if result != tt.expected {
				t.Errorf("getObjKind() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestGetObjID_DifferentKindsSameName(t *testing.T) {
	t.Parallel()

	// Create DatadogAgent and DatadogAgentInternal with the same name and namespace
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAgentName,
			Namespace: fooNamespace,
		},
	}
	dda.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   rbac.DatadogAPIGroup,
		Version: "v2alpha1",
		Kind:    datadogAgentKind,
	})

	ddai := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAgentName,
			Namespace: fooNamespace,
		},
	}
	ddai.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   rbac.DatadogAPIGroup,
		Version: "v1alpha1",
		Kind:    datadogAgentInternalKind,
	})

	ddaID := getObjID(dda)
	ddaiID := getObjID(ddai)

	if ddaID == ddaiID {
		t.Errorf("Expected different IDs for different object kinds with same name/namespace. Got: %s", ddaID)
	}

	expectedDdaID := "DatadogAgent/foo/" + testAgentName
	expectedDdaiID := "DatadogAgentInternal/foo/" + testAgentName

	if ddaID != expectedDdaID {
		t.Errorf("DatadogAgent ID = %q, expected %q", ddaID, expectedDdaID)
	}

	if ddaiID != expectedDdaiID {
		t.Errorf("DatadogAgentInternal ID = %q, expected %q", ddaiID, expectedDdaiID)
	}
}
