// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"

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
	crs := []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers"}
	type fields struct {
		enable                   bool
		runInClusterChecksRunner bool
		rbacSuffix               string
		serviceAccountName       string
		owner                    metav1.Object
		customConfig             *v2alpha1.CustomConfig
		configConfigMapName      string
		crCollection             []string
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
				runInClusterChecksRunner: false,
				configConfigMapName:      v2alpha1.DefaultOrchestratorExplorerConf,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), v2alpha1.DefaultOrchestratorExplorerConf, orchestratorExplorerCheckConfig(false, []string{})),
		},
		{
			name: "override",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configConfigMapName:      v2alpha1.DefaultOrchestratorExplorerConf,
				customConfig: &v2alpha1.CustomConfig{
					ConfigData: &overrideConf,
				},
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), v2alpha1.DefaultOrchestratorExplorerConf, overrideConf),
		}, {
			name: "default config with crs",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configConfigMapName:      v2alpha1.DefaultOrchestratorExplorerConf,
				crCollection:             crs,
			},
			want: buildDefaultConfigMap(owner.GetNamespace(), v2alpha1.DefaultOrchestratorExplorerConf, orchestratorExplorerCheckConfig(false, crs)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &orchestratorExplorerFeature{
				runInClusterChecksRunner: tt.fields.runInClusterChecksRunner,
				rbacSuffix:               tt.fields.rbacSuffix,
				serviceAccountName:       tt.fields.serviceAccountName,
				owner:                    tt.fields.owner,
				customConfig:             tt.fields.customConfig,
				configConfigMapName:      tt.fields.configConfigMapName,
				customResources:          tt.fields.crCollection,
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

func Test_orchestratorExplorerCheckConfig(t *testing.T) {
	crs := []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers"}

	got := orchestratorExplorerCheckConfig(false, crs)
	want := `---
cluster_check: false
ad_identifiers:
  - _kube_orchestrator
init_config:

instances:
  - skip_leader_election: false
    crd_collectors:
      - datadoghq.com/v1alpha1/datadogmetrics
      - datadoghq.com/v1alpha1/watermarkpodautoscalers
`
	require.Equal(t, want, got)
}
