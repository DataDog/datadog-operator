// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package podtemplate

import (
	"context"
	"errors"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/api/v1alpha1/test"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func podTemplateSpec(ctrName, ctrImage string) *corev1.PodTemplateSpec {
	return &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  ctrName,
					Image: ctrImage,
				},
			},
		},
	}
}

func defaultOwnerRef() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         "datadoghq.com/v1alpha1",
			Name:               "eds-name",
			Kind:               "ExtendedDaemonSet",
			Controller:         datadoghqv1alpha1.NewBool(true),
			BlockOwnerDeletion: datadoghqv1alpha1.NewBool(true),
		},
	}
}

func defaultRequest() reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "eds-ns",
			Name:      "eds-name",
		},
	}
}

func TestReconciler_newPodTemplate(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSet{})
	tests := []struct {
		name    string
		eds     *datadoghqv1alpha1.ExtendedDaemonSet
		want    *corev1.PodTemplate
		wantErr bool
	}{
		{
			name: "nominal case",
			eds:  test.NewExtendedDaemonSet("eds-ns", "eds-name", &test.NewExtendedDaemonSetOptions{PodTemplateSpec: podTemplateSpec("name", "image")}),
			want: &corev1.PodTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "eds-name",
					Namespace: "eds-ns",
					Labels: map[string]string{
						"cluster-autoscaler.kubernetes.io/daemonset-pod": "true",
					},
					Annotations: map[string]string{
						"extendeddaemonset.datadoghq.com/templatehash": "c08e4c7b196a8a9ba3fd4723a4366c8b",
					},
					OwnerReferences: defaultOwnerRef(),
				},
				Template: *podTemplateSpec("name", "image"),
			},
			wantErr: false,
		},
		{
			name: "with extra labels and annotations",
			eds:  test.NewExtendedDaemonSet("eds-ns", "eds-name", &test.NewExtendedDaemonSetOptions{Labels: map[string]string{"l-key": "l-val"}, Annotations: map[string]string{"a-key": "a-val"}, PodTemplateSpec: podTemplateSpec("name", "image")}),
			want: &corev1.PodTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "eds-name",
					Namespace: "eds-ns",
					Labels: map[string]string{
						"cluster-autoscaler.kubernetes.io/daemonset-pod": "true",
						"l-key": "l-val",
					},
					Annotations: map[string]string{
						"extendeddaemonset.datadoghq.com/templatehash": "c08e4c7b196a8a9ba3fd4723a4366c8b",
						"a-key": "a-val",
					},
					OwnerReferences: defaultOwnerRef(),
				},
				Template: *podTemplateSpec("name", "image"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{scheme: s}
			got, err := r.newPodTemplate(tt.eds)
			assert.True(t, (err == nil), tt.wantErr)
			assert.EqualValues(t, tt.want, got)
		})
	}
}

func TestReconciler_Reconcile(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.ExtendedDaemonSet{})
	tests := []struct {
		name     string
		request  reconcile.Request
		loadFunc func(c client.Client)
		want     reconcile.Result
		wantErr  bool
		wantFunc func(c client.Client) error
	}{
		{
			name:    "EDS not found",
			request: defaultRequest(),
			want:    reconcile.Result{},
			wantErr: true,
		},
		{
			name:    "PodTemplate not found",
			request: defaultRequest(),
			loadFunc: func(c client.Client) {
				eds := test.NewExtendedDaemonSet("eds-ns", "eds-name", &test.NewExtendedDaemonSetOptions{PodTemplateSpec: podTemplateSpec("name", "image")})
				eds = datadoghqv1alpha1.DefaultExtendedDaemonSet(eds, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)
				_ = c.Create(context.TODO(), eds)
			},
			want:    reconcile.Result{},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				return c.Get(context.TODO(), defaultRequest().NamespacedName, &corev1.PodTemplate{})
			},
		},
		{
			name:    "PodTemplate found and up-to-date",
			request: defaultRequest(),
			loadFunc: func(c client.Client) {
				eds := test.NewExtendedDaemonSet("eds-ns", "eds-name", &test.NewExtendedDaemonSetOptions{PodTemplateSpec: podTemplateSpec("name", "image")})
				eds = datadoghqv1alpha1.DefaultExtendedDaemonSet(eds, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)
				_ = c.Create(context.TODO(), eds)
				podTemplate := &corev1.PodTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "eds-name",
						Namespace: "eds-ns",
						Labels: map[string]string{
							"cluster-autoscaler.kubernetes.io/daemonset-pod": "true",
						},
						Annotations: map[string]string{
							"extendeddaemonset.datadoghq.com/templatehash": "c08e4c7b196a8a9ba3fd4723a4366c8b",
						},
						OwnerReferences: defaultOwnerRef(),
					},
					Template: *podTemplateSpec("name", "image"),
				}
				_ = c.Create(context.TODO(), podTemplate)
			},
			want:    reconcile.Result{},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				podTemplate := &corev1.PodTemplate{}

				return c.Get(context.TODO(), defaultRequest().NamespacedName, podTemplate)
			},
		},
		{
			name:    "PodTemplate outdated",
			request: defaultRequest(),
			loadFunc: func(c client.Client) {
				eds := test.NewExtendedDaemonSet("eds-ns", "eds-name", &test.NewExtendedDaemonSetOptions{PodTemplateSpec: podTemplateSpec("new-name", "new-image")})
				eds = datadoghqv1alpha1.DefaultExtendedDaemonSet(eds, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)
				_ = c.Create(context.TODO(), eds)
				podTemplate := &corev1.PodTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "eds-name",
						Namespace: "eds-ns",
						Labels: map[string]string{
							"cluster-autoscaler.kubernetes.io/daemonset-pod": "true",
						},
						Annotations: map[string]string{
							"extendeddaemonset.datadoghq.com/templatehash": "outdated-hash",
						},
						OwnerReferences: defaultOwnerRef(),
					},
					Template: *podTemplateSpec("name", "image"),
				}
				_ = c.Create(context.TODO(), podTemplate)
			},
			want:    reconcile.Result{},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				podTemplate := &corev1.PodTemplate{}
				err := c.Get(context.TODO(), defaultRequest().NamespacedName, podTemplate)
				if err != nil {
					return err
				}

				err = errors.New("podTemplate not updated")
				if podTemplate.Annotations["extendeddaemonset.datadoghq.com/templatehash"] == "outdated-hash" {
					return err
				}

				if podTemplate.Template.Spec.Containers[0].Name != "new-name" {
					return err
				}

				if podTemplate.Template.Spec.Containers[0].Image != "new-image" {
					return err
				}

				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:   fake.NewClientBuilder().Build(),
				scheme:   s,
				recorder: record.NewBroadcaster().NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconciler_Reconcile"}),
				log:      logf.Log.WithName("test"),
			}

			if tt.loadFunc != nil {
				tt.loadFunc(r.client)
			}

			got, err := r.Reconcile(context.TODO(), tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			assert.Equal(t, tt.want, got)
			if tt.wantFunc != nil {
				if err := tt.wantFunc(r.client); err != nil {
					t.Errorf("Reconciler.Reconcile() wantFunc validation error: %v", err)
				}
			}
		})
	}
}
