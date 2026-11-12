// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package converter

import (
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	componentLabel     = "app.kubernetes.io/component"
	instanceLabel      = "app.kubernetes.io/instance"
	indexerComponent   = "indexer"
	irsaRoleAnnotation = "eks.amazonaws.com/role-arn"
)

// PipelineManifest is an Observability Pipelines Worker manifest converted to a BYOC pipeline.
type PipelineManifest struct {
	Pipeline string
	Manifest Manifest
}

// Result is a converted DatadogBYOCCluster and what the conversion could not express.
type Result struct {
	Cluster  *datadoghqv1alpha1.DatadogBYOCCluster
	Findings []Finding
}

// Convert builds a DatadogBYOCCluster that takes over the CloudPrem manifest in place and whose pipelines replace the
// Observability Pipelines Worker manifests. It copies the settings of the rendered objects, then compares them with the
// resources that the controllers create from the cluster, and keeps the environment variables and node configuration
// that differ as overrides.
func Convert(cloudprem Manifest, pipelines []PipelineManifest) (*Result, error) {
	pipelinesByManifest := make(map[string]string, len(pipelines))
	for _, pipeline := range pipelines {
		if other, duplicate := pipelinesByManifest[pipeline.Manifest.Name]; duplicate {
			return nil, fmt.Errorf("pipelines %s and %s use the same manifest %s", other, pipeline.Pipeline, pipeline.Manifest.Name)
		}
		pipelinesByManifest[pipeline.Manifest.Name] = pipeline.Pipeline
	}
	indexer, name, err := indexerOf(cloudprem)
	if err != nil {
		return nil, err
	}
	container, err := firstContainer(indexer)
	if err != nil {
		return nil, err
	}
	datadog, err := datadogSpec(container.Env)
	if err != nil {
		return nil, fmt.Errorf("manifest %s: %w", cloudprem.Name, err)
	}
	components, err := cloudpremComponents(cloudprem.Objects)
	if err != nil {
		return nil, fmt.Errorf("manifest %s: %w", cloudprem.Name, err)
	}

	cluster := &datadoghqv1alpha1.DatadogBYOCCluster{
		TypeMeta:   metav1.TypeMeta{APIVersion: datadoghqv1alpha1.GroupVersion.String(), Kind: "DatadogBYOCCluster"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cloudprem.Namespace},
		Spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
			ImageOverrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{BYOC: imageSpec(indexer, container)},
			Datadog:        datadog,
			Provider:       &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{AWS: awsSpec(cloudprem.Objects, indexer, container)},
			Identity:       identity(cloudprem.Objects, name, indexer),
			Components:     components,
		},
	}

	for _, pipeline := range pipelines {
		worker, workerErr := WorkerStatefulSet(pipeline.Manifest)
		if workerErr != nil {
			return nil, workerErr
		}
		workerContainer, containerErr := firstContainer(worker)
		if containerErr != nil {
			return nil, containerErr
		}
		if cluster.Spec.ImageOverrides.ObservabilityPipelinesWorker == nil {
			cluster.Spec.ImageOverrides.ObservabilityPipelinesWorker = imageSpec(worker, workerContainer)
		}
		spec, specErr := pipelineSpec(pipeline.Pipeline, pipeline.Manifest.Objects, worker)
		if specErr != nil {
			return nil, fmt.Errorf("manifest %s: %w", pipeline.Manifest.Name, specErr)
		}
		cluster.Spec.Components.Pipelines = append(cluster.Spec.Components.Pipelines, spec)
	}

	if err = applyOverrides(cluster, cloudprem, pipelines); err != nil {
		return nil, err
	}
	plan, err := NewPlan(cluster, cloudprem, pipelines)
	if err != nil {
		return nil, err
	}
	return &Result{Cluster: cluster, Findings: plan.Findings()}, nil
}

// ClusterName returns the name of the DatadogBYOCCluster that adopts the CloudPrem manifest.
func ClusterName(cloudprem Manifest) (string, error) {
	_, name, err := indexerOf(cloudprem)
	return name, err
}

// indexerOf returns the indexer StatefulSet of the CloudPrem manifest and the Helm fullname of the chart. The resource
// names of the cluster are <cluster>-<component>, so the cluster is named after the Helm fullname.
func indexerOf(cloudprem Manifest) (*appsv1.StatefulSet, string, error) {
	// The chart sets the component label on the selector, not on the StatefulSet.
	indexer, found := findObject(cloudprem.Objects, func(statefulSet *appsv1.StatefulSet) bool {
		return selects(statefulSet.Spec.Selector, indexerComponent)
	})
	if !found {
		return nil, "", fmt.Errorf("manifest %s has no indexer StatefulSet", cloudprem.Name)
	}
	name, found := strings.CutSuffix(indexer.Name, "-"+indexerComponent)
	if !found {
		return nil, "", fmt.Errorf("indexer StatefulSet %s of manifest %s is not named <fullname>-%s", indexer.Name, cloudprem.Name, indexerComponent)
	}
	return indexer, name, nil
}

// WorkerStatefulSet returns the StatefulSet of an Observability Pipelines Worker manifest.
func WorkerStatefulSet(manifest Manifest) (*appsv1.StatefulSet, error) {
	statefulSet, found := findObject(manifest.Objects, func(*appsv1.StatefulSet) bool { return true })
	if !found {
		return nil, fmt.Errorf("manifest %s has no StatefulSet", manifest.Name)
	}
	return statefulSet, nil
}

