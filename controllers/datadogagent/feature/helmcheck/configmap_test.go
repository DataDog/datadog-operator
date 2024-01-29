// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
)

func Test_buildHelmCheckConfigMap(t *testing.T) {
	owner := &metav1.ObjectMeta{
		Name:      "test",
		Namespace: "foo",
	}
	overrideConf := `---
cluster_check: true
init_config:
instances:
  - collect_events: true
    helm_values_as_tags:
      foo: bar
      zip: zap
`

	type fields struct {
		enable                   bool
		runInClusterChecksRunner bool
		rbacSuffix               string
		serviceAccountName       string
		owner                    metav1.Object
		customConfig             *apicommonv1.CustomConfig
		configMapName            string

		collectEvents bool
		valuesAsTags  map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		want    *corev1.ConfigMap
		wantErr bool
	}{
		{
			name: "default helm check",
			fields: fields{
				owner:         owner,
				enable:        true,
				configMapName: apicommon.DefaultHelmCheckConf,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), apicommon.DefaultHelmCheckConf, helmCheckConfig(false, false, nil)),
		},
		{
			name: "override configmap",
			fields: fields{
				owner:         owner,
				enable:        true,
				configMapName: apicommon.DefaultHelmCheckConf,
				customConfig: &apicommonv1.CustomConfig{
					ConfigData: &overrideConf,
				},
				collectEvents: false, // should be overridden by custom configMap
				valuesAsTags:  nil,   // should be overridden by custom configMap
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), apicommon.DefaultHelmCheckConf, overrideConf),
		},
		{
			name: "no cluster check runners",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configMapName:            apicommon.DefaultHelmCheckConf,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), apicommon.DefaultHelmCheckConf, helmCheckConfig(false, false, nil)),
		},
		{
			name: "collect events",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configMapName:            apicommon.DefaultHelmCheckConf,
				collectEvents:            true,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), apicommon.DefaultHelmCheckConf, helmCheckConfig(true, true, nil)),
		},
		{
			name: "collect events, no cluster check runners",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configMapName:            apicommon.DefaultHelmCheckConf,
				collectEvents:            true,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), apicommon.DefaultHelmCheckConf, helmCheckConfig(false, true, nil)),
		},
		{
			name: "values as tags",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configMapName:            apicommon.DefaultHelmCheckConf,
				valuesAsTags:             map[string]string{"foo": "bar", "zip": "zap"},
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), apicommon.DefaultHelmCheckConf, helmCheckConfig(true, false, map[string]string{"foo": "bar", "zip": "zap"})),
		},
		{
			name: "values as tags, no cluster check runners",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configMapName:            apicommon.DefaultHelmCheckConf,
				valuesAsTags:             map[string]string{"foo": "bar", "zip": "zap"},
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), apicommon.DefaultHelmCheckConf, helmCheckConfig(false, false, map[string]string{"foo": "bar", "zip": "zap"})),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &helmCheckFeature{
				runInClusterChecksRunner: tt.fields.runInClusterChecksRunner,
				rbacSuffix:               tt.fields.rbacSuffix,
				serviceAccountName:       tt.fields.serviceAccountName,
				owner:                    tt.fields.owner,
				customConfig:             tt.fields.customConfig,
				configMapName:            tt.fields.configMapName,
				collectEvents:            tt.fields.collectEvents,
				valuesAsTags:             tt.fields.valuesAsTags,
			}
			got, err := f.buildHelmCheckConfigMap()
			if (err != nil) != tt.wantErr {
				t.Errorf("helmCheckFeature.buildHelmCheckConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got helmCheckFeature.buildHelmCheckConfigMap() = %#v,\nwant %#v", got, tt.want)
			}
		})
	}
}
