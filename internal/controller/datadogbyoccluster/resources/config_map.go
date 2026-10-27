// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/imdario/mergo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func newConfigMap(cluster *datadoghqv1alpha1.DatadogBYOCCluster) (*corev1.ConfigMap, error) {
	indexerMemoryLimit := cluster.Spec.Components.Indexer.Resources.Limits[corev1.ResourceMemory]
	searcherMemoryLimit := cluster.Spec.Components.Searcher.Resources.Limits[corev1.ResourceMemory]
	indexerMemoryBytes := indexerMemoryLimit.Value()
	searcherMemoryBytes := searcherMemoryLimit.Value()
	maxQueueDiskUsage := indexerMemoryBytes * 3 / 5

	var override map[string]any
	if cluster.Spec.NodeConfigOverrides != nil && len(cluster.Spec.NodeConfigOverrides.Raw) != 0 {
		if err := yaml.Unmarshal(cluster.Spec.NodeConfigOverrides.Raw, &override, func(d *json.Decoder) *json.Decoder {
			d.UseNumber()
			return d
		}); err != nil {
			return nil, fmt.Errorf("decode spec.nodeConfigOverrides: %w", err)
		}
	}

	config := defaultNodeConfig()
	config["ingest_api"] = map[string]any{
		// ByteSize accepts integer byte counts, so no unit conversion is needed.
		"max_queue_disk_usage":   maxQueueDiskUsage,
		"max_queue_memory_usage": indexerMemoryBytes * 3 / 10,
	}
	searcher := map[string]any{
		"aggregation_memory_limit":          "500M",
		"max_num_concurrent_split_searches": int64(math.Ceil(float64(searcherMemoryBytes) / bytesPerGiB * 3.125)),
	}
	if !overrideSets(override, []string{quickwitSearcherServiceName, "fast_field_cache_capacity"}) {
		searcher["fast_field_cache"] = map[string]any{"capacity": searcherMemoryBytes * 13 / 32}
	}
	if !overrideSets(override, []string{quickwitSearcherServiceName, "partial_request_cache_capacity"}) {
		searcher["partial_request_cache"] = map[string]any{"capacity": searcherMemoryBytes / 64}
	}
	if !overrideSets(override, []string{quickwitSearcherServiceName, "split_footer_cache_capacity"}) {
		searcher["split_footer_cache"] = map[string]any{"capacity": searcherMemoryBytes / 32}
	}
	if cluster.Spec.Components.ReadOnlyMetastore != nil {
		searcher[quickwitUseReadOnlyMetastoreConfigKey] = true
	}
	config[quickwitSearcherServiceName] = searcher
	signal := datadoghqv1alpha1.DatadogBYOCClusterSignalLogs
	if cluster.Spec.Signal != nil {
		signal = *cluster.Spec.Signal
	}
	// Set both keys explicitly: CloudPrem treats a missing create_dd_logs_index as true.
	cloudprem := config["cloudprem"].(map[string]any)
	if !overrideSets(override, []string{"cloudprem", "create_datadog_index"}) {
		cloudprem["create_dd_logs_index"] = signal == datadoghqv1alpha1.DatadogBYOCClusterSignalLogs
	}
	cloudprem["create_dd_traces_index"] = signal == datadoghqv1alpha1.DatadogBYOCClusterSignalTraces
	storage := cluster.Spec.Components.Indexer.Storage
	if storage != nil && storage.VolumeClaimTemplate != nil {
		indexer := config[quickwitIndexerServiceName].(map[string]any)
		capacity := storage.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage]
		indexer["split_store_max_num_bytes"] = calculateSplitStoreMaxNumBytes(capacity, maxQueueDiskUsage)
	}
	if override != nil {
		if err := mergo.Merge(&config, override, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("merge spec.nodeConfigOverrides: %w", err)
		}
	}
	nodeConfig, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode node config: %w", err)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cluster.Name,
			Namespace:   cluster.Namespace,
			Labels:      labels(cluster),
			Annotations: annotations(cluster),
		},
		Data: map[string]string{nodeConfigFileName: strings.TrimSuffix(string(nodeConfig), "\n")},
	}, nil
}

// overrideSets reports whether the override sets any of paths. The operator does not set a key that
// the override sets under a serde alias (quickwit-config/src/node_config/mod.rs), because Pomsky
// rejects a key set twice. Merging the override replaces a key it sets under the same name.
func overrideSets(override map[string]any, paths ...[]string) bool {
	return slices.ContainsFunc(paths, func(path []string) bool {
		_, found, _ := unstructured.NestedFieldNoCopy(override, path...)
		return found
	})
}

func calculateSplitStoreMaxNumBytes(capacity resource.Quantity, maxQueueDiskUsage int64) int64 {
	capacityBytes := capacity.Value()
	// Divide before multiplying to avoid overflowing large PVC capacities.
	budgetBytes := capacityBytes/10*7 + capacityBytes%10*7/10
	// The split store is a local cache; skip caching when no disk budget remains.
	return max(0, budgetBytes-maxQueueDiskUsage)
}
