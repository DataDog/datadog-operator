// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"fmt"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

func (f *helmCheckFeature) buildHelmCheckConfigMap() (*corev1.ConfigMap, error) {
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		return nil, nil
	}
	if f.customConfig != nil && f.customConfig.ConfigData != nil {
		return configmap.BuildConfigMapConfigData(f.owner.GetNamespace(), f.customConfig.ConfigData, f.configMapName, helmCheckConfFileName)
	}

	configMap := buildDefaultConfigMap(f.owner.GetNamespace(), f.configMapName, helmCheckConfig(f.runInClusterChecksRunner, f.collectEvents, f.valuesAsTags))
	return configMap, nil
}

func buildDefaultConfigMap(namespace, cmName string, content string) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
		},
		Data: map[string]string{
			helmCheckConfFileName: content,
		},
	}
	return configMap
}

func helmCheckConfig(clusterCheck bool, collectEvents bool, valuesAsTags map[string]string) string {
	clusterChecksVal := strconv.FormatBool(clusterCheck)
	collectEventsVal := strconv.FormatBool(collectEvents)
	config := fmt.Sprintf(`---
cluster_check: %s
init_config:
instances:
  - collect_events: %s
`, clusterChecksVal, collectEventsVal)

	if len(valuesAsTags) > 0 {
		config += "    helm_values_as_tags:\n"
		for key, val := range valuesAsTags {
			config += fmt.Sprintf("      %s: %s\n", key, val)
		}
	}

	return config
}
