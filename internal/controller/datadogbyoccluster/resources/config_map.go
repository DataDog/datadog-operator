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

type configMapBuilder struct {
	cluster *datadoghqv1alpha1.DatadogBYOCCluster
}

func newConfigMapBuilder(cluster *datadoghqv1alpha1.DatadogBYOCCluster) configMapBuilder {
	return configMapBuilder{cluster: cluster}
}

func (b configMapBuilder) build() (*corev1.ConfigMap, error) {
	indexerMemoryLimit := b.cluster.Spec.Components.Indexer.Resources.Limits[corev1.ResourceMemory]
	searcherMemoryLimit := b.cluster.Spec.Components.Searcher.Resources.Limits[corev1.ResourceMemory]
	indexerMemoryBytes := indexerMemoryLimit.Value()
	searcherMemoryBytes := searcherMemoryLimit.Value()
	maxQueueDiskUsage := indexerMemoryBytes * 3 / 5

	config := map[string]any{
		"version":               0.8,
		"listen_address":        "0.0.0.0",
		"gossip_listen_port":    gossipPort,
		"cloudprem_listen_port": cloudpremPort,
		"data_dir":              defaultDataPath,
		"grpc":                  map[string]any{"keep_alive": map[string]any{"interval": "30s", "timeout": "10s"}},
		"health":                map[string]any{"listen_port": healthPort},
		"cloudprem": map[string]any{
			"mtls_header":              "X-Amzn-Mtls-Clientcert",
			"create_dd_logs_index":     true,
			"create_dd_metrics_index":  false,
			"create_dd_sketches_index": false,
			"create_dd_traces_index":   false,
		},
		"docs_clustering": []any{
			map[string]any{"fingerprint": []any{map[string]any{"kind": "structure"}}},
			map[string]any{"fingerprint": []any{map[string]any{"kind": "raw", "path": "source"}}},
			map[string]any{"fingerprint": []any{map[string]any{"kind": "raw", "path": "status"}}},
			map[string]any{"fingerprint": []any{map[string]any{"kind": "tokenized", "path": "message"}}},
		},
		quickwitIndexerServiceName: map[string]any{"split_store_max_num_splits": 10000},
		"ingest_api": map[string]any{
			// ByteSize accepts integer byte counts, so no unit conversion is needed.
			"max_queue_disk_usage":   maxQueueDiskUsage,
			"max_queue_memory_usage": indexerMemoryBytes * 3 / 10,
		},
		quickwitSearcherServiceName: map[string]any{
			"aggregation_memory_limit":          "500M",
			"fast_field_cache_capacity":         searcherMemoryBytes * 13 / 32,
			"max_num_concurrent_split_searches": int64(math.Ceil(float64(searcherMemoryBytes) / bytesPerGiB * 3.125)),
			"partial_request_cache_capacity":    searcherMemoryBytes / 64,
			"split_footer_cache_capacity":       searcherMemoryBytes / 32,
		},
	}
	if b.cluster.Spec.Components.ReadOnlyMetastore != nil {
		config[quickwitSearcherServiceName].(map[string]any)[quickwitUseReadOnlyMetastoreConfigKey] = true
	}
	storage := b.cluster.Spec.Components.Indexer.Storage
	if storage != nil && storage.VolumeClaimTemplate != nil {
		indexer := config[quickwitIndexerServiceName].(map[string]any)
		capacity := storage.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage]
		indexer["split_store_max_num_bytes"] = calculateSplitStoreMaxNumBytes(capacity, maxQueueDiskUsage)
	}
	if b.cluster.Spec.NodeConfig != nil && len(b.cluster.Spec.NodeConfig.Raw) != 0 {
		var override map[string]any
		if err := yaml.Unmarshal(b.cluster.Spec.NodeConfig.Raw, &override, func(d *json.Decoder) *json.Decoder {
			d.UseNumber()
			return d
		}); err != nil {
			return nil, fmt.Errorf("decode spec.nodeConfig: %w", err)
		}
		if err := mergo.Merge(&config, override, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("merge spec.nodeConfig: %w", err)
		}
	}
	nodeConfig, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode node config: %w", err)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        b.cluster.Name,
			Namespace:   b.cluster.Namespace,
			Labels:      labels(b.cluster),
			Annotations: annotations(b.cluster),
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
