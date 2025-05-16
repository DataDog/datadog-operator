// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/object/configmap"
)

func (f *orchestratorExplorerFeature) buildOrchestratorExplorerConfigMap() (*corev1.ConfigMap, error) {
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		return nil, nil
	}
	if f.customConfig != nil && f.customConfig.ConfigData != nil {
		return configmap.BuildConfigMapConfigData(f.owner.GetNamespace(), f.customConfig.ConfigData, f.configConfigMapName, orchestratorExplorerConfFileName)
	}

	configMap := buildDefaultConfigMap(f.owner.GetNamespace(), f.configConfigMapName, orchestratorExplorerCheckConfig(f.runInClusterChecksRunner, f.customResources))
	return configMap, nil
}

func buildDefaultConfigMap(namespace, cmName string, content string) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
		},
		Data: map[string]string{
			orchestratorExplorerConfFileName: content,
		},
	}
	return configMap
}

func orchestratorExplorerCheckConfig(clusterCheckRunners bool, crs []string) string {
	stringClusterCheckRunners := strconv.FormatBool(clusterCheckRunners)
	config := fmt.Sprintf(`---
cluster_check: %s
ad_identifiers:
  - _kube_orchestrator
init_config:

instances:
  - skip_leader_election: %s
`, stringClusterCheckRunners, stringClusterCheckRunners)

	if len(crs) > 0 {
		config = config + "    crd_collectors:\n"
		for _, cr := range crs {
			config = config + fmt.Sprintf("      - %s\n", cr)
		}
	}

	return config
}
