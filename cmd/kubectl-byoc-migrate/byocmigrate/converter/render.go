// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package converter

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocdefaults "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/defaults"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
	byocresources "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/resources"
	workerdefaults "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/defaults"
	workerresources "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/resources"
)

const (
	nodeConfigFileName     = "node.yaml"
	nodeConfigClusterIDKey = "cluster_id"
	envQuickwitClusterID   = "QW_CLUSTER_ID"
)

// pomskyNodeConfigDefaults holds the values that Pomsky uses for the node configuration keys that older charts omit
// but the controller sets, so that the converted spec keeps the value the cluster runs with. The values are float64,
// like the numbers of decoded YAML.
var pomskyNodeConfigDefaults = map[string]any{"gossip_protocol_version": float64(0)}

// operatorOwnedEnv describe the chart or the image, so the controllers set them even when the chart value differs.
var operatorOwnedEnv = []string{"OTEL_RESOURCE_ATTRIBUTES", "IMAGE_NAME", "IMAGE_TAG"}

// rendered holds the resources that the controllers create for a cluster.
type rendered struct {
	cluster *byocresources.Resources
	// workers holds the Worker of each pipeline, and workerResources the resources that the Worker controller creates.
	workers         map[string]*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker
	workerResources map[string]*workerresources.Resources
	defaulted       *datadoghqv1alpha1.DatadogBYOCCluster
}

// render builds the resources that the controllers create for the cluster.
func render(cluster *datadoghqv1alpha1.DatadogBYOCCluster) (*rendered, error) {
	defaulted := byocdefaults.Apply(withCRDDefaults(cluster))
	images := byocimage.Resolve(defaulted.Spec.ImageOverrides)
	resources, err := byocresources.BuildResources(defaulted, images)
	if err != nil {
		return nil, fmt.Errorf("build the resources of the converted cluster: %w", err)
	}
	result := &rendered{
		cluster:         resources,
		workers:         map[string]*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{},
		workerResources: map[string]*workerresources.Resources{},
		defaulted:       defaulted,
	}
	for i := range defaulted.Spec.Components.Pipelines {
		pipeline := &defaulted.Spec.Components.Pipelines[i]
		worker, err := byocresources.BuildObservabilityPipelinesWorker(defaulted, pipeline, images.ObservabilityPipelinesWorker)
		if err != nil {
			return nil, fmt.Errorf("build the worker of pipeline %s: %w", pipeline.Name, err)
		}
		workerResources, err := workerresources.BuildResources(workerdefaults.Apply(worker))
		if err != nil {
			return nil, fmt.Errorf("build the resources of pipeline %s: %w", pipeline.Name, err)
		}
		result.workers[pipeline.Name] = worker
		result.workerResources[pipeline.Name] = workerResources
	}
	return result, nil
}

// withCRDDefaults applies the defaults that the API server sets from the CRD schema.
func withCRDDefaults(cluster *datadoghqv1alpha1.DatadogBYOCCluster) *datadoghqv1alpha1.DatadogBYOCCluster {
	cluster = cluster.DeepCopy()
	datadog := cluster.Spec.Datadog
	if datadog.Site == nil {
		datadog.Site = new("datadoghq.com")
	}
	if datadog.BYOCTelemetry == nil {
		datadog.BYOCTelemetry = new(true)
	}
	if datadog.DogstatsdServer == nil {
		datadog.DogstatsdServer = &datadoghqv1alpha1.DatadogBYOCClusterDogstatsdServerSpec{}
	}
	if datadog.DogstatsdServer.Port == nil {
		datadog.DogstatsdServer.Port = new(int32(8125))
	}
	return cluster
}

// envOverrides returns the variables of the chart that the controller does not set to the same value, so that the
// converted spec keeps them. A literal QW_CLUSTER_ID is compared as cluster_id of the node configuration, as
// chartNodeConfig does.
func envOverrides(chart, controller []corev1.EnvVar) []corev1.EnvVar {
	var overrides []corev1.EnvVar
	for _, variable := range chart {
		if slices.Contains(operatorOwnedEnv, variable.Name) || (variable.Name == envQuickwitClusterID && variable.ValueFrom == nil) {
			continue
		}
		if current, found := findEnv(controller, variable.Name); !found || !apiequality.Semantic.DeepEqual(current, variable) {
			overrides = append(overrides, variable)
		}
	}
	return overrides
}

// probeOverrides returns the probes of the chart that the controller does not set to the same value, so that the
// converted spec keeps them. A probe that only the controller sets stays, because a spec cannot remove it.
func probeOverrides(chart, controller corev1.Container) (startup, liveness, readiness *corev1.Probe) {
	override := func(chart, controller *corev1.Probe) *corev1.Probe {
		if chart == nil || apiequality.Semantic.DeepEqual(chart, controller) {
			return nil
		}
		return chart.DeepCopy()
	}
	return override(chart.StartupProbe, controller.StartupProbe), override(chart.LivenessProbe, controller.LivenessProbe), override(chart.ReadinessProbe, controller.ReadinessProbe)
}

// nodeConfigOverrides returns the node configuration keys of the chart that the controller does not set to the same
// value.
func nodeConfigOverrides(chart, controller string) (*runtime.RawExtension, error) {
	var chartConfig, controllerConfig map[string]any
	if err := yaml.Unmarshal([]byte(chart), &chartConfig); err != nil {
		return nil, fmt.Errorf("decode the node configuration of the chart: %w", err)
	}
	if err := yaml.Unmarshal([]byte(controller), &controllerConfig); err != nil {
		return nil, fmt.Errorf("decode the node configuration of the controller: %w", err)
	}
	for key, value := range pomskyNodeConfigDefaults {
		if _, found := chartConfig[key]; !found {
			chartConfig[key] = value
		}
	}
	delta := configDelta(chartConfig, controllerConfig)
	if len(delta) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(delta)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: raw}, nil
}

// configDelta mirrors how the controller merges the overrides: maps are merged by key, and other values are replaced.
func configDelta(chart, controller map[string]any) map[string]any {
	delta := map[string]any{}
	for key, chartValue := range chart {
		controllerValue, found := controller[key]
		chartMap, chartIsMap := chartValue.(map[string]any)
		controllerMap, controllerIsMap := controllerValue.(map[string]any)
		switch {
		case chartIsMap && controllerIsMap:
			if nested := configDelta(chartMap, controllerMap); len(nested) > 0 {
				delta[key] = nested
			}
		case !found || !reflect.DeepEqual(chartValue, controllerValue):
			delta[key] = chartValue
		}
	}
	return delta
}
