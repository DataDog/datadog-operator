// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

// buildOrchestratorExplorerConfigMap constructs the ConfigMap for the orchestrator explorer.
//
// When remote config is disabled:
//   - If a custom ConfigMap is provided, no new ConfigMap is created.
//   - If custom config data is provided, a new ConfigMap is created using the parsed data.
//   - If no custom config is provided, a new ConfigMap is created using the default config.
//
// When remote config is enabled:
//   - If a custom ConfigMap is provided, a new ConfigMap is created by merging it with remote resources.
//     The generated ConfigMap will be used by the Cluster Agent.
//   - If custom config data is provided, a new ConfigMap is created by merging the data with remote resources.
//   - If no custom config is provided, a new ConfigMap is created using the default config merged with remote resources.
func (f *orchestratorExplorerFeature) buildOrchestratorExplorerConfigMap() (*corev1.ConfigMap, error) {
	switch {
	case f.customConfig != nil && f.customConfig.ConfigMap != nil:
		cm, err := f.buildConfigMapWithCustomConfig()
		if err != nil {
			f.logger.Error(err, "Unable to build ConfigMap from custom ConfigMap; using default.")
		}
		return cm, nil

	case f.customConfig != nil && f.customConfig.ConfigData != nil:
		cm, err := f.buildConfigMapWithConfigData()
		if err != nil && cm == nil {
			return nil, err // Fatal: failed to parse config data
		}
		if err != nil {
			f.logger.Error(err, "Unable to build ConfigMap from custom config data; using default.")
		}
		return cm, nil

	default:
		return buildDefaultConfigMap(
			f.owner.GetNamespace(),
			f.configConfigMapName,
			orchestratorExplorerCheckConfig(f.runInClusterChecksRunner, f.customResources),
		), nil
	}
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

// getConfigConfigMapName returns the name of the ConfigMap to use for the orchestrator explorer.
//
// - By default, if a custom config is provided, its ConfigMap name is used.
// - If remote config is enabled:
//   - The default ConfigMap name is used instead.
//   - If the custom ConfigMap name conflicts with the default name, a "-rc" suffix will be added to avoid naming collisions.
func (f *orchestratorExplorerFeature) getConfigConfigMapName() string {
	name := constants.GetConfName(f.owner, f.customConfig, defaultOrchestratorExplorerConf)

	if f.remoteConfigEnabled {
		name = constants.GetConfName(f.owner, nil, defaultOrchestratorExplorerConf)
		if f.customConfig != nil && f.customConfig.ConfigMap != nil &&
			name == f.customConfig.ConfigMap.Name {
			name = fmt.Sprintf("%s-rc", name)
		}
	}

	return name
}

// buildConfigMapWithConfigData creates a ConfigMap from the provided custom config data.
// Merges remote resources if remote config is enabled.
func (f *orchestratorExplorerFeature) buildConfigMapWithConfigData() (*corev1.ConfigMap, error) {
	ns := f.owner.GetNamespace()

	cm, err := configmap.BuildConfigMapConfigData(ns, f.customConfig.ConfigData, f.configConfigMapName, orchestratorExplorerConfFileName)
	if err != nil || !f.remoteConfigEnabled {
		return cm, err
	}

	merged, err := addCRToConfig(*f.customConfig.ConfigData, f.customResources)
	if err != nil {
		return cm, err
	}

	return configmap.BuildConfigMapConfigData(ns, &merged, f.configConfigMapName, orchestratorExplorerConfFileName)
}

// buildConfigMapWithCustomConfig creates a new ConfigMap by merging a user-provided custom ConfigMap
// with custom resources fetched from remote config.
//
// - If remote config is disabled, no new ConfigMap is created.
// - If the provided custom ConfigMap is invalid, a default ConfigMap is used instead.
// - If the custom ConfigMap is valid, a new ConfigMap is returned with the custom resources injected into the last entry.
func (f *orchestratorExplorerFeature) buildConfigMapWithCustomConfig() (*corev1.ConfigMap, error) {
	if !f.remoteConfigEnabled {
		return nil, nil
	}

	ns := f.owner.GetNamespace()
	defaultCM := buildDefaultConfigMap(ns, f.configConfigMapName, orchestratorExplorerCheckConfig(f.runInClusterChecksRunner, f.customResources))

	configs, instances, err := getAndValidateCustomConfig(f.k8sClient, ns, f.customConfig.ConfigMap.Name, f.runInClusterChecksRunner)
	if err != nil {
		return defaultCM, fmt.Errorf("unable to validate custom ConfigMap: %w", err)
	}

	lastFile := getLastKey(configs)
	merged, err := addCRToConfig(configs[lastFile], getUniqueCustomResources(f.customResources, instances))
	if err != nil {
		return defaultCM, err
	}
	configs[lastFile] = merged

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.configConfigMapName,
			Namespace: ns,
		},
		Data: configs,
	}, nil
}

