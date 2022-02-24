// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"reflect"
	"testing"

	common "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestConvertCustomConfig(t *testing.T) {
	fakeData := "fake data"
	cmName := "foo"
	fileKey := "config.yaml"
	tests := []struct {
		name   string
		config *CustomConfig
		want   *common.CustomConfig
	}{
		{
			name:   "nil customConfig",
			config: nil,
			want:   nil,
		},
		{
			name: "simple configData",
			config: &CustomConfig{
				ConfigData: &fakeData,
			},
			want: &common.CustomConfig{
				ConfigData: &fakeData,
			},
		},
		{
			name: "simple configma[",
			config: &CustomConfig{
				ConfigMap: &commonv1.ConfigMapConfig{
					Name: cmName,
					Items: []corev1.KeyToPath{
						{
							Key:  fileKey,
							Path: fileKey,
						},
					},
				},
			},
			want: &common.CustomConfig{
				ConfigMap: &commonv1.ConfigMapConfig{
					Name: cmName,
					Items: []corev1.KeyToPath{
						{
							Key:  fileKey,
							Path: fileKey,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertCustomConfig(tt.config); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertCustomConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
