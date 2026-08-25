// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"maps"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// StatefulSetResources contains the resources managed for a stateful component.
type StatefulSetResources struct {
	Service     *corev1.Service
	StatefulSet *appsv1.StatefulSet
	HPA         *autoscalingv2.HorizontalPodAutoscaler
}

// Objects returns the component resources in apply order.
func (r *StatefulSetResources) Objects() []client.Object {
	objects := []client.Object{r.Service, r.StatefulSet}
	if r.HPA != nil {
		objects = append(objects, r.HPA)
	}
	return objects
}

// statefulSetBuilder renders the resources for a stateful component.
type statefulSetBuilder struct {
	workload workloadInput
	spec     *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec
	defaults statefulSetDefaults
}

type statefulSetDefaults struct {
	PodManagementPolicy appsv1.PodManagementPolicyType
	Autoscaling         autoscalingDefaults
}

type autoscalingDefaults struct {
	MinReplicas *int32
	MaxReplicas int32
	Metrics     []autoscalingv2.MetricSpec
	Behavior    *autoscalingv2.HorizontalPodAutoscalerBehavior
}

type statefulSetValues struct {
	Workload             workloadValues
	Replicas             *int32
	ServiceName          string
	PodManagementPolicy  appsv1.PodManagementPolicyType
	VolumeClaimTemplates []corev1.PersistentVolumeClaim
}

type hpaValues struct {
	Metadata       metav1.ObjectMeta
	ScaleTargetRef autoscalingv2.CrossVersionObjectReference
	MinReplicas    *int32
	MaxReplicas    int32
	Metrics        []autoscalingv2.MetricSpec
	Behavior       *autoscalingv2.HorizontalPodAutoscalerBehavior
}

func newStatefulSetBuilder(
	workload workloadInput,
	spec *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec,
	defaults statefulSetDefaults,
) statefulSetBuilder {
	return statefulSetBuilder{
		workload: workload,
		spec:     spec,
		defaults: defaults,
	}
}

func (b statefulSetBuilder) values() (serviceValues, statefulSetValues, *hpaValues, error) {
	workload, err := resolveWorkloadValues(b.workload)
	if err != nil {
		return serviceValues{}, statefulSetValues{}, nil, err
	}

	var volumeClaimTemplates []corev1.PersistentVolumeClaim
	if b.spec.PersistentVolumeClaim != nil {
		claim := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data"},
			Spec:       *b.spec.PersistentVolumeClaim.PersistentVolumeClaimSpec.DeepCopy(),
		}
		volumeClaimTemplates = []corev1.PersistentVolumeClaim{claim}
	}

	var replicas *int32
	var hpa *hpaValues
	if b.spec.Autoscaling == nil {
		replicas = ptr.To(workload.Replicas)
	} else {
		values := b.hpaValues(workload, b.spec.Autoscaling)
		hpa = &values
	}

	return workload.Service, statefulSetValues{
		Workload:             workload,
		Replicas:             replicas,
		ServiceName:          headlessServiceName(b.workload.Cluster.Name),
		PodManagementPolicy:  b.defaults.PodManagementPolicy,
		VolumeClaimTemplates: volumeClaimTemplates,
	}, hpa, nil
}

func (b statefulSetBuilder) build() (*StatefulSetResources, error) {
	service, statefulSet, hpa, err := b.values()
	if err != nil {
		return nil, err
	}
	result := &StatefulSetResources{
		Service:     createService(service),
		StatefulSet: createStatefulSet(statefulSet),
	}
	if hpa != nil {
		result.HPA = createHPA(*hpa)
	}
	return result, nil
}

func (b statefulSetBuilder) hpaValues(workload workloadValues, autoscaling *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec) hpaValues {
	metadata := metav1.ObjectMeta{
		Name:      workload.Metadata.Name,
		Namespace: workload.Metadata.Namespace,
		Labels:    maps.Clone(workload.Service.Metadata.Labels),
	}
	minReplicas := autoscaling.MinReplicas
	if minReplicas == nil {
		minReplicas = ptr.To(ptr.Deref(b.defaults.Autoscaling.MinReplicas, int32(0)))
	}
	maxReplicas := b.defaults.Autoscaling.MaxReplicas
	if autoscaling.MaxReplicas != nil {
		maxReplicas = *autoscaling.MaxReplicas
	}
	metrics := slices.Clone(autoscaling.Metrics)
	if len(metrics) == 0 {
		metrics = slices.Clone(b.defaults.Autoscaling.Metrics)
	}
	behavior := autoscaling.Behavior.DeepCopy()
	if behavior == nil {
		behavior = b.defaults.Autoscaling.Behavior.DeepCopy()
	}
	return hpaValues{
		Metadata:       metadata,
		ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: metadata.Name},
		MinReplicas:    minReplicas,
		MaxReplicas:    maxReplicas,
		Metrics:        metrics,
		Behavior:       behavior,
	}
}

func cpuUtilizationMetrics(averageUtilization int32) []autoscalingv2.MetricSpec {
	return []autoscalingv2.MetricSpec{{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name:   corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: ptr.To(averageUtilization)},
		},
	}}
}

func hpaBehavior(scaleUpWindow, scaleDownWindow int32) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	return &autoscalingv2.HorizontalPodAutoscalerBehavior{
		ScaleUp:   &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: ptr.To(scaleUpWindow)},
		ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: ptr.To(scaleDownWindow)},
	}
}

func createStatefulSet(values statefulSetValues) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: *values.Workload.Metadata.DeepCopy(),
		Spec: appsv1.StatefulSetSpec{
			Replicas:             values.Replicas,
			ServiceName:          values.ServiceName,
			PodManagementPolicy:  values.PodManagementPolicy,
			Selector:             &metav1.LabelSelector{MatchLabels: maps.Clone(values.Workload.Selector)},
			Template:             *values.Workload.Template.DeepCopy(),
			VolumeClaimTemplates: slices.Clone(values.VolumeClaimTemplates),
		},
	}
}

func createHPA(values hpaValues) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: *values.Metadata.DeepCopy(),
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: values.ScaleTargetRef,
			MinReplicas:    values.MinReplicas,
			MaxReplicas:    values.MaxReplicas,
			Metrics:        slices.Clone(values.Metrics),
			Behavior:       values.Behavior,
		},
	}
}
