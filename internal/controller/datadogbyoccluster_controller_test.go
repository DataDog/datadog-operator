// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type patchRecordingClient struct {
	client.Client
	patchType    k8stypes.PatchType
	patchOptions client.PatchOptions
}

func (c *patchRecordingClient) Patch(_ context.Context, _ client.Object, patch client.Patch, opts ...client.PatchOption) error {
	c.patchType = patch.Type()
	c.patchOptions.ApplyOptions(opts)
	return nil
}

func TestDatadogBYOCClusterReconcilerApplyObject(t *testing.T) {
	tests := []struct {
		name    string
		desired client.Object
		wantGVK schema.GroupVersionKind
	}{
		{
			name:    "ConfigMap",
			desired: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: "testing"}},
			wantGVK: corev1.SchemeGroupVersion.WithKind("ConfigMap"),
		},
		{
			name:    "StatefulSet",
			desired: &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "indexer", Namespace: "testing"}},
			wantGVK: appsv1.SchemeGroupVersion.WithKind("StatefulSet"),
		},
		{
			name:    "PodDisruptionBudget",
			desired: &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "indexer", Namespace: "testing"}},
			wantGVK: policyv1.SchemeGroupVersion.WithKind("PodDisruptionBudget"),
		},
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() unexpected error: %v", err)
	}
	if err := datadoghqv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() unexpected error: %v", err)
	}
	owner := &datadoghqv1alpha1.DatadogBYOCCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: "testing", UID: k8stypes.UID("owner-uid")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := &patchRecordingClient{}
			reconciler := &DatadogBYOCClusterReconciler{Client: kubeClient, Scheme: scheme}

			if err := reconciler.applyObject(context.Background(), owner, tt.desired); err != nil {
				t.Fatalf("applyObject() unexpected error: %v", err)
			}
			if got := kubeClient.patchType; got != k8stypes.ApplyPatchType {
				t.Errorf("Patch() type = %q, want %q", got, k8stypes.ApplyPatchType)
			}
			if got := kubeClient.patchOptions.FieldManager; got != datadogBYOCClusterFieldOwner {
				t.Errorf("Patch() field manager = %q, want %q", got, datadogBYOCClusterFieldOwner)
			}
			if kubeClient.patchOptions.Force == nil || !*kubeClient.patchOptions.Force {
				t.Error("Patch() force ownership = false, want true")
			}
			if got := tt.desired.GetObjectKind().GroupVersionKind(); got != tt.wantGVK {
				t.Errorf("applied GVK = %s, want %s", got, tt.wantGVK)
			}
			if !metav1.IsControlledBy(tt.desired, owner) {
				t.Error("applied object is not controlled by its DatadogBYOCCluster")
			}
		})
	}
}

func TestDatadogBYOCClusterReconcilerDeletePodDisruptionBudget(t *testing.T) {
	controllerOwnerReference := metav1.OwnerReference{
		APIVersion:         datadoghqv1alpha1.GroupVersion.String(),
		Kind:               "DatadogBYOCCluster",
		Name:               "byoc",
		UID:                k8stypes.UID("owner-uid"),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
	tests := []struct {
		name            string
		ownerReferences []metav1.OwnerReference
		wantDeleted     bool
	}{
		{
			name:            "owned PDB",
			ownerReferences: []metav1.OwnerReference{controllerOwnerReference},
			wantDeleted:     true,
		},
		{
			name: "unowned PDB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := clientgoscheme.AddToScheme(scheme); err != nil {
				t.Fatalf("AddToScheme() unexpected error: %v", err)
			}
			if err := datadoghqv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatalf("AddToScheme() unexpected error: %v", err)
			}
			owner := &datadoghqv1alpha1.DatadogBYOCCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: "testing", UID: k8stypes.UID("owner-uid")},
			}
			podDisruptionBudget := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{
				Name:            "byoc-indexer",
				Namespace:       "testing",
				OwnerReferences: tt.ownerReferences,
			}}
			kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner, podDisruptionBudget).Build()
			reconciler := &DatadogBYOCClusterReconciler{Client: kubeClient}

			if err := reconciler.deletePodDisruptionBudget(context.Background(), owner, "indexer"); err != nil {
				t.Fatalf("deletePodDisruptionBudget() unexpected error: %v", err)
			}
			err := kubeClient.Get(context.Background(), client.ObjectKeyFromObject(podDisruptionBudget), &policyv1.PodDisruptionBudget{})
			gotDeleted := apierrors.IsNotFound(err)
			if err != nil && !gotDeleted {
				t.Fatalf("Get() unexpected error: %v", err)
			}
			if gotDeleted != tt.wantDeleted {
				t.Errorf("deleted = %v, want %v", gotDeleted, tt.wantDeleted)
			}
		})
	}
}
