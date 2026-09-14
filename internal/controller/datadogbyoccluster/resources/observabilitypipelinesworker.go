// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"
	"net"
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
)

// BuildObservabilityPipelinesWorker builds the child worker resource from a defaulted BYOC cluster.
func BuildObservabilityPipelinesWorker(cluster *datadoghqv1alpha1.DatadogBYOCCluster, image byocimage.ResolvedImage) (*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, error) {
	pipeline := cluster.Spec.Components.Pipeline
	if err := applyGlobalPipelineSettings(cluster, pipeline); err != nil {
		return nil, err
	}

	workerImage := datadoghqv1alpha1.DatadogBYOCImageSpec{
		Repository:       new(image.Repository),
		PullPolicy:       new(image.GetImagePullPolicy()),
		ImagePullSecrets: slices.Clone(image.ImagePullSecrets),
	}
	if image.Digest != "" {
		workerImage.Digest = new(image.Digest)
	} else {
		workerImage.Tag = new(image.Tag)
	}
	resolvedImage := datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec(workerImage)

	site := ptr.Deref(cluster.Spec.Datadog.Site, "datadoghq.com")
	return &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ComponentResourceName(cluster.Name, PipelineComponentName),
			Namespace: cluster.Namespace,
		},
		Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
			DatadogBYOCClusterPipelineComponentSpec: *pipeline,
			Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
				Site:            new(site),
				APIKeySecretRef: cluster.Spec.Datadog.APIKeySecretRef,
				AppKeySecretRef: cluster.Spec.Datadog.AppKeySecretRef,
			},
			Image: &resolvedImage,
		},
	}, nil
}

func applyGlobalPipelineSettings(cluster *datadoghqv1alpha1.DatadogBYOCCluster, pipeline *datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec) error {
	global := cluster.Spec.Global
	component := &pipeline.DatadogBYOCClusterComponentSpec

	component.Labels = labels(cluster, componentLabel(PipelineComponentName), component.Labels)
	component.Annotations = annotations(cluster, component.Annotations)
	component.Env = mergeEnv(global.Env, component.Env)
	component.Env = mergeEnv(component.Env, []corev1.EnvVar{{
		Name:  pipelineDestinationEndpointEnvName,
		Value: "http://" + net.JoinHostPort(ComponentResourceName(cluster.Name, IndexerComponentName), strconv.Itoa(int(restPort))),
	}})
	component.EnvFrom = slices.Concat(global.EnvFrom, component.EnvFrom)
	component.Volumes = mergeVolumes(global.Volumes, component.Volumes)
	component.VolumeMounts = mergeVolumeMounts(global.VolumeMounts, component.VolumeMounts)
	component.Tolerations = slices.Concat(global.Tolerations, component.Tolerations)
	component.TopologySpreadConstraints = mergeTopologySpreadConstraints(global.TopologySpreadConstraints, component.TopologySpreadConstraints)
	if global.Affinity != nil {
		var err error
		component.Affinity, err = mergeAffinity(global.Affinity, component.Affinity)
		if err != nil {
			return fmt.Errorf("merge pipeline affinity: %w", err)
		}
	}
	if component.PodDisruptionBudget == nil && global.PodDisruptionBudget != nil {
		component.PodDisruptionBudget = global.PodDisruptionBudget
	}
	return nil
}
