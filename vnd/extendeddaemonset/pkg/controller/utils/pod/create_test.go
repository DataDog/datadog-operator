// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package pod

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	datadoghqv1alpha1test "github.com/DataDog/extendeddaemonset/api/v1alpha1/test"
	ctrltest "github.com/DataDog/extendeddaemonset/pkg/controller/test"
)

func Test_overwriteResourcesFromNode(t *testing.T) {
	type args struct {
		template     *corev1.PodTemplateSpec
		edsNamespace string
		edsName      string
		node         *corev1.Node
	}
	tests := []struct {
		name         string
		args         args
		wantErr      bool
		wantTemplate *corev1.PodTemplateSpec
	}{
		{
			name: "nil node",
			args: args{
				template: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container1"},
						},
					},
				},
				edsNamespace: "bar",
				edsName:      "foo",
			},
			wantErr: false,
		},
		{
			name: "no annotation",
			args: args{
				template: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container1"},
						},
					},
				},
				edsNamespace: "bar",
				edsName:      "foo",
				node:         ctrltest.NewNode("node1", nil),
			},
			wantErr: false,
		},
		{
			name: "annotation requests.cpu",
			args: args{
				template: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container1"},
						},
					},
				},
				edsNamespace: "bar",
				edsName:      "foo",
				node: ctrltest.NewNode("node1", &ctrltest.NewNodeOptions{
					Annotations: map[string]string{
						fmt.Sprintf(datadoghqv1alpha1.ExtendedDaemonSetRessourceNodeAnnotationKey, "bar", "foo", "container1"): `{"Requests": {"cpu": "1.5"}}`,
					},
				}),
			},
			wantErr: false,
			wantTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "container1",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1.5"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "annotation requests.cpu",
			args: args{
				template: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "container1"},
						},
					},
				},
				edsNamespace: "bar",
				edsName:      "foo",
				node: ctrltest.NewNode("node1", &ctrltest.NewNodeOptions{
					Annotations: map[string]string{
						fmt.Sprintf(datadoghqv1alpha1.ExtendedDaemonSetRessourceNodeAnnotationKey, "bar", "foo", "container1"): `{"Requests": invalid {"cpu": "1.5"}}`,
					},
				}),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := overwriteResourcesFromNode(tt.args.template, tt.args.edsNamespace, tt.args.edsName, tt.args.node); (err != nil) != tt.wantErr {
				t.Errorf("overwriteResourcesFromNode() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantTemplate != nil {
				if diff := cmp.Diff(tt.wantTemplate, tt.args.template); diff != "" {
					t.Errorf("template mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func Test_overwriteResourcesFromEdsNode(t *testing.T) {
	templateOriginal := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "container1"},
			},
		},
	}

	templateCopy := templateOriginal.DeepCopy()

	// nil case, no change to template
	edsSettingName := "bar"
	edsSettingNamespace := "foo"
	edsNode := &datadoghqv1alpha1.ExtendedDaemonsetSetting{
		ObjectMeta: v1.ObjectMeta{
			Name:      edsSettingName,
			Namespace: edsSettingNamespace,
		},
	}
	overwriteResourcesFromEdsNode(templateOriginal, edsNode)
	assert.Equal(t, templateCopy.Spec, templateOriginal.Spec)
	assert.Equal(t, templateOriginal.GetLabels()[datadoghqv1alpha1.ExtendedDaemonSetSettingNameLabelKey], edsSettingName)
	assert.Equal(t, templateOriginal.GetLabels()[datadoghqv1alpha1.ExtendedDaemonSetSettingNamespaceLabelKey], edsSettingNamespace)

	// template changed
	resourcesRef := corev1.ResourceList{
		"cpu":    resource.MustParse("0.1"),
		"memory": resource.MustParse("20M"),
	}
	edsNode = datadoghqv1alpha1test.NewExtendedDaemonsetSetting(edsSettingNamespace, edsSettingName, "reference", &datadoghqv1alpha1test.NewExtendedDaemonsetSettingOptions{
		CreationTime: time.Now(),
		Resources: map[string]corev1.ResourceRequirements{
			"container1": {
				Requests: resourcesRef,
			},
		},
	})
	overwriteResourcesFromEdsNode(templateOriginal, edsNode)
	assert.NotEqual(t, templateCopy, templateOriginal)
	assert.Equal(t, resourcesRef, templateOriginal.Spec.Containers[0].Resources.Requests)
}
