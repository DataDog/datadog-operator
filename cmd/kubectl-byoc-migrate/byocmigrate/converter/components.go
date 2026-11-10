// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package converter

import (
	"fmt"
	"maps"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocresources "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/resources"
)

// Component label values of the CloudPrem chart.
const (
	searcherComponent          = "searcher"
	metastoreComponent         = "metastore"
	readOnlyMetastoreComponent = "metastore-ro"
	controlPlaneComponent      = "control-plane"
	janitorComponent           = "janitor"
	compactorComponent         = "compactor"
)

const (
	configVolumeName        = "config"
	dataVolumeName          = "data"
	metastoreURIEnv         = "QW_METASTORE_URI"
	readOnlyMetastoreURIEnv = "QW_METASTORE_READ_REPLICA_URI"
)

// chartLabels are set by the charts. The controllers set their own name, instance, component, and managed-by labels.
var chartLabels = []string{
	"helm.sh/chart",
	"app.kubernetes.io/name",
	instanceLabel,
	"app.kubernetes.io/version",
	"app.kubernetes.io/managed-by",
	componentLabel,
}

// cloudpremComponents reads the CloudPrem components from the workloads of the manifest.
func cloudpremComponents(objects []client.Object) (*datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec, error) {
	components := &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{}
	var err error
	if components.Indexer, err = statefulComponent(objects, indexerComponent); err != nil {
		return nil, err
	}
	if components.Searcher, err = statefulComponent(objects, searcherComponent); err != nil {
		return nil, err
	}
	if components.Metastore, err = metastore(objects, metastoreComponent, metastoreURIEnv); err != nil {
		return nil, err
	}
	if components.ControlPlane, err = deploymentComponent(objects, controlPlaneComponent); err != nil {
		return nil, err
	}
	if components.Janitor, err = deploymentComponent(objects, janitorComponent); err != nil {
		return nil, err
	}
	if hasDeployment(objects, readOnlyMetastoreComponent) {
		if components.ReadOnlyMetastore, err = metastore(objects, readOnlyMetastoreComponent, readOnlyMetastoreURIEnv); err != nil {
			return nil, err
		}
	}
	if hasDeployment(objects, compactorComponent) {
		if components.Compactor, err = compactor(objects); err != nil {
			return nil, err
		}
	}
	return components, nil
}

func statefulComponent(objects []client.Object, component string) (*datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec, error) {
	statefulSet, found := findObject(objects, func(statefulSet *appsv1.StatefulSet) bool { return selects(statefulSet.Spec.Selector, component) })
	if !found {
		return nil, fmt.Errorf("no %s StatefulSet", component)
	}
	spec := &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Storage: storage(statefulSet), PodManagementPolicy: podManagementPolicy(statefulSet)}
	hpa, autoscaled := findObject(objects, func(hpa *autoscalingv2.HorizontalPodAutoscaler) bool {
		return hpa.Spec.ScaleTargetRef.Kind == "StatefulSet" && hpa.Spec.ScaleTargetRef.Name == statefulSet.Name
	})
	if autoscaled {
		spec.Autoscaling = autoscaling(hpa)
	}
	var err error
	spec.DatadogBYOCClusterComponentSpec, err = componentSpec(objects, component, statefulSet.Spec.Replicas, statefulSet.Spec.Template, autoscaled)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

func metastore(objects []client.Object, component, databaseURIEnv string) (*datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec, error) {
	deployment, err := deploymentComponent(objects, component)
	if err != nil {
		return nil, err
	}
	spec := &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{DatadogBYOCClusterComponentSpec: *deployment}
	workload, _ := findDeployment(objects, component)
	container := workload.Spec.Template.Spec.Containers[0]
	if uri, found := findEnv(container.Env, databaseURIEnv); found {
		if uri.ValueFrom != nil && uri.ValueFrom.SecretKeyRef != nil {
			spec.Database = &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{URISecretRef: uri.ValueFrom.SecretKeyRef.DeepCopy()}
		}
		return spec, nil
	}
	// The chart recommends importing the URI from a Secret with envFrom, which names the variables after the keys.
	for _, source := range container.EnvFrom {
		if source.SecretRef != nil && source.Prefix == "" {
			spec.Database = &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{URISecretRef: &corev1.SecretKeySelector{
				LocalObjectReference: source.SecretRef.LocalObjectReference,
				Key:                  databaseURIEnv,
			}}
			return spec, nil
		}
	}
	return spec, nil
}

