// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplaneconfiguration

import (
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_controlPlaneConfigurationFeature_buildControlPlaneConfigurationConfigMap(t *testing.T) {
	owner := &metav1.ObjectMeta{
		Name:      "test",
		Namespace: "foo",
	}

	type fields struct {
		enabled       bool
		owner         metav1.Object
		provider      string
		configMapName string
	}
	tests := []struct {
		name    string
		fields  fields
		want    *corev1.ConfigMap
		wantErr bool
	}{
		{
			name: "default",
			fields: fields{
				owner:         owner,
				provider:      "default",
				configMapName: defaultControlPlaneConfigurationConfFileName,
				enabled:       true,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultControlPlaneConfigurationConfFileName, controlPlaneConfigurationConfig("default")),
		},
		{
			name: "openshift",
			fields: fields{
				owner:         owner,
				provider:      "rhcos",
				configMapName: defaultControlPlaneConfigurationConfFileName,
				enabled:       true,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultControlPlaneConfigurationConfFileName, controlPlaneConfigurationConfig("rhcos")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &controlPlaneConfigurationFeature{
				owner:         tt.fields.owner,
				enabled:       tt.fields.enabled,
				configMapName: tt.fields.configMapName,
				provider:      tt.fields.provider,
			}
			got, err := f.buildControlPlaneConfigurationConfigMap()
			fmt.Println(got)
			if (err != nil) != tt.wantErr {
				t.Errorf("controlPlaneConfigurationFeature.buildControlPlaneConfigurationConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controlPlaneConfigurationFeature.buildControlPlaneConfigurationConfigMap() = %#v,\nwant %#v", got, tt.want)
			}
		})
	}
}
