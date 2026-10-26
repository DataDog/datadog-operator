// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// statefulSetWorkload runs a component as a StatefulSet, optionally autoscaled.
type statefulSetWorkload struct {
	spec        *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec
	serviceName string
}

// dataVolumes returns nil when the data volume comes from the volume claim template.
func (w statefulSetWorkload) dataVolumes() []corev1.Volume {
	if storage := w.spec.Storage; storage != nil && storage.VolumeClaimTemplate == nil && storage.EmptyDir != nil {
		return []corev1.Volume{{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: storage.EmptyDir}}}
	}
	return nil
}

func (w statefulSetWorkload) populate(r *ComponentResources, metadata metav1.ObjectMeta, selector map[string]string, template corev1.PodTemplateSpec) {
	r.StatefulSet = newStatefulSet(metadata, selector, template, w.serviceName, w.spec)
	if w.spec.Autoscaling != nil {
		r.HPA = newHPA(metadata, w.spec.Autoscaling)
	}
}

func (w statefulSetWorkload) obsoleteObjects(metadata metav1.ObjectMeta) []client.Object {
	if w.spec.Autoscaling != nil {
		return nil
	}
	return []client.Object{&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metadata}}
}

func newStatefulSet(metadata metav1.ObjectMeta, selector map[string]string, template corev1.PodTemplateSpec, serviceName string, spec *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec) *appsv1.StatefulSet {
	var replicas *int32
	if spec.Autoscaling == nil {
		replicas = spec.Replicas
	}
	return &appsv1.StatefulSet{
		ObjectMeta: metadata,
		Spec: appsv1.StatefulSetSpec{
			Replicas:             replicas,
			ServiceName:          serviceName,
			PodManagementPolicy:  appsv1.OrderedReadyPodManagement,
			Selector:             &metav1.LabelSelector{MatchLabels: selector},
			Template:             template,
			VolumeClaimTemplates: newVolumeClaimTemplates(spec.Storage),
		},
	}
}

func newVolumeClaimTemplates(storage *datadoghqv1alpha1.DatadogBYOCClusterStorageSpec) []corev1.PersistentVolumeClaim {
	if storage == nil || storage.VolumeClaimTemplate == nil {
		return nil
	}
	template := storage.VolumeClaimTemplate
	return []corev1.PersistentVolumeClaim{{
		TypeMeta: template.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:        dataVolumeName,
			Labels:      template.Metadata.Labels,
			Annotations: template.Metadata.Annotations,
		},
		Spec: template.Spec,
	}}
}

func newHPA(metadata metav1.ObjectMeta, autoscaling *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metadata,
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: metadata.Name},
			MinReplicas:    autoscaling.MinReplicas,
			MaxReplicas:    *autoscaling.MaxReplicas,
			Metrics:        autoscaling.Metrics,
			Behavior:       autoscaling.Behavior,
		},
	}
}