func compactor(objects []client.Object) (*datadoghqv1alpha1.DatadogBYOCClusterCompactorComponentSpec, error) {
	deployment, _ := findDeployment(objects, compactorComponent)
	spec := &datadoghqv1alpha1.DatadogBYOCClusterCompactorComponentSpec{}
	hpa, autoscaled := findObject(objects, func(hpa *autoscalingv2.HorizontalPodAutoscaler) bool { return scales(hpa, deployment) })
	if autoscaled {
		spec.Autoscaling = autoscaling(hpa)
	}
	var err error
	spec.DatadogBYOCClusterComponentSpec, err = componentSpec(objects, compactorComponent, deployment.Spec.Replicas, deployment.Spec.Template, autoscaled)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

func deploymentComponent(objects []client.Object, component string) (*datadoghqv1alpha1.DatadogBYOCClusterComponentSpec, error) {
	deployment, found := findDeployment(objects, component)
	if !found {
		return nil, fmt.Errorf("no %s Deployment", component)
	}
	spec, err := componentSpec(objects, component, deployment.Spec.Replicas, deployment.Spec.Template, false)
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

func componentSpec(objects []client.Object, component string, replicas *int32, template corev1.PodTemplateSpec, autoscaled bool) (datadoghqv1alpha1.DatadogBYOCClusterComponentSpec, error) {
	if len(template.Spec.Containers) == 0 {
		return datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{}, fmt.Errorf("%s workload has no container", component)
	}
	pod := template.Spec
	container := pod.Containers[0]
	spec := datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
		EnvFrom:                       container.EnvFrom,
		Volumes:                       slices.DeleteFunc(slices.Clone(pod.Volumes), func(volume corev1.Volume) bool { return isChartVolume(volume.Name) }),
		VolumeMounts:                  slices.DeleteFunc(slices.Clone(container.VolumeMounts), func(mount corev1.VolumeMount) bool { return isChartVolume(mount.Name) }),
		Annotations:                   withoutChecksums(template.Annotations),
		Labels:                        withoutKeys(template.Labels, chartLabels...),
		NodeSelector:                  pod.NodeSelector,
		Tolerations:                   pod.Tolerations,
		TopologySpreadConstraints:     pod.TopologySpreadConstraints,
		InitContainers:                pod.InitContainers,
		TerminationGracePeriodSeconds: pod.TerminationGracePeriodSeconds,
		// Without an affinity, the controller adds its default pod anti-affinity, which the chart does not set.
		Affinity:            pod.Affinity,
		PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{},
	}
	if !autoscaled {
		spec.Replicas = replicas
	}
	if len(container.Resources.Limits) > 0 || len(container.Resources.Requests) > 0 {
		spec.Resources = container.Resources.DeepCopy()
	}
	if budget, found := findObject(objects, func(budget *policyv1.PodDisruptionBudget) bool { return selects(budget.Spec.Selector, component) }); found {
		spec.PodDisruptionBudget = &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
			MinAvailable:   budget.Spec.MinAvailable,
			MaxUnavailable: budget.Spec.MaxUnavailable,
		}
	}
	return spec, nil
}

// podManagementPolicy returns the pod management policy of the StatefulSet, which cannot change, or nil when the chart
// leaves it to the API server default.
func podManagementPolicy(statefulSet *appsv1.StatefulSet) *appsv1.PodManagementPolicyType {
	if statefulSet.Spec.PodManagementPolicy == "" {
		return nil
	}
	return new(statefulSet.Spec.PodManagementPolicy)
}

// storage returns the volume claim template of the StatefulSet, or the emptyDir data volume of its Pods.
func storage(statefulSet *appsv1.StatefulSet) *datadoghqv1alpha1.DatadogBYOCClusterStorageSpec {
	for _, claim := range statefulSet.Spec.VolumeClaimTemplates {
		if claim.Name == dataVolumeName {
			return &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
				Metadata: datadoghqv1alpha1.DatadogBYOCClusterEmbeddedObjectMetadata{Labels: claim.Labels, Annotations: claim.Annotations},
				Spec:     claim.Spec,
			}}
		}
	}
	emptyDir := &corev1.EmptyDirVolumeSource{}
	for _, volume := range statefulSet.Spec.Template.Spec.Volumes {
		if volume.Name == dataVolumeName && volume.EmptyDir != nil {
			emptyDir = volume.EmptyDir
		}
	}
	return &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{EmptyDir: emptyDir}
}

