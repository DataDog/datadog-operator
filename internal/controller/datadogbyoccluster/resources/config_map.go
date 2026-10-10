// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/imdario/mergo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func newConfigMap(cluster *datadoghqv1alpha1.DatadogBYOCCluster) (*corev1.ConfigMap, error) {
	indexerMemoryLimit := cluster.Spec.Components.Indexer.Resources.Limits[corev1.ResourceMemory]
	searcherMemoryLimit := cluster.Spec.Components.Searcher.Resources.Limits[corev1.ResourceMemory]
	indexerMemoryBytes := indexerMemoryLimit.Value()
	searcherMemoryBytes := searcherMemoryLimit.Value()
	maxQueueDiskUsage := indexerMemoryBytes * 3 / 5

	config := defaultNodeConfig()
	config["ingest_api"] = map[string]any{
		// ByteSize accepts integer byte counts, so no unit conversion is needed.
		"max_queue_disk_usage":   maxQueueDiskUsage,
		"max_queue_memory_usage": indexerMemoryBytes * 3 / 10,
	}
	searcher := config[quickwitSearcherServiceName].(map[string]any)
	searcher["fast_field_cache_capacity"] = searcherMemoryBytes * 13 / 32
	searcher["max_num_concurrent_split_searches"] = int64(math.Ceil(float64(searcherMemoryBytes) / bytesPerGiB * 3.125))
	searcher["partial_request_cache_capacity"] = searcherMemoryBytes / 64
	searcher["split_footer_cache_capacity"] = searcherMemoryBytes / 32
	if cluster.Spec.Components.ReadOnlyMetastore != nil {
		searcher[quickwitUseReadOnlyMetastoreConfigKey] = true
	}
	if cluster.Spec.Type != nil && *cluster.Spec.Type == datadoghqv1alpha1.DatadogBYOCClusterTypeTraces {
		config["cloudprem"].(map[string]any)["create_dd_traces_index"] = true
	}
	storage := cluster.Spec.Components.Indexer.Storage
	if storage != nil && storage.VolumeClaimTemplate != nil {
		indexer := config[quickwitIndexerServiceName].(map[string]any)
		capacity := storage.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage]
		indexer["split_store_max_num_bytes"] = calculateSplitStoreMaxNumBytes(capacity, maxQueueDiskUsage)
	}
	if cluster.Spec.NodeConfigOverrides != nil && len(cluster.Spec.NodeConfigOverrides.Raw) != 0 {
		var override map[string]any
		if err := yaml.Unmarshal(cluster.Spec.NodeConfigOverrides.Raw, &override, func(d *json.Decoder) *json.Decoder {
			d.UseNumber()
			return d
		}); err != nil {
			return nil, fmt.Errorf("decode spec.nodeConfigOverrides: %w", err)
		}
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

func calculateSplitStoreMaxNumBytes(capacity resource.Quantity, maxQueueDiskUsage int64) int64 {
	capacityBytes := capacity.Value()
	// Divide before multiplying to avoid overflowing large PVC capacities.
	budgetBytes := capacityBytes/10*7 + capacityBytes%10*7/10
	// The split store is a local cache; skip caching when no disk budget remains.
	return max(0, budgetBytes-maxQueueDiskUsage)
}
