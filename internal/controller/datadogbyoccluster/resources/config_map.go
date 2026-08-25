// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"
	"strings"

	"github.com/imdario/mergo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const nodeConfigFileName = "node.yaml"

type configMapBuilder struct {
	cluster *datadoghqv1alpha1.DatadogBYOCCluster
}

func newConfigMapBuilder(cluster *datadoghqv1alpha1.DatadogBYOCCluster) configMapBuilder {
	return configMapBuilder{cluster: cluster}
}

func (b configMapBuilder) build() (*corev1.ConfigMap, error) {
	config := map[string]interface{}{
		"version":               0.8,
		"listen_address":        "0.0.0.0",
		"gossip_listen_port":    7282,
		"cloudprem_listen_port": 7283,
		"data_dir":              "/quickwit/qwdata",
		"grpc":                  map[string]interface{}{"keep_alive": map[string]interface{}{"interval": "30s", "timeout": "10s"}},
		"health":                map[string]interface{}{"listen_port": 7284},
		"cloudprem": map[string]interface{}{
			"mtls_header":              "X-Amzn-Mtls-Clientcert",
			"create_dd_logs_index":     true,
			"create_dd_metrics_index":  false,
			"create_dd_sketches_index": false,
			"create_dd_traces_index":   false,
		},
		"docs_clustering": []interface{}{
			map[string]interface{}{"fingerprint": []interface{}{map[string]interface{}{"kind": "structure"}}},
			map[string]interface{}{"fingerprint": []interface{}{map[string]interface{}{"kind": "raw", "path": "source"}}},
			map[string]interface{}{"fingerprint": []interface{}{map[string]interface{}{"kind": "raw", "path": "status"}}},
			map[string]interface{}{"fingerprint": []interface{}{map[string]interface{}{"kind": "tokenized", "path": "message"}}},
		},
		"indexer":    map[string]interface{}{"split_store_max_num_bytes": "200G", "split_store_max_num_splits": 10000},
		"ingest_api": map[string]interface{}{"max_queue_disk_usage": "7.2GiB", "max_queue_memory_usage": "3.6GiB"},
		"searcher": map[string]interface{}{
			"aggregation_memory_limit":          "500M",
			"fast_field_cache_capacity":         "4.875G",
			"max_num_concurrent_split_searches": 40,
			"partial_request_cache_capacity":    "187.5M",
			"split_footer_cache_capacity":       "375M",
		},
	}
	if b.cluster.Spec.Components.ReadOnlyMetastore != nil {
		config["searcher"].(map[string]interface{})["use_metastore_read_replica"] = true
	}
	if b.cluster.Spec.NodeConfig != nil && len(b.cluster.Spec.NodeConfig.Raw) != 0 {
		var override map[string]interface{}
		if err := yaml.Unmarshal(b.cluster.Spec.NodeConfig.Raw, &override); err != nil {
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
