// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ksmFeature_buildKSMCoreConfigMap(t *testing.T) {
	owner := &metav1.ObjectMeta{
		Name:      "test",
		Namespace: "foo",
	}
	overrideConf := `cluster_check: true
init_config:
instances:
  - collectors:
      - pods
`
	defaultOptions := collectorOptions{}
	optionsWithVPA := collectorOptions{enableVPA: true}
	optionsWithCRD := collectorOptions{enableCRD: true}
	optionsWithAPIService := collectorOptions{enableAPIService: true}

	type fields struct {
		enable                   bool
		runInClusterChecksRunner bool
		rbacSuffix               string
		serviceAccountName       string
		owner                    metav1.Object
		customConfig             *v2alpha1.CustomConfig
		configConfigMapName      string
		collectorOpts            collectorOptions
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
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      v2alpha1.DefaultKubeStateMetricsCoreConf,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), v2alpha1.DefaultKubeStateMetricsCoreConf, ksmCheckConfig(true, defaultOptions)),
		},
		{
			name: "override",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      v2alpha1.DefaultKubeStateMetricsCoreConf,
				customConfig: &v2alpha1.CustomConfig{
					ConfigData: &overrideConf,
				},
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), v2alpha1.DefaultKubeStateMetricsCoreConf, overrideConf),
		},
		{
			name: "no cluster check runners",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configConfigMapName:      v2alpha1.DefaultKubeStateMetricsCoreConf,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), v2alpha1.DefaultKubeStateMetricsCoreConf, ksmCheckConfig(false, defaultOptions)),
		},
		{
			name: "with vpa",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      v2alpha1.DefaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithVPA,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), v2alpha1.DefaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithVPA)),
		},
		{
			name: "with CRDs",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      v2alpha1.DefaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithCRD,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), v2alpha1.DefaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithCRD)),
		},
		{
			name: "with APIServices",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      v2alpha1.DefaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithAPIService,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), v2alpha1.DefaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithAPIService)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &ksmFeature{
				runInClusterChecksRunner: tt.fields.runInClusterChecksRunner,
				rbacSuffix:               tt.fields.rbacSuffix,
				serviceAccountName:       tt.fields.serviceAccountName,
				owner:                    tt.fields.owner,
				customConfig:             tt.fields.customConfig,
				configConfigMapName:      tt.fields.configConfigMapName,
			}
			got, err := f.buildKSMCoreConfigMap(tt.fields.collectorOpts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ksmFeature.buildKSMCoreConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ksmFeature.buildKSMCoreConfigMap() = %#v,\nwant %#v", got, tt.want)
			}
		})
	}
}
