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
	optionsWithControllerRevisions := collectorOptions{enableControllerRevisions: true}

	// Test custom resources
	optionsWithCustomResources := collectorOptions{
		customResources: []v2alpha1.Resource{
			{
				GroupVersionKind: v2alpha1.GroupVersionKind{
					Group:   "monitoring.example.com",
					Version: "v1beta1",
					Kind:    "ServiceMonitor",
				},
			},
		},
	}

	optionsWithMultipleCustomResources := collectorOptions{
		customResources: []v2alpha1.Resource{
			{
				GroupVersionKind: v2alpha1.GroupVersionKind{
					Group:   "kafka.strimzi.io",
					Version: "v1beta2",
					Kind:    "Kafka",
				},
			},
			{
				GroupVersionKind: v2alpha1.GroupVersionKind{
					Group:   "postgresql.cnpg.io",
					Version: "v1",
					Kind:    "Cluster",
				},
			},
		},
	}

	// Combined options
	optionsWithVPAAndCustomResources := collectorOptions{
		enableVPA: true,
		customResources: []v2alpha1.Resource{
			{
				GroupVersionKind: v2alpha1.GroupVersionKind{
					Group:   "networking.istio.io",
					Version: "v1beta1",
					Kind:    "VirtualService",
				},
			},
		},
	}

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
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, defaultOptions)),
		},
		{
			name: "override",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
				customConfig: &v2alpha1.CustomConfig{
					ConfigData: &overrideConf,
				},
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, overrideConf),
		},
		{
			name: "no cluster check runners",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(false, defaultOptions)),
		},
		{
			name: "with vpa",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithVPA,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithVPA)),
		},
		{
			name: "with CRDs",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithCRD,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithCRD)),
		},
		{
			name: "with APIServices",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithAPIService,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithAPIService)),
		},
		{
			name: "with ControllerRevisions",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithControllerRevisions,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithControllerRevisions)),
		},
		{
			name: "with custom resources",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithCustomResources,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithCustomResources)),
		},
		{
			name: "with multiple custom resources",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithMultipleCustomResources,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithMultipleCustomResources)),
		},
		{
			name: "with VPA and custom resources",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithVPAAndCustomResources,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithVPAAndCustomResources)),
		},
		{
			name: "with custom resources and no cluster check",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configConfigMapName:      defaultKubeStateMetricsCoreConf,
				collectorOpts:            optionsWithCustomResources,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(false, optionsWithCustomResources)),
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
