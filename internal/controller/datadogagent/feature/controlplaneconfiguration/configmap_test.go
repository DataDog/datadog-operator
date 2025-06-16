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
		enabled  bool
		owner    metav1.Object
		provider string
	}
	tests := []struct {
		name          string
		fields        fields
		configMapName string
		want          *corev1.ConfigMap
		wantErr       bool
	}{
		{
			name: "default",
			fields: fields{
				owner:    owner,
				provider: "default",
				enabled:  true,
			},
			configMapName: "datadog-controlplane-configuration-default",
			want:          buildDefaultConfigMap(owner.GetNamespace(), "datadog-controlplane-configuration-default", controlPlaneConfigurationConfig("default")),
		},
		{
			name: "openshift",
			fields: fields{
				owner:    owner,
				provider: "rhcos",
				enabled:  true,
			},
			configMapName: "datadog-controlplane-configuration-openshift",
			want:          buildDefaultConfigMap(owner.GetNamespace(), "datadog-controlplane-configuration-openshift", controlPlaneConfigurationConfig("rhcos")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &controlPlaneConfigurationFeature{
				owner:    tt.fields.owner,
				enabled:  tt.fields.enabled,
				provider: tt.fields.provider,
			}
			got, err := f.buildControlPlaneConfigurationConfigMap(tt.fields.provider, tt.configMapName)
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
