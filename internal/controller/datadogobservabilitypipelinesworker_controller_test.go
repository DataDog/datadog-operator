// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controller

import (
	"context"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

var _ = Describe("DatadogObservabilityPipelinesWorker Controller", func() {
	var (
		worker    *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker
		namespace *corev1.Namespace
	)

	BeforeEach(func() {
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "worker-test-"}}
		createKubernetesObject(k8sClient, namespace)

		image := datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
			Repository: ptr.To("registry.invalid/datadog/observability-pipelines-worker"),
			Tag:        ptr.To("envtest"),
		}
		worker = &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
			ObjectMeta: metav1.ObjectMeta{Name: "standalone-worker", Namespace: namespace.Name},
			Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
								MinAvailable: ptr.To(intstr.FromInt32(1)),
							},
						},
						Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{},
					},
					PipelineID: ptr.To("pipeline-id"),
				},
				Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
					Site: ptr.To("datadoghq.com"),
					APIKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
						Key:                  "api-key",
					},
				},
				Image: &image,
				Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{{Name: "otlp-grpc", Port: 4317}},
			},
		}
		createKubernetesObject(k8sClient, worker)
	})

	AfterEach(func() {
		deleteKubernetesObject(k8sClient, namespace)
	})

	It("reconciles standalone resources and status", func() {
		objects := []client.Object{
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
			&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
			&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
			&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
		}
		for _, object := range objects {
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKeyFromObject(object), object)
			}, timeout, interval).Should(Succeed())
		}

		statefulSet := objects[2].(*appsv1.StatefulSet)

		Eventually(func(g Gomega) {
			current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current)).To(Succeed())
			want := datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerStatus{
				ObservedGeneration: ptr.To(current.Generation),
				Replicas:           ptr.To[int32](0),
				ReadyReplicas:      ptr.To[int32](0),
				Conditions: []metav1.Condition{
					{Type: conditionReconciled, Status: metav1.ConditionTrue, ObservedGeneration: current.Generation, Reason: "Reconciled", Message: "Managed resources match the desired state"},
					{Type: conditionAvailable, Status: metav1.ConditionFalse, ObservedGeneration: current.Generation, Reason: "WorkloadUnavailable", Message: "Worker workload is not yet available"},
				},
			}
			g.Expect(cmp.Diff(want, current.Status, cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"))).To(BeEmpty())
		}, timeout, interval).Should(Succeed())

		// envtest does not run the StatefulSet controller, so simulate a ready workload by updating its status.
		Eventually(func() error {
			current := &appsv1.StatefulSet{}
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), current); err != nil {
				return err
			}
			current.Status.ObservedGeneration = current.Generation
			current.Status.Replicas = 2
			current.Status.ReadyReplicas = 2
			return k8sClient.Status().Update(context.Background(), current)
		}, timeout, interval).Should(Succeed())

		Eventually(func(g Gomega) {
			current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current)).To(Succeed())
			want := datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerStatus{
				ObservedGeneration: ptr.To(current.Generation),
				Replicas:           ptr.To[int32](2),
				ReadyReplicas:      ptr.To[int32](2),
				Conditions: []metav1.Condition{
					{Type: conditionReconciled, Status: metav1.ConditionTrue, ObservedGeneration: current.Generation, Reason: "Reconciled", Message: "Managed resources match the desired state"},
					{Type: conditionAvailable, Status: metav1.ConditionTrue, ObservedGeneration: current.Generation, Reason: "Available", Message: "Worker workload is available"},
				},
			}
			g.Expect(cmp.Diff(want, current.Status, cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"))).To(BeEmpty())
		}, timeout, interval).Should(Succeed())
	})

	It("deletes obsolete optional resources", func() {
		statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}}
		obsolete := []client.Object{
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
			&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
			&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
		}
		for _, object := range append(obsolete, statefulSet) {
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKeyFromObject(object), object)
			}, timeout, interval).Should(Succeed())
		}

		Eventually(func() error {
			current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current); err != nil {
				return err
			}
			current.Spec.Ports = nil
			current.Spec.Identity = &datadoghqv1alpha1.DatadogBYOCClusterIdentitySpec{ServiceAccountName: ptr.To("existing-worker")}
			current.Spec.Autoscaling = nil
			current.Spec.PodDisruptionBudget = &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{}
			return k8sClient.Update(context.Background(), current)
		}, timeout, interval).Should(Succeed())

		for _, object := range obsolete {
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(object), object)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		}

		Eventually(func() error {
			current := &appsv1.StatefulSet{}
			return k8sClient.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), current)
		}, timeout, interval).Should(Succeed())
	})
})