func autoscaling(hpa *autoscalingv2.HorizontalPodAutoscaler) *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec {
	spec := &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{
		MinReplicas: hpa.Spec.MinReplicas,
		MaxReplicas: new(hpa.Spec.MaxReplicas),
		Metrics:     hpa.Spec.Metrics,
		// An empty behavior keeps the controller from adding its default behavior, which the chart does not set.
		Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
	}
	if hpa.Spec.Behavior != nil {
		spec.Behavior = hpa.Spec.Behavior
	}
	return spec
}

func findDeployment(objects []client.Object, component string) (*appsv1.Deployment, bool) {
	return findObject(objects, func(deployment *appsv1.Deployment) bool { return selects(deployment.Spec.Selector, component) })
}

func hasDeployment(objects []client.Object, component string) bool {
	_, found := findDeployment(objects, component)
	return found
}

// selects reports whether a selector of the chart selects the Pods of a component.
func selects(selector *metav1.LabelSelector, component string) bool {
	return selector != nil && selector.MatchLabels[componentLabel] == component
}

func isChartVolume(name string) bool {
	return name == configVolumeName || name == dataVolumeName
}

func withoutKeys(values map[string]string, keys ...string) map[string]string {
	result := maps.Clone(values)
	for _, key := range keys {
		delete(result, key)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// componentRef links a CloudPrem component of the chart to the component that the controller renders.
type componentRef struct {
	chart      string
	controller string
	spec       *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
}

func componentRefs(components *datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec) []componentRef {
	refs := []componentRef{
		{chart: indexerComponent, controller: byocresources.IndexerComponentName, spec: &components.Indexer.DatadogBYOCClusterComponentSpec},
		{chart: searcherComponent, controller: byocresources.SearcherComponentName, spec: &components.Searcher.DatadogBYOCClusterComponentSpec},
		{chart: metastoreComponent, controller: byocresources.MetastoreComponentName, spec: &components.Metastore.DatadogBYOCClusterComponentSpec},
		{chart: controlPlaneComponent, controller: byocresources.ControlPlaneComponentName, spec: components.ControlPlane},
		{chart: janitorComponent, controller: byocresources.JanitorComponentName, spec: components.Janitor},
	}
	if components.ReadOnlyMetastore != nil {
		refs = append(refs, componentRef{chart: readOnlyMetastoreComponent, controller: byocresources.ReadOnlyMetastoreComponentName, spec: &components.ReadOnlyMetastore.DatadogBYOCClusterComponentSpec})
	}
	if components.Compactor != nil {
		refs = append(refs, componentRef{chart: compactorComponent, controller: byocresources.CompactorComponentName, spec: &components.Compactor.DatadogBYOCClusterComponentSpec})
	}
	return refs
}

// chartPodTemplate returns the Pod template of the workload of a chart component.
func chartPodTemplate(objects []client.Object, component string) (corev1.PodTemplateSpec, bool) {
	if statefulSet, found := findObject(objects, func(statefulSet *appsv1.StatefulSet) bool { return selects(statefulSet.Spec.Selector, component) }); found {
		return statefulSet.Spec.Template, true
	}
	if deployment, found := findDeployment(objects, component); found {
		return deployment.Spec.Template, true
	}
	return corev1.PodTemplateSpec{}, false
}

// controllerPodTemplate returns the Pod template of the workload that the controller renders for a component.
func controllerPodTemplate(resources *byocresources.Resources, component string) (corev1.PodTemplateSpec, bool) {
	rendered := resources.Component(component)
	switch {
	case rendered == nil:
		return corev1.PodTemplateSpec{}, false
	case rendered.StatefulSet != nil:
		return rendered.StatefulSet.Spec.Template, true
	default:
		return rendered.Deployment.Spec.Template, true
	}
}
