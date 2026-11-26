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

// workload renders the controller running the pods of a component.
type workload interface {
	dataVolumes() []corev1.Volume
	populate(r *ComponentResources, metadata metav1.ObjectMeta, selector map[string]string, template corev1.PodTemplateSpec)
	obsoleteObjects(metadata metav1.ObjectMeta) []client.Object
}

// deploymentWorkload runs a component as a Deployment with ephemeral data.
type deploymentWorkload struct {
	replicas int32
	strategy appsv1.DeploymentStrategyType
}

func (w deploymentWorkload) dataVolumes() []corev1.Volume {
	return []corev1.Volume{{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
}

func (w deploymentWorkload) populate(r *ComponentResources, metadata metav1.ObjectMeta, selector map[string]string, template corev1.PodTemplateSpec) {
	r.Deployment = newDeployment(metadata, selector, template, w.replicas, w.strategy)
}

func (w deploymentWorkload) obsoleteObjects(metav1.ObjectMeta) []client.Object {
	return nil
}

// autoscalableDeploymentWorkload runs a component as a Deployment, optionally autoscaled.
type autoscalableDeploymentWorkload struct {
	deploymentWorkload
	autoscaling *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec
}

func (w autoscalableDeploymentWorkload) populate(r *ComponentResources, metadata metav1.ObjectMeta, selector map[string]string, template corev1.PodTemplateSpec) {
	w.deploymentWorkload.populate(r, metadata, selector, template)
	if w.autoscaling != nil {
		// The HPA sets the replicas of an autoscaled Deployment.
		r.Deployment.Spec.Replicas = nil
		r.HPA = newHPA(metadata, "Deployment", w.autoscaling)
	}
}

func (w autoscalableDeploymentWorkload) obsoleteObjects(metadata metav1.ObjectMeta) []client.Object {
	if w.autoscaling != nil {
		return nil
	}
	return []client.Object{&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metadata}}
}

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
		r.HPA = newHPA(metadata, "StatefulSet", w.spec.Autoscaling)
	}
}

func (w statefulSetWorkload) obsoleteObjects(metadata metav1.ObjectMeta) []client.Object {
	if w.spec.Autoscaling != nil {
		return nil
	}
	return []client.Object{&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metadata}}
}
