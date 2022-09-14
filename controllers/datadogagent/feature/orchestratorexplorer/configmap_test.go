// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"reflect"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_orchestratorExplorerFeature_buildOrchestratorExplorerConfigMap(t *testing.T) {
	owner := &metav1.ObjectMeta{
		Name:      "test",
		Namespace: "foo",
	}
	overrideConf := `cluster_check: true
init_config:
instances:
  - collectors:
      - nodes
      - services
`
	type fields struct {
		enable               bool
		clusterChecksEnabled bool
		rbacSuffix           string
		serviceAccountName   string
		owner                metav1.Object
		customConfig         *apicommonv1.CustomConfig
		configConfigMapName  string
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
				owner:                owner,
				enable:               true,
				clusterChecksEnabled: true,
				configConfigMapName:  apicommon.DefaultOrchestratorExplorerConf,
			},
			want: buildDefaultConfigMap(owner, apicommon.DefaultOrchestratorExplorerConf, orchestratorExplorerCheckConfig(true)),
		},
		{
			name: "override",
			fields: fields{
				owner:                owner,
				enable:               true,
				clusterChecksEnabled: true,
				configConfigMapName:  apicommon.DefaultOrchestratorExplorerConf,
				customConfig: &apicommonv1.CustomConfig{
					ConfigData: &overrideConf,
				},
			},
			want: buildDefaultConfigMap(owner, apicommon.DefaultOrchestratorExplorerConf, overrideConf),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &orchestratorExplorerFeature{
				clusterChecksEnabled: tt.fields.clusterChecksEnabled,
				rbacSuffix:           tt.fields.rbacSuffix,
				serviceAccountName:   tt.fields.serviceAccountName,
				owner:                tt.fields.owner,
				customConfig:         tt.fields.customConfig,
				configConfigMapName:  tt.fields.configConfigMapName,
			}
			got, err := f.buildOrchestratorExplorerConfigMap()
			if (err != nil) != tt.wantErr {
				t.Errorf("orchestratorExplorerFeature.buildOrchestratorExplorerConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("orchestratorExplorerFeature.buildOrchestratorExplorerConfigMap() = %#v,\nwant %#v", got, tt.want)
			}
		})
	}
}
