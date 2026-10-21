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
	"k8s.io/apimachinery/pkg/api/meta"
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
							Replicas: ptr.To[int32](3),
							PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
								MinAvailable: ptr.To(intstr.FromInt32(1)),
							},
						},
						Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{},
					},
					PipelineID: ptr.To("pipeline-id"),
					Ports:      []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{{Name: "otlp-grpc", Port: 4317}},
				},
				Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
					Site: ptr.To("datadoghq.com"),
					APIKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
						Key:                  "api-key",
					},
				},
				Image: &image,
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
					{Type: "Reconciled", Status: metav1.ConditionTrue, ObservedGeneration: current.Generation, Reason: "Reconciled", Message: "Managed resources match the desired state"},
					{Type: "Available", Status: metav1.ConditionFalse, ObservedGeneration: current.Generation, Reason: "WorkloadUnavailable", Message: "Worker workload is not yet available"},
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
					{Type: "Reconciled", Status: metav1.ConditionTrue, ObservedGeneration: current.Generation, Reason: "Reconciled", Message: "Managed resources match the desired state"},
					{Type: "Available", Status: metav1.ConditionTrue, ObservedGeneration: current.Generation, Reason: "Available", Message: "Worker workload is available"},
				},
			}
			g.Expect(cmp.Diff(want, current.Status, cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"))).To(BeEmpty())
		}, timeout, interval).Should(Succeed())
	})

	DescribeTable("uses the StatefulSet replica target for autoscaled availability", func(replicas, readyReplicas int32, observedGenerationLag int64, wantStatus metav1.ConditionStatus, wantReason, wantMessage string) {
		By("updating the replica target as an HPA would")
		Eventually(func() error {
			current := &appsv1.StatefulSet{}
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current); err != nil {
				return err
			}
			current.Spec.Replicas = ptr.To(replicas)
			return k8sClient.Update(context.Background(), current, client.FieldOwner("test-hpa"))
		}, timeout, interval).Should(Succeed())

		// envtest does not run the StatefulSet controller.
		Eventually(func() error {
			current := &appsv1.StatefulSet{}
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current); err != nil {
				return err
			}
			current.Status = appsv1.StatefulSetStatus{
				ObservedGeneration: current.Generation - observedGenerationLag,
				Replicas:           replicas,
				ReadyReplicas:      readyReplicas,
			}
			return k8sClient.Status().Update(context.Background(), current)
		}, timeout, interval).Should(Succeed())

		want := datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerStatus{
			ObservedGeneration: ptr.To[int64](1),
			Replicas:           ptr.To(replicas),
			ReadyReplicas:      ptr.To(readyReplicas),
			Conditions: []metav1.Condition{
				{Type: "Reconciled", Status: metav1.ConditionTrue, ObservedGeneration: 1, Reason: "Reconciled", Message: "Managed resources match the desired state"},
				{Type: "Available", Status: wantStatus, ObservedGeneration: 1, Reason: wantReason, Message: wantMessage},
			},
		}
		Eventually(func(g Gomega) {
			current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current)).To(Succeed())
			g.Expect(cmp.Diff(want, current.Status, cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"))).To(BeEmpty())
		}, timeout, interval).Should(Succeed())
	},
		Entry("scaled below the configured replicas", int32(1), int32(1), int64(0), metav1.ConditionTrue, "Available", "Worker workload is available"),
		Entry("waiting for the additional replica", int32(4), int32(3), int64(0), metav1.ConditionFalse, "WorkloadUnavailable", "Worker workload is not yet available"),
		Entry("all scaled replicas are ready", int32(4), int32(4), int64(0), metav1.ConditionTrue, "Available", "Worker workload is available"),
		Entry("waiting for the StatefulSet generation", int32(4), int32(4), int64(1), metav1.ConditionFalse, "WorkloadUnavailable", "Worker workload is not yet available"),
	)

	It("deletes obsolete optional resources", func() {
		statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}}
		service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}}
		obsolete := []client.Object{
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
			&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
			&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: worker.Name, Namespace: worker.Namespace}},
		}
		for _, object := range append(obsolete, statefulSet, service) {
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKeyFromObject(object), object)
			}, timeout, interval).Should(Succeed())
		}

		Eventually(func() error {
			current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current); err != nil {
				return err
			}
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
		Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(service), service)).To(Succeed())
	})

	It("switches between Remote Config and a local pipeline", func() {
		configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: worker.Name + "-pipeline", Namespace: worker.Namespace}}
		setPipelineConfig := func(config *string) {
			Eventually(func() error {
				current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current); err != nil {
					return err
				}
				current.Spec.PipelineConfig = config
				return k8sClient.Update(context.Background(), current)
			}, timeout, interval).Should(Succeed())
		}
		statefulSet := func(g Gomega) *appsv1.StatefulSet {
			current := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current)).To(Succeed())
			return current
		}

		By("rendering the local pipeline")
		setPipelineConfig(ptr.To("sources: {}\n"))
		var checksum string
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(configMap), configMap)).To(Succeed())
			g.Expect(configMap.Data).To(Equal(map[string]string{"pipeline.yaml": "sources: {}\n"}))
			g.Expect(metav1.IsControlledBy(configMap, workerWithUID(g, worker))).To(BeTrue())
			template := statefulSet(g).Spec.Template
			g.Expect(template.Spec.Containers[0].Args).To(Equal([]string{"run", "--pipeline-config=/etc/observability-pipelines-worker/pipeline/pipeline.yaml", "--skip-key-validation"}))
			checksum = template.Annotations["checksum/pipeline"]
			g.Expect(checksum).NotTo(BeEmpty())
		}, timeout, interval).Should(Succeed())

		By("rolling the pods when the pipeline changes")
		setPipelineConfig(ptr.To("sources: {}\nsinks: {}\n"))
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(configMap), configMap)).To(Succeed())
			g.Expect(configMap.Data["pipeline.yaml"]).To(Equal("sources: {}\nsinks: {}\n"))
			g.Expect(statefulSet(g).Spec.Template.Annotations["checksum/pipeline"]).NotTo(BeElementOf("", checksum))
		}, timeout, interval).Should(Succeed())

		By("removing the local pipeline when switching back to Remote Config")
		setPipelineConfig(nil)
		Eventually(func(g Gomega) {
			g.Expect(apierrors.IsNotFound(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(configMap), &corev1.ConfigMap{}))).To(BeTrue())
			template := statefulSet(g).Spec.Template
			g.Expect(template.Annotations).NotTo(HaveKey("checksum/pipeline"))
			g.Expect(template.Spec.Containers[0].Args).To(Equal([]string{"run"}))
			for _, volume := range template.Spec.Volumes {
				g.Expect(volume.Name).NotTo(Equal("pipeline-config"))
			}
			for _, mount := range template.Spec.Containers[0].VolumeMounts {
				g.Expect(mount.Name).NotTo(Equal("pipeline-config"))
			}
		}, timeout, interval).Should(Succeed())
	})

	It("runs a local pipeline without a pipelineID", func() {
		local := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
			ObjectMeta: metav1.ObjectMeta{Name: "local-worker", Namespace: namespace.Name},
			Spec:       *worker.Spec.DeepCopy(),
		}
		local.Spec.PipelineID = nil
		local.Spec.PipelineConfig = ptr.To("sources: {}\n")
		createKubernetesObject(k8sClient, local)

		By("reporting the Worker UID as the pipeline ID")
		statefulSet := &appsv1.StatefulSet{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(local), statefulSet)).To(Succeed())
			g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{Name: "DD_OP_PIPELINE_ID", Value: string(workerWithUID(g, local).UID)}))
		}, timeout, interval).Should(Succeed())

		By("becoming Available once the workload is ready")
		// envtest does not run the StatefulSet controller, so simulate a ready workload by updating its status.
		Eventually(func() error {
			current := &appsv1.StatefulSet{}
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(statefulSet), current); err != nil {
				return err
			}
			current.Status.ObservedGeneration = current.Generation
			current.Status.Replicas = 3
			current.Status.ReadyReplicas = 3
			return k8sClient.Status().Update(context.Background(), current)
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			current := workerWithUID(g, local)
			g.Expect(meta.IsStatusConditionTrue(current.Status.Conditions, "Reconciled")).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(current.Status.Conditions, "Available")).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	Context("when an unowned ConfigMap has the pipeline name", func() {
		var configMap *corev1.ConfigMap

		BeforeEach(func() {
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: worker.Name + "-pipeline", Namespace: namespace.Name},
				Data:       map[string]string{"key": "value"},
			}
			createKubernetesObject(k8sClient, configMap)
		})

		It("reports a conflict and keeps the ConfigMap after local mode is turned off", func() {
			setPipelineConfig := func(config *string) {
				Eventually(func() error {
					current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
					if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current); err != nil {
						return err
					}
					current.Spec.PipelineConfig = config
					return k8sClient.Update(context.Background(), current)
				}, timeout, interval).Should(Succeed())
			}

			By("reporting Reconciled=False with reason Conflict")
			setPipelineConfig(ptr.To("sources: {}\n"))
			Eventually(func(g Gomega) {
				condition := meta.FindStatusCondition(workerWithUID(g, worker).Status.Conditions, "Reconciled")
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(condition.Reason).To(Equal("Conflict"))
			}, timeout, interval).Should(Succeed())

			By("leaving the ConfigMap alone after switching back to Remote Config")
			setPipelineConfig(nil)
			Eventually(func(g Gomega) {
				g.Expect(meta.IsStatusConditionTrue(workerWithUID(g, worker).Status.Conditions, "Reconciled")).To(BeTrue())
			}, timeout, interval).Should(Succeed())
			current := &corev1.ConfigMap{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(configMap), current)).To(Succeed())
			Expect(current.Data).To(Equal(map[string]string{"key": "value"}))
			Expect(current.OwnerReferences).To(BeEmpty())
		})
	})

	Context("when an unowned object has a managed name", func() {
		var (
			conflicting    *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker
			serviceAccount *corev1.ServiceAccount
		)

		BeforeEach(func() {
			serviceAccount = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "conflicting-worker", Namespace: namespace.Name, Labels: map[string]string{"key": "value"}},
			}
			createKubernetesObject(k8sClient, serviceAccount)
			conflicting = &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
				ObjectMeta: metav1.ObjectMeta{Name: "conflicting-worker", Namespace: namespace.Name},
				Spec:       *worker.Spec.DeepCopy(),
			}
			createKubernetesObject(k8sClient, conflicting)
		})

		It("reports a conflict without changing the object", func() {
			By("reporting Reconciled=False with reason Conflict")
			Eventually(func(g Gomega) {
				current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(conflicting), current)).To(Succeed())
				g.Expect(meta.IsStatusConditionFalse(current.Status.Conditions, "Reconciled")).To(BeTrue())
				g.Expect(meta.FindStatusCondition(current.Status.Conditions, "Reconciled").Reason).To(Equal("Conflict"))
				g.Expect(meta.FindStatusCondition(current.Status.Conditions, "Reconciled").Message).To(Equal("apply *v1.ServiceAccount " + namespace.Name + "/conflicting-worker: exists and is not controlled by DatadogObservabilityPipelinesWorker conflicting-worker"))
			}, timeout, interval).Should(Succeed())

			By("keeping the unowned ServiceAccount unchanged")
			current := &corev1.ServiceAccount{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(serviceAccount), current)).To(Succeed())
			Expect(current.Labels).To(Equal(map[string]string{"key": "value"}))
			Expect(current.OwnerReferences).To(BeEmpty())
		})
	})

	Context("when the configuration is invalid", func() {
		It("reports the failure without retrying", func() {
			terminalErrorsBefore := terminalReconcileErrors("datadogobservabilitypipelinesworker")
			invalid := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-worker", Namespace: namespace.Name},
				Spec:       *worker.Spec.DeepCopy(),
			}
			invalid.Spec.Ports = []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{{Name: "api", Port: 9000}}
			createKubernetesObject(k8sClient, invalid)

			Eventually(func(g Gomega) {
				current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(invalid), current)).To(Succeed())
				g.Expect(meta.IsStatusConditionFalse(current.Status.Conditions, "Reconciled")).To(BeTrue())
				g.Expect(meta.FindStatusCondition(current.Status.Conditions, "Reconciled").Reason).To(Equal("InvalidConfiguration"))
				g.Expect(meta.IsStatusConditionFalse(current.Status.Conditions, "Available")).To(BeTrue())
				g.Expect(terminalReconcileErrors("datadogobservabilitypipelinesworker")).To(BeNumerically(">", terminalErrorsBefore))
			}, timeout, interval).Should(Succeed())
		})
	})
})

// workerWithUID reads the Worker back so owner checks can compare its UID.
func workerWithUID(g Gomega, worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker {
	current := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
	g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), current)).To(Succeed())
	return current
}
