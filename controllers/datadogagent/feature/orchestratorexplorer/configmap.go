// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *orchestratorExplorerFeature) buildOrchestratorExplorerConfigMap() (*corev1.ConfigMap, error) {
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		return nil, nil
	}
	if f.customConfig != nil && f.customConfig.ConfigData != nil {
		return configmap.BuildConfiguration(f.owner.GetNamespace(), f.customConfig.ConfigData, f.configConfigMapName, orchestratorExplorerCheckName)
	}

	configMap := buildDefaultConfigMap(f.owner.GetNamespace(), f.configConfigMapName, orchestratorExplorerCheckConfig(f.clusterChecksEnabled))
	return configMap, nil
}

func buildDefaultConfigMap(namespace, cmName string, content string) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
		},
		Data: map[string]string{
			orchestratorExplorerCheckName: content,
		},
	}
	return configMap
}

func orchestratorExplorerCheckConfig(clusterCheck bool) string {
	stringClusterCheck := strconv.FormatBool(clusterCheck)
	return fmt.Sprintf(`---
cluster_check: %s
ad_identifiers:
  - _kube_orchestrator
init_config:
instances:
  - skip_leader_election: %s
`, stringClusterCheck, stringClusterCheck)
}
