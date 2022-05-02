// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_buildConfigurationConfigMap(t *testing.T) {
	defaultDda := test.NewDefaultedDatadogAgent("bar", "foo", nil)

	ddaWithConfigData := defaultDda.DeepCopy()
	dataContent := "config: data"
	ddaWithConfigData.Spec.Agent.CustomConfig = &datadoghqv1alpha1.CustomConfigSpec{
		ConfigData: &dataContent,
	}

	ddaWithConfigMap := defaultDda.DeepCopy()
	ddaWithConfigMap.Spec.Agent.CustomConfig = &datadoghqv1alpha1.CustomConfigSpec{
		ConfigMap: &datadoghqv1alpha1.ConfigFileConfigMapSpec{
			Name: "foo",
		},
	}
	type args struct {
		dda           *datadoghqv1alpha1.DatadogAgent
		cfcm          *datadoghqv1alpha1.CustomConfigSpec
		configMapName string
		subPath       string
	}
	tests := []struct {
		name    string
		args    args
		want    *corev1.ConfigMap
		wantErr bool
	}{
		{
			name: "nil customConfig",
			args: args{
				dda:           defaultDda,
				cfcm:          defaultDda.Spec.Agent.CustomConfig,
				configMapName: "foo",
				subPath:       "datadog.yaml",
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "customConfig.ConfigData set",
			args: args{
				dda:           ddaWithConfigData,
				cfcm:          ddaWithConfigData.Spec.Agent.CustomConfig,
				configMapName: "foo",
				subPath:       "datadog.yaml",
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "foo",
					Namespace:   "bar",
					Labels:      getDefaultLabels(ddaWithConfigData, ddaWithConfigData.Name, getAgentVersion(ddaWithConfigData)),
					Annotations: getDefaultAnnotations(ddaWithConfigData),
				},
				Data: map[string]string{
					"datadog.yaml": dataContent,
				},
			},
			wantErr: false,
		},
		{
			name: "customConfig.ConfigMap set",
			args: args{
				dda:           ddaWithConfigMap,
				cfcm:          ddaWithConfigMap.Spec.Agent.CustomConfig,
				configMapName: "foo",
				subPath:       "datadog.yaml",
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildConfigurationConfigMap(tt.args.dda, datadoghqv1alpha1.ConvertCustomConfig(tt.args.cfcm), tt.args.configMapName, tt.args.subPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildConfigurationConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !apiequality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("buildConfigurationConfigMap() \n got:%#v\nwant %#v", got, tt.want)
			}
		})
	}
}
