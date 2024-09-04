// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/api/datadoghq/common/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertCustomConfig(t *testing.T) {
	fakeData := "fake data"
	cmName := "foo"
	fileKey := "config.yaml"
	tests := []struct {
		name   string
		config *CustomConfig
		want   *commonv1.CustomConfig
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
			want: &commonv1.CustomConfig{
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
			want: &commonv1.CustomConfig{
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

func TestServiceAccountOverride(t *testing.T) {
	customServiceAccount := "fake"
	ddaName := "test-dda"
	tests := []struct {
		name string
		dda  *DatadogAgent
		want map[ComponentName]string
	}{
		{
			name: "custom serviceaccount for dca and clc",
			dda: &DatadogAgent{
				ObjectMeta: v1.ObjectMeta{
					Name: ddaName,
				},
				Spec: DatadogAgentSpec{
					Override: map[ComponentName]*DatadogAgentComponentOverride{
						ClusterAgentComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
						ClusterChecksRunnerComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
					},
				},
			},
			want: map[ComponentName]string{
				ClusterAgentComponentName:        customServiceAccount,
				NodeAgentComponentName:           fmt.Sprintf("%s-%s", ddaName, common.DefaultAgentResourceSuffix),
				ClusterChecksRunnerComponentName: customServiceAccount,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := map[ComponentName]string{}
			res[NodeAgentComponentName] = GetAgentServiceAccount(tt.dda)
			res[ClusterChecksRunnerComponentName] = GetClusterChecksRunnerServiceAccount(tt.dda)
			res[ClusterAgentComponentName] = GetClusterAgentServiceAccount(tt.dda)
			for name, sa := range tt.want {
				if res[name] != sa {
					t.Errorf("Service Account Override error = %v, want %v", res[name], tt.want[name])
				}
			}
		})
	}
}
