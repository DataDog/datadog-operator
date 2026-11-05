// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestApplyControlled(t *testing.T) {
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default", UID: "owner-uid"}}
	ownerReferences := []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "owner", UID: "owner-uid", Controller: new(true), BlockOwnerDeletion: new(true)}}

	tests := []struct {
		name    string
		cached  *corev1.ConfigMap
		live    *corev1.ConfigMap
		wantErr string
		wantObj *corev1.ConfigMap
	}{
		{
			name: "object unchanged since the cache read",
			cached: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "applied", Namespace: "default", ResourceVersion: "1", OwnerReferences: ownerReferences},
				Data:       map[string]string{"key": "old"},
			},
			live: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "applied", Namespace: "default", ResourceVersion: "1", OwnerReferences: ownerReferences},
				Data:       map[string]string{"key": "old"},
			},
			wantObj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "applied",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{APIVersion: "v1", Kind: "ConfigMap", Name: "owner", UID: "owner-uid", Controller: new(true), BlockOwnerDeletion: new(true)},
					},
				},
				Data: map[string]string{"key": "new"},
			},
		},
		{
			name: "owner references removed since the cache read",
			cached: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "applied", Namespace: "default", ResourceVersion: "1", OwnerReferences: ownerReferences},
				Data:       map[string]string{"key": "old"},
			},
			live: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "applied", Namespace: "default", ResourceVersion: "2"},
				Data:       map[string]string{"key": "old"},
			},
			wantErr: "Operation cannot be fulfilled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(owner, tt.live).Build()
			cache := interceptor.NewClient(c, interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, object client.Object, _ ...client.GetOption) error {
					tt.cached.DeepCopyInto(object.(*corev1.ConfigMap))
					return nil
				},
			})
			desired := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "applied", Namespace: "default"},
				Data:       map[string]string{"key": "new"},
			}

			err := ApplyControlled(ctx, cache, c, clientgoscheme.Scheme, owner, desired, "test")

			if err != nil {
				if tt.wantErr == "" {
					t.Errorf("ApplyControlled() error = %v, wantErr %v", err, tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ApplyControlled() error = %v, want an error containing %q", err, tt.wantErr)
				}
				return
			}

			if tt.wantErr != "" {
				t.Errorf("ApplyControlled() return no error, want %q", tt.wantErr)
			}

			got := &corev1.ConfigMap{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(desired), got); err != nil {
				t.Fatalf("Get() error: %v", err)
			}
			if diff := cmp.Diff(tt.wantObj, got, cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion", "ManagedFields")); diff != "" {
				t.Errorf("object mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