// MarshalCluster encodes a cluster as YAML, without the fields that a new object does not set.
func MarshalCluster(cluster *datadoghqv1alpha1.DatadogBYOCCluster) ([]byte, error) {
	object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cluster)
	if err != nil {
		return nil, err
	}
	delete(object, "status")
	if metadata, ok := object["metadata"].(map[string]any); ok {
		delete(metadata, "creationTimestamp")
	}
	return yaml.Marshal(object)
}

// identity references the ServiceAccount of the Pods, unless the chart created it with the name that the controller
// gives to its own ServiceAccount, in which case the controller adopts it.
func identity(objects []client.Object, clusterName string, workload *appsv1.StatefulSet) *datadoghqv1alpha1.DatadogBYOCClusterIdentitySpec {
	name := workload.Spec.Template.Spec.ServiceAccountName
	if name == "" {
		name = "default"
	}
	_, created := findObject(objects, func(serviceAccount *corev1.ServiceAccount) bool { return serviceAccount.Name == name })
	if created && name == clusterName {
		return nil
	}
	return &datadoghqv1alpha1.DatadogBYOCClusterIdentitySpec{ServiceAccountName: new(name)}
}

func datadogSpec(env []corev1.EnvVar) (*datadoghqv1alpha1.DatadogBYOCClusterDatadogSpec, error) {
	spec := &datadoghqv1alpha1.DatadogBYOCClusterDatadogSpec{
		Site:            envValue(env, "DD_SITE"),
		BYOCTelemetry:   new(strings.EqualFold(ptr.Deref(envValue(env, "BYOC_TELEMETRY_ENABLED"), ""), "true")),
		DogstatsdServer: &datadoghqv1alpha1.DatadogBYOCClusterDogstatsdServerSpec{Host: envValue(env, "CP_DOGSTATSD_SERVER_HOST")},
	}
	if apiKey, found := findEnv(env, "DD_API_KEY"); found && apiKey.ValueFrom != nil {
		spec.APIKeySecretRef = apiKey.ValueFrom.SecretKeyRef.DeepCopy()
	}
	if port := envValue(env, "CP_DOGSTATSD_SERVER_PORT"); port != nil {
		value, err := strconv.ParseInt(*port, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("parse CP_DOGSTATSD_SERVER_PORT: %w", err)
		}
		spec.DogstatsdServer.Port = new(int32(value))
	}
	return spec, nil
}

func awsSpec(objects []client.Object, workload *appsv1.StatefulSet, container corev1.Container) *datadoghqv1alpha1.DatadogBYOCClusterAWSSpec {
	spec := &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{Region: envValue(container.Env, "AWS_REGION")}
	serviceAccount, found := findObject(objects, func(serviceAccount *corev1.ServiceAccount) bool {
		return serviceAccount.Name == workload.Spec.Template.Spec.ServiceAccountName
	})
	if !found {
		return spec
	}
	if role, annotated := serviceAccount.Annotations[irsaRoleAnnotation]; annotated {
		spec.IRSARoleARN = new(role)
	}
	return spec
}

func imageSpec(workload *appsv1.StatefulSet, container corev1.Container) *datadoghqv1alpha1.DatadogBYOCImageSpec {
	repository, tag, digest := parseImage(container.Image)
	spec := &datadoghqv1alpha1.DatadogBYOCImageSpec{
		Repository:       new(repository),
		ImagePullSecrets: workload.Spec.Template.Spec.ImagePullSecrets,
	}
	// The chart prefers the digest, and the API accepts either a tag or a digest.
	switch {
	case digest != "":
		spec.Digest = new(digest)
	case tag != "":
		spec.Tag = new(tag)
	}
	if container.ImagePullPolicy != "" {
		spec.PullPolicy = new(container.ImagePullPolicy)
	}
	return spec
}

// parseImage splits an image reference into its repository, tag, and digest.
func parseImage(reference string) (repository, tag, digest string) {
	repository, digest, _ = strings.Cut(reference, "@")
	// A colon before the last slash separates a registry port, not a tag.
	if i := strings.LastIndex(repository, ":"); i > strings.LastIndex(repository, "/") {
		repository, tag = repository[:i], repository[i+1:]
	}
	return repository, tag, digest
}

func firstContainer(workload *appsv1.StatefulSet) (corev1.Container, error) {
	if len(workload.Spec.Template.Spec.Containers) == 0 {
		return corev1.Container{}, fmt.Errorf("StatefulSet %s has no container", workload.Name)
	}
	return workload.Spec.Template.Spec.Containers[0], nil
}

// findObject returns the first object of type T that matches.
func findObject[T client.Object](objects []client.Object, match func(T) bool) (T, bool) {
	for _, object := range objects {
		if typed, ok := object.(T); ok && match(typed) {
			return typed, true
		}
	}
	var zero T
	return zero, false
}

func findEnv(env []corev1.EnvVar, name string) (corev1.EnvVar, bool) {
	for _, variable := range env {
		if variable.Name == name {
			return variable, true
		}
	}
	return corev1.EnvVar{}, false
}

// envValue returns the literal value of a variable, or nil when it is unset or set from a source.
func envValue(env []corev1.EnvVar, name string) *string {
	variable, found := findEnv(env, name)
	if !found || variable.ValueFrom != nil {
		return nil
	}
	return new(variable.Value)
}
