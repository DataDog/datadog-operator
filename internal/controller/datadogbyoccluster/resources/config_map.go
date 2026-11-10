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

	"dario.cat/mergo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	quickwitCloudPremConfigKey                   = "cloudprem"
	quickwitCacheCapacityConfigKey               = "capacity"
	quickwitFastFieldCacheConfigKey              = "fast_field_cache"
	quickwitFastFieldCacheCapacityConfigKey      = "fast_field_cache_capacity"
	quickwitPartialRequestCacheConfigKey         = "partial_request_cache"
	quickwitPartialRequestCacheCapacityConfigKey = "partial_request_cache_capacity"
	quickwitSplitFooterCacheConfigKey            = "split_footer_cache"
	quickwitSplitFooterCacheCapacityConfigKey    = "split_footer_cache_capacity"
	quickwitCreateLogsIndexConfigKey             = "create_dd_logs_index"
	quickwitCreateDatadogIndexConfigKey          = "create_datadog_index"
	quickwitUseReadOnlyMetastoreConfigKey        = "use_metastore_read_replica"
	quickwitClusterIDConfigKey                   = "cluster_id"
)

// newNodeConfig returns the Quickwit node configuration with spec.nodeConfigOverrides merged into the defaults.
func newNodeConfig(cluster *datadoghqv1alpha1.DatadogBYOCCluster) (map[string]any, error) {
	indexerResources := cluster.Spec.Components.Indexer.Resources
	searcherMemoryLimit := cluster.Spec.Components.Searcher.Resources.Limits[corev1.ResourceMemory]
	maxQueueMemoryUsage := calculateMaxQueueMemoryUsage(indexerResources)
	maxQueueDiskUsage := maxQueueMemoryUsage * 2
	searcherGiB := sizingGiB(searcherMemoryLimit.Value())

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
	config[quickwitClusterIDConfigKey] = cluster.Namespace + "-" + cluster.Name
	config["ingest_api"] = map[string]any{
		// ByteSize accepts integer byte counts, so no unit conversion is needed.
		"max_queue_disk_usage":   maxQueueDiskUsage,
		"max_queue_memory_usage": maxQueueMemoryUsage,
	}
	// The searcher sizing follows the CloudPrem Helm chart sizing map, which scales with the memory in
	// whole GiB and writes cache capacities in decimal units (G = 10^9 bytes).
	searcher := config[quickwitSearcherServiceName].(map[string]any)
	searcher["max_num_concurrent_split_searches"] = int64(math.Ceil(searcherGiB * 3.125))
	if !overrideSets(override, []string{quickwitSearcherServiceName, quickwitFastFieldCacheCapacityConfigKey}) {
		searcher[quickwitFastFieldCacheConfigKey] = map[string]any{quickwitCacheCapacityConfigKey: int64(searcherGiB * 406_250_000)}
	}
	if !overrideSets(override, []string{quickwitSearcherServiceName, quickwitPartialRequestCacheCapacityConfigKey}) {
		searcher[quickwitPartialRequestCacheConfigKey] = map[string]any{quickwitCacheCapacityConfigKey: int64(searcherGiB * 15_625_000)}
	}
	if !overrideSets(override, []string{quickwitSearcherServiceName, quickwitSplitFooterCacheCapacityConfigKey}) {
		searcher[quickwitSplitFooterCacheConfigKey] = map[string]any{quickwitCacheCapacityConfigKey: int64(searcherGiB * 31_250_000)}
	}
	if cluster.Spec.Components.ReadOnlyMetastore != nil {
		searcher[quickwitUseReadOnlyMetastoreConfigKey] = true
	}
	signal := datadoghqv1alpha1.DatadogBYOCClusterSignalLogs
	if cluster.Spec.Signal != nil {
		signal = *cluster.Spec.Signal
	}
	// Set both keys explicitly: CloudPrem treats a missing create_dd_logs_index as true.
	cloudprem := config[quickwitCloudPremConfigKey].(map[string]any)
	if !overrideSets(override, []string{quickwitCloudPremConfigKey, quickwitCreateDatadogIndexConfigKey}) {
		cloudprem[quickwitCreateLogsIndexConfigKey] = signal == datadoghqv1alpha1.DatadogBYOCClusterSignalLogs
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
	return config, nil
}

func newConfigMap(cluster *datadoghqv1alpha1.DatadogBYOCCluster, nodeConfig map[string]any) (*corev1.ConfigMap, error) {
	data, err := yaml.Marshal(nodeConfig)
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
		Data: map[string]string{nodeConfigFileName: strings.TrimSuffix(string(data), "\n")},
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

// calculateMaxQueueMemoryUsage returns the indexer WAL memory budget. The CloudPrem Helm chart sizing map
// tuned it with overload tests to 0.8, 2.4, 7.2 and 21.6 GiB for 2, 4, 8 and 16 cores: it triples each
// time the cores double. The cores are the CPU request, or the limit, rounded up to a whole core, as the
// chart rounds its indexer CPU limit. The budget is capped at 40% of the memory limit, above the chart's
// largest ratio of 37%, so that a pod with many cores and little memory keeps memory for indexing.
func calculateMaxQueueMemoryUsage(resources *corev1.ResourceRequirements) int64 {
	cpu, found := resources.Requests[corev1.ResourceCPU]
	if !found {
		cpu = resources.Limits[corev1.ResourceCPU]
	}
	cores := max(1, (cpu.MilliValue()+999)/1000)
	memoryLimit := resources.Limits[corev1.ResourceMemory]
	budgetBytes := int64(0.8 * bytesPerGiB * math.Pow(3, math.Log2(float64(cores)/2)))
	return min(budgetBytes, memoryLimit.Value()*2/5)
}

// sizingGiB returns the memory rounded down to a whole GiB, as the CloudPrem Helm chart sizing map does.
// Memory under 1 GiB keeps its fraction so that the caches are not sized to zero.
func sizingGiB(memoryBytes int64) float64 {
	if memoryBytes < bytesPerGiB {
		return float64(memoryBytes) / bytesPerGiB
	}
	return float64(memoryBytes / bytesPerGiB)
}

func calculateSplitStoreMaxNumBytes(capacity resource.Quantity, maxQueueDiskUsage int64) int64 {
	capacityBytes := capacity.Value()
	// Divide before multiplying to avoid overflowing large PVC capacities.
	budgetBytes := capacityBytes/10*7 + capacityBytes%10*7/10
	// The split store is a local cache; skip caching when no disk budget remains.
	return max(0, budgetBytes-maxQueueDiskUsage)
}
