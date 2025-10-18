// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *helmCheckFeature) buildHelmCheckConfigMap() (*corev1.ConfigMap, error) {
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

// Helm check should be configured as a cluster check only when there are Cluster Check
// Runners deployed.
// This check is not designed to work on the DaemonSet Agent. That's why when
// cluster checks are enabled but without Cluster Check Runners, we don't want
// to set this check as a cluster check, because then it would be scheduled in
// the DaemonSet agent instead of the DCA.
func helmCheckConfig(clusterCheck bool, collectEvents bool, valuesAsTags map[string]string) string {
	clusterChecksVal := strconv.FormatBool(clusterCheck)
	collectEventsVal := strconv.FormatBool(collectEvents)
	sortedTagsKeys := sortTagsKeys(valuesAsTags)
	config := fmt.Sprintf(`---
cluster_check: %s
init_config:
instances:
  - collect_events: %s
`, clusterChecksVal, collectEventsVal)

	if len(valuesAsTags) > 0 {
		config += "    helm_values_as_tags:\n"
		for _, key := range sortedTagsKeys {
			config += fmt.Sprintf("      %s: %s\n", key, valuesAsTags[key])
		}
	}

	return config
}

func sortTagsKeys(tags map[string]string) []string {
	keys := make([]string, 0, len(tags))

	for k := range tags {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
