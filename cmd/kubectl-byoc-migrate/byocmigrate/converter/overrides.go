// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package converter

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// applyOverrides sets the environment variables, the probes, and the node configuration of the chart that the
// controllers would not set to the same value. Variables are set per component, because global variables also apply
// to the pipelines.
func applyOverrides(cluster *datadoghqv1alpha1.DatadogBYOCCluster, cloudprem Manifest, pipelines []PipelineManifest) error {
	result, err := render(cluster)
	if err != nil {
		return err
	}
	for _, ref := range componentRefs(cluster.Spec.Components) {
		chart, _ := chartPodTemplate(cloudprem.Objects, ref.chart)
		controller, found := controllerPodTemplate(result.cluster, ref.controller)
		if !found {
			return fmt.Errorf("the controller renders no %s workload", ref.controller)
		}
		chartContainer, controllerContainer := chart.Spec.Containers[0], controller.Spec.Containers[0]
		ref.spec.Env = envOverrides(chartContainer.Env, controllerContainer.Env)
		ref.spec.StartupProbe, ref.spec.LivenessProbe, ref.spec.ReadinessProbe = probeOverrides(chartContainer, controllerContainer)
	}
	for i, pipeline := range pipelines {
		chart, _ := findObject(pipeline.Manifest.Objects, func(*appsv1.StatefulSet) bool { return true })
		controller := result.workerResources[pipeline.Pipeline].StatefulSet
		chartContainer, controllerContainer := chart.Spec.Template.Spec.Containers[0], controller.Spec.Template.Spec.Containers[0]
		spec := &cluster.Spec.Components.Pipelines[i]
		spec.Env = envOverrides(chartContainer.Env, controllerContainer.Env)
		spec.StartupProbe, spec.LivenessProbe, spec.ReadinessProbe = probeOverrides(chartContainer, controllerContainer)
	}

	chartConfig, found, err := chartNodeConfig(cloudprem, cluster.Name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("manifest %s has no ConfigMap %s with %s", cloudprem.Name, cluster.Name, nodeConfigFileName)
	}
	controllerConfig, _ := nodeConfig(result.cluster.Objects(), cluster.Name)
	cluster.Spec.NodeConfigOverrides, err = nodeConfigOverrides(chartConfig, controllerConfig)
	return err
}

// chartNodeConfig returns the node configuration of the chart with cluster_id set to the QW_CLUSTER_ID of the indexer,
// which takes precedence over it. The chart sets QW_CLUSTER_ID unless its values set cluster_id, but the controller
// sets cluster_id, so the converter compares the cluster IDs in the node configuration.
func chartNodeConfig(cloudprem Manifest, name string) (string, bool, error) {
	config, found := nodeConfig(cloudprem.Objects, name)
	if !found {
		return "", false, nil
	}
	indexer, _, err := indexerOf(cloudprem)
	if err != nil {
		return "", false, err
	}
	container, err := firstContainer(indexer)
	if err != nil {
		return "", false, err
	}
	clusterID := envValue(container.Env, envQuickwitClusterID)
	if clusterID == nil {
		return config, true, nil
	}
	var values map[string]any
	if err = yaml.Unmarshal([]byte(config), &values); err != nil {
		return "", false, fmt.Errorf("decode the node configuration of the chart: %w", err)
	}
	values[nodeConfigClusterIDKey] = *clusterID
	encoded, err := yaml.Marshal(values)
	if err != nil {
		return "", false, fmt.Errorf("encode the node configuration of the chart: %w", err)
	}
	return string(encoded), true, nil
}

func nodeConfig(objects []client.Object, name string) (string, bool) {
	configMap, found := findObject(objects, func(configMap *corev1.ConfigMap) bool { return configMap.Name == name })
	if !found {
		return "", false
	}
	config, found := configMap.Data[nodeConfigFileName]
	return config, found
}