type orchestratorInstance struct {
	LeaderSkip              bool     `json:"skip_leader_election,omitempty"`
	Collectors              []string `json:"collectors,omitempty"`
	CRDCollectors           []string `json:"crd_collectors,omitempty"`
	ExtraSyncTimeoutSeconds int      `json:"extra_sync_timeout_seconds,omitempty"`
}

// getAndValidateCustomConfig fetches and validates the user-provided ConfigMap.
// Parses its entries into orchestrator instances.
func getAndValidateCustomConfig(k8sClient client.Client, namespace, cmName string, clusterCheckRunners bool) (map[string]string, []orchestratorInstance, error) {
	cm := &corev1.ConfigMap{}
	err := k8sClient.Get(context.Background(), client.ObjectKey{Name: cmName, Namespace: namespace}, cm)
	if err != nil {
		return nil, nil, fmt.Errorf("ConfigMap %q not found in namespace %q", cmName, namespace)
	}

	if len(cm.Data) == 0 {
		return nil, nil, fmt.Errorf("ConfigMap %q is empty", cmName)
	}
	if !clusterCheckRunners && len(cm.Data) != 1 {
		return nil, nil, fmt.Errorf("ConfigMap %q must contain exactly one entry if cluster checks are disabled", cmName)
	}

	instances := make([]orchestratorInstance, 0, len(cm.Data))
	for _, content := range cm.Data {
		_, inst, err := parseConfig(content)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse config: %w", err)
		}
		instances = append(instances, inst)
	}

	return cm.Data, instances, nil
}

// getUniqueCustomResources filters out CRDs already present in the provided config instances.
func getUniqueCustomResources(customResources []string, instances []orchestratorInstance) []string {
	seen := map[string]struct{}{}
	for _, inst := range instances {
		for _, cr := range inst.CRDCollectors {
			seen[cr] = struct{}{}
		}
	}

	var unique []string
	for _, cr := range customResources {
		if _, found := seen[cr]; !found {
			unique = append(unique, cr)
		}
	}
	return unique
}

// parseConfig unmarshals a single orchestrator instance from the given config content.
func parseConfig(content string) (map[string]interface{}, orchestratorInstance, error) {
	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil, orchestratorInstance{}, err
	}

	rawInstances, ok := config["instances"]
	if !ok {
		return nil, orchestratorInstance{}, fmt.Errorf("missing 'instances' section")
	}

	instList, ok := rawInstances.([]interface{})
	if !ok || len(instList) != 1 {
		return nil, orchestratorInstance{}, fmt.Errorf("'instances' must contain exactly one entry")
	}

	instData, err := yaml.Marshal(instList[0])
	if err != nil {
		return nil, orchestratorInstance{}, err
	}

	var instance orchestratorInstance
	if err := yaml.Unmarshal(instData, &instance); err != nil {
		return nil, orchestratorInstance{}, err
	}

	return config, instance, nil
}

// addCRToConfig merges custom resources into the orchestrator config.
func addCRToConfig(content string, crs []string) (string, error) {
	config, instance, err := parseConfig(content)
	if err != nil {
		return content, fmt.Errorf("unable to parse config: %w", err)
	}

	instance.CRDCollectors = append(instance.CRDCollectors, crs...)
	config["instances"] = []interface{}{instance}

	output, err := yaml.Marshal(config)
	if err != nil {
		return content, fmt.Errorf("unable to marshal config: %w", err)
	}
	return string(output), nil
}

// getLastKey returns the last key from a sorted map.
func getLastKey(data map[string]string) string {
	if len(data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[len(keys)-1]
}
