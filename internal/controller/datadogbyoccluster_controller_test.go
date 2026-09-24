// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controller

import (
	"context"
	"errors"
	"slices"
	"time"

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
)

const (
	byocSuccessReleaseTag = "success"
	byocUpdatedReleaseTag = "updated"
	byocFailureReleaseTag = "failure"
	byocPipelineID        = "pipeline-id"
	testDeletionFinalizer = "test.datadoghq.com/deletion"
)

var _ = Describe("DatadogBYOCCluster Controller", func() {
	var (
		cluster   *datadoghqv1alpha1.DatadogBYOCCluster
		namespace *corev1.Namespace
	)

	BeforeEach(func() {
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "byoc-test-"},
		}
		createKubernetesObject(k8sClient, namespace)

		cluster = &datadoghqv1alpha1.DatadogBYOCCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: namespace.Name},
			Spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Tag: ptr.To(byocSuccessReleaseTag),
				},
				Datadog: &datadoghqv1alpha1.DatadogBYOCClusterDatadogSpec{
					APIKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
						Key:                  "api-key",
					},
				},
				Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
					Metastore:         &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
					Indexer:           &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
					Searcher:          &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
					Pipelines:         []datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{{Name: "logs", DatadogBYOCClusterPipelineComponentSpec: *byocTestPipeline()}},
					ControlPlane:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
					Janitor:           &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
					ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
					Compactor:         &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
				},
			},
		}
	})

	AfterEach(func() {
		deleteKubernetesObject(k8sClient, namespace)
	})

	Context("when release resolution succeeds", func() {
		BeforeEach(func() {
			createKubernetesObject(k8sClient, cluster)
		})

		AfterEach(func() {
			deleteKubernetesObject(k8sClient, cluster)
		})

		It("creates the managed resources and reports their status", func() {
			want := &datadoghqv1alpha1.DatadogBYOCCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "byoc",
					Namespace:  namespace.Name,
					Generation: 1,
					Finalizers: []string{datadogBYOCClusterFinalizer},
				},
				Spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
					Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
						Tag: ptr.To(byocSuccessReleaseTag),
					},
					Datadog: &datadoghqv1alpha1.DatadogBYOCClusterDatadogSpec{
						Site:          ptr.To("datadoghq.com"),
						BYOCTelemetry: ptr.To(true),
						APIKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
							Key:                  "api-key",
						},
						DogstatsdServer: &datadoghqv1alpha1.DatadogBYOCClusterDogstatsdServerSpec{
							Port: ptr.To[int32](8125),
						},
					},
					Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
						Metastore:         &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
						Indexer:           &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Searcher:          &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Pipelines:         []datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{{Name: "logs", DatadogBYOCClusterPipelineComponentSpec: *byocTestPipeline()}},
						ControlPlane:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
						Janitor:           &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
						ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
						Compactor:         &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
					},
				},
				Status: datadoghqv1alpha1.DatadogBYOCClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionReleaseResolved,
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 1,
							Reason:             "Resolved",
							Message:            "Workload images resolved successfully",
						},
						{
							Type:               conditionReconciled,
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 1,
							Reason:             "Reconciled",
							Message:            "Managed resources match the desired state",
						},
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 1,
							Reason:             "WorkloadsUnavailable",
							Message:            "One or more workloads are not yet available",
						},
					},
					Indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulSetStatus{
						ObservedGeneration: ptr.To[int64](0),
						Replicas:           ptr.To[int32](0),
						ReadyReplicas:      ptr.To[int32](0),
					},
					Searcher: &datadoghqv1alpha1.DatadogBYOCClusterStatefulSetStatus{
						ObservedGeneration: ptr.To[int64](0),
						Replicas:           ptr.To[int32](0),
						ReadyReplicas:      ptr.To[int32](0),
					},
					Pipelines: []datadoghqv1alpha1.DatadogBYOCClusterPipelineStatus{{
						Name: "logs", WorkerName: "byoc-pipeline-logs",
					}},
					Metastore: &datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus{
						Replicas:            ptr.To[int32](0),
						ReadyReplicas:       ptr.To[int32](0),
						UnavailableReplicas: ptr.To[int32](0),
						AvailableReplicas:   ptr.To[int32](0),
					},
					ControlPlane: &datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus{
						Replicas:            ptr.To[int32](0),
						ReadyReplicas:       ptr.To[int32](0),
						UnavailableReplicas: ptr.To[int32](0),
						AvailableReplicas:   ptr.To[int32](0),
					},
					Janitor: &datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus{
						Replicas:            ptr.To[int32](0),
						ReadyReplicas:       ptr.To[int32](0),
						UnavailableReplicas: ptr.To[int32](0),
						AvailableReplicas:   ptr.To[int32](0),
					},
					ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus{
						Replicas:            ptr.To[int32](0),
						ReadyReplicas:       ptr.To[int32](0),
						UnavailableReplicas: ptr.To[int32](0),
						AvailableReplicas:   ptr.To[int32](0),
					},
					Compactor: &datadoghqv1alpha1.DatadogBYOCClusterDeploymentStatus{
						Replicas:            ptr.To[int32](0),
						ReadyReplicas:       ptr.To[int32](0),
						UnavailableReplicas: ptr.To[int32](0),
						AvailableReplicas:   ptr.To[int32](0),
					},
				},
			}

			Eventually(func() string {
				got := &datadoghqv1alpha1.DatadogBYOCCluster{}
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), got); err != nil {
					return err.Error()
				}
				return cmp.Diff(want, got,
					cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion", "UID", "CreationTimestamp", "ManagedFields"),
					cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
				)
			}, timeout, interval).Should(BeEmpty())

			worker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{ObjectMeta: metav1.ObjectMeta{Name: "byoc-pipeline-logs", Namespace: namespace.Name}}
			resources := []client.Object{
				worker,
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: namespace.Name}},
				&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: namespace.Name}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "byoc-headless", Namespace: namespace.Name}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: namespace.Name}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "byoc-searcher", Namespace: namespace.Name}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "byoc-metastore", Namespace: namespace.Name}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "byoc-control-plane", Namespace: namespace.Name}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "byoc-janitor", Namespace: namespace.Name}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "byoc-read-only-metastore", Namespace: namespace.Name}},
				&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "byoc-compactor", Namespace: namespace.Name}},
				&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: namespace.Name}},
				&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "byoc-searcher", Namespace: namespace.Name}},
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "byoc-metastore", Namespace: namespace.Name}},
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "byoc-control-plane", Namespace: namespace.Name}},
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "byoc-janitor", Namespace: namespace.Name}},
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "byoc-read-only-metastore", Namespace: namespace.Name}},
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "byoc-compactor", Namespace: namespace.Name}},
				&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: namespace.Name}},
				&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "byoc-searcher", Namespace: namespace.Name}},
				&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: namespace.Name}},
				&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "byoc-searcher", Namespace: namespace.Name}},
				&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "byoc-metastore", Namespace: namespace.Name}},
				&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "byoc-control-plane", Namespace: namespace.Name}},
				&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "byoc-janitor", Namespace: namespace.Name}},
				&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "byoc-read-only-metastore", Namespace: namespace.Name}},
				&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "byoc-compactor", Namespace: namespace.Name}},
			}
			for _, resource := range resources {
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(resource), resource)).Should(Succeed())
			}
			Expect(metav1.IsControlledBy(worker, cluster)).To(BeTrue())
		})

		It("becomes available only when both workers are available", func() {
			By("adding a second pipeline")
			Eventually(func() error {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster); err != nil {
					return err
				}
				cluster.Spec.Components.Pipelines = []datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{
					{Name: "logs", DatadogBYOCClusterPipelineComponentSpec: *byocTestPipeline()},
					{Name: "audit", DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
						PipelineID: ptr.To("audit-pipeline-id"),
						Ports:      []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{{Name: "http", Port: 8080}},
					}},
				}
				return k8sClient.Update(context.Background(), cluster)
			}, timeout, interval).Should(Succeed())

			By("making the shared workloads and logs worker ready")
			// envtest does not run the Deployment or StatefulSet controllers.
			for _, name := range []string{"byoc-metastore", "byoc-control-plane", "byoc-janitor", "byoc-read-only-metastore", "byoc-compactor"} {
				Eventually(func() error {
					current := &appsv1.Deployment{}
					if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace.Name}, current); err != nil {
						return err
					}
					current.Status = appsv1.DeploymentStatus{
						ObservedGeneration: current.Generation,
						Replicas:           *current.Spec.Replicas, ReadyReplicas: *current.Spec.Replicas, AvailableReplicas: *current.Spec.Replicas,
					}
					return k8sClient.Status().Update(context.Background(), current)
				}, timeout, interval).Should(Succeed())
			}
			markStatefulSetReady := func(name string) {
				Eventually(func() error {
					current := &appsv1.StatefulSet{}
					if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace.Name}, current); err != nil {
						return err
					}
					current.Status = appsv1.StatefulSetStatus{
						ObservedGeneration: current.Generation,
						Replicas:           *current.Spec.Replicas, ReadyReplicas: *current.Spec.Replicas,
					}
					return k8sClient.Status().Update(context.Background(), current)
				}, timeout, interval).Should(Succeed())
			}
			for _, name := range []string{"byoc-indexer", "byoc-searcher", "byoc-pipeline-logs"} {
				markStatefulSetReady(name)
			}

			By("reporting Reconciled=True and Available=False while only logs is ready")
			Eventually(func(g Gomega) {
				logsWorker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc-pipeline-logs", Namespace: namespace.Name}, logsWorker)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(logsWorker.Status.Conditions, conditionAvailable)).To(BeTrue())
				current := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(current.Status.Conditions, conditionReconciled)).To(BeTrue())
				g.Expect(meta.IsStatusConditionFalse(current.Status.Conditions, conditionAvailable)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			By("reporting Available=True once audit is also ready")
			markStatefulSetReady("byoc-pipeline-audit")
			Eventually(func(g Gomega) {
				current := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(current.Status.Conditions, conditionAvailable)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("replaces the worker when a pipeline is renamed", func() {
			original := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc-pipeline-logs", Namespace: namespace.Name}, original)
			}, timeout, interval).Should(Succeed())

			Eventually(func() error {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster); err != nil {
					return err
				}
				cluster.Spec.Components.Pipelines[0].Name = "archive"
				return k8sClient.Update(context.Background(), cluster)
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				renamed := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc-pipeline-archive", Namespace: namespace.Name}, renamed)).To(Succeed())
				g.Expect(renamed.UID).NotTo(Equal(original.UID))
				g.Expect(cmp.Diff(original.Spec, renamed.Spec)).To(BeEmpty())
				obsolete := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(original), obsolete)).To(Succeed())
				g.Expect(obsolete.DeletionTimestamp.IsZero()).To(BeFalse())
			}, timeout, interval).Should(Succeed())

			// envtest does not run the garbage collector that clears foreground deletion.
			removeBYOCTestFinalizers(original, metav1.FinalizerDeleteDependents)
			waitForBYOCTestDeletion(original)
		})

		It("updates the existing worker when its pipeline ID changes", func() {
			worker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc-pipeline-logs", Namespace: namespace.Name}, worker)
			}, timeout, interval).Should(Succeed())
			originalUID := worker.UID

			Eventually(func() error {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster); err != nil {
					return err
				}
				cluster.Spec.Components.Pipelines[0].PipelineID = ptr.To("updated-pipeline-id")
				return k8sClient.Update(context.Background(), cluster)
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), worker)).To(Succeed())
				g.Expect(worker.UID).To(Equal(originalUID))
				g.Expect(worker.Spec.PipelineID).To(Equal(ptr.To("updated-pipeline-id")))
			}, timeout, interval).Should(Succeed())
		})

		It("reconciles named pipelines across additions, reordering, and removal", func() {
			logs := cluster.Spec.Components.Pipelines[0]
			audit := datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{
				Name: "audit",
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							Replicas: ptr.To[int32](3),
							Env:      []corev1.EnvVar{{Name: "AUDIT_ONLY", Value: "true"}},
						},
					},
					PipelineID: ptr.To("audit-pipeline-id"),
					Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{
						{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP},
					},
				},
			}
			setPipelines := func(pipelines ...datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec) {
				Eventually(func() error {
					if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster); err != nil {
						return err
					}
					cluster.Spec.Components.Pipelines = pipelines
					return k8sClient.Update(context.Background(), cluster)
				}, timeout, interval).Should(Succeed())
			}

			By("adding a pipeline with its own settings")
			setPipelines(logs, audit)
			wantLogs := datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
				DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
					DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						Replicas: ptr.To[int32](2),
						Env: []corev1.EnvVar{
							{Name: "DD_OP_DESTINATION_CLOUDPREM_ENDPOINT_URL", Value: "http://byoc-indexer:7280"},
						},
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
						Labels: map[string]string{
							"app.kubernetes.io/name":       "cloudprem",
							"app.kubernetes.io/instance":   "byoc",
							"app.kubernetes.io/component":  "pipeline",
							"app.kubernetes.io/managed-by": "datadog-operator",
						},
						PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
							MaxUnavailable: ptr.To(intstr.FromInt32(1)),
						},
						TerminationGracePeriodSeconds: ptr.To[int64](70),
					},
					Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
						VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
								Resources: corev1.VolumeResourceRequirements{
									Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("30Gi")},
								},
							},
						},
					},
				},
				PipelineID: ptr.To(byocPipelineID),
				Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{
					{Name: "otlp-grpc", Port: 4317, Protocol: corev1.ProtocolTCP},
					{Name: "otlp-http", Port: 4318, Protocol: corev1.ProtocolTCP},
				},
			}
			wantAudit := datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
				DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
					DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						Replicas: ptr.To[int32](3),
						Env: []corev1.EnvVar{
							{Name: "AUDIT_ONLY", Value: "true"},
							{Name: "DD_OP_DESTINATION_CLOUDPREM_ENDPOINT_URL", Value: "http://byoc-indexer:7280"},
						},
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
						Labels: map[string]string{
							"app.kubernetes.io/name":       "cloudprem",
							"app.kubernetes.io/instance":   "byoc",
							"app.kubernetes.io/component":  "pipeline",
							"app.kubernetes.io/managed-by": "datadog-operator",
						},
						PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
							MaxUnavailable: ptr.To(intstr.FromInt32(1)),
						},
						TerminationGracePeriodSeconds: ptr.To[int64](70),
					},
					Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
						VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
								Resources: corev1.VolumeResourceRequirements{
									Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("30Gi")},
								},
							},
						},
					},
				},
				PipelineID: ptr.To("audit-pipeline-id"),
				Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{
					{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP},
				},
			}
			logsWorker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			auditWorker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc-pipeline-logs", Namespace: namespace.Name}, logsWorker)).To(Succeed())
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc-pipeline-audit", Namespace: namespace.Name}, auditWorker)).To(Succeed())
				g.Expect(metav1.IsControlledBy(logsWorker, cluster)).To(BeTrue())
				g.Expect(metav1.IsControlledBy(auditWorker, cluster)).To(BeTrue())
				g.Expect(cmp.Diff(wantLogs, logsWorker.Spec.DatadogBYOCClusterPipelineComponentSpec)).To(BeEmpty())
				g.Expect(cmp.Diff(wantAudit, auditWorker.Spec.DatadogBYOCClusterPipelineComponentSpec)).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
			originalLogs := logsWorker.DeepCopy()
			originalAudit := auditWorker.DeepCopy()

			By("preserving worker identity and settings when pipelines are reordered")
			setPipelines(audit, logs)
			Eventually(func(g Gomega) {
				current := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
				condition := meta.FindStatusCondition(current.Status.Conditions, conditionReconciled)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(condition.ObservedGeneration).To(Equal(cluster.Generation))
				g.Expect(current.Status.Pipelines).To(ConsistOf(
					datadoghqv1alpha1.DatadogBYOCClusterPipelineStatus{Name: "logs", WorkerName: "byoc-pipeline-logs"},
					datadoghqv1alpha1.DatadogBYOCClusterPipelineStatus{Name: "audit", WorkerName: "byoc-pipeline-audit"},
				))
				for _, original := range []*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{originalLogs, originalAudit} {
					worker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(original), worker)).To(Succeed())
					g.Expect(worker.UID).To(Equal(original.UID))
					g.Expect(worker.Generation).To(Equal(original.Generation))
					g.Expect(cmp.Diff(original.Spec, worker.Spec)).To(BeEmpty())
				}
			}, timeout, interval).Should(Succeed())

			By("waiting for the removed pipeline to be deleted while retaining the other worker")
			addBYOCTestFinalizer(logsWorker)
			setPipelines(audit)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(logsWorker), logsWorker)).To(Succeed())
				g.Expect(logsWorker.DeletionTimestamp.IsZero()).To(BeFalse())
				current := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
				condition := meta.FindStatusCondition(current.Status.Conditions, conditionReconciled)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(condition.Reason).To(Equal("CleanupInProgress"))
				g.Expect(current.Status.Pipelines).To(ConsistOf(HaveField("Name", "audit")))
			}, timeout, interval).Should(Succeed())
			// envtest does not run the garbage collector that clears foreground deletion.
			removeBYOCTestFinalizers(logsWorker, testDeletionFinalizer, metav1.FinalizerDeleteDependents)
			waitForBYOCTestDeletion(logsWorker)
			Eventually(func(g Gomega) {
				current := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(current.Status.Conditions, conditionReconciled)).To(BeTrue())
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(auditWorker), auditWorker)).To(Succeed())
				g.Expect(auditWorker.UID).To(Equal(originalAudit.UID))
				g.Expect(auditWorker.DeletionTimestamp.IsZero()).To(BeTrue())
				g.Expect(cmp.Diff(originalAudit.Spec, auditWorker.Spec)).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
	})

	It("rejects an existing unowned worker with the expected name", func() {
		worker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
			ObjectMeta: metav1.ObjectMeta{Name: "byoc-pipeline-logs", Namespace: namespace.Name},
			Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					PipelineID: ptr.To("standalone-pipeline"),
					Ports:      []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{{Name: "otlp-grpc", Port: 4317}},
				},
				Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
					Site:            ptr.To("datadoghq.com"),
					APIKeySecretRef: cluster.Spec.Datadog.APIKeySecretRef.DeepCopy(),
				},
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.invalid/standalone-worker"),
					Tag:        ptr.To("existing"),
				},
			},
		}
		createKubernetesObject(k8sClient, worker)
		originalWorker := worker.DeepCopy()

		createKubernetesObject(k8sClient, cluster)
		DeferCleanup(deleteKubernetesObject, k8sClient, cluster)

		Eventually(func(g Gomega) {
			current := &datadoghqv1alpha1.DatadogBYOCCluster{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
			condition := meta.FindStatusCondition(current.Status.Conditions, conditionReconciled)
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Reason).To(Equal("WorkerReconcileFailed"))
			g.Expect(condition.Message).To(ContainSubstring("worker is not controlled by DatadogBYOCCluster"))
		}, timeout, interval).Should(Succeed())

		Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), worker)).To(Succeed())
		Expect(worker.UID).To(Equal(originalWorker.UID))
		Expect(worker.OwnerReferences).To(BeEmpty())
		Expect(cmp.Diff(originalWorker.Spec, worker.Spec)).To(BeEmpty())
	})

	Context("when the cluster is deleted", func() {
		BeforeEach(func() {
			cluster.Spec.Components.Pipelines = append(cluster.Spec.Components.Pipelines, datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{
				Name: "audit", DatadogBYOCClusterPipelineComponentSpec: *byocTestPipeline(),
			})
			createKubernetesObject(k8sClient, cluster)
		})

		It("deletes all workers before the indexer and the indexer before the parent", func() {
			worker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{ObjectMeta: metav1.ObjectMeta{Name: "byoc-pipeline-logs", Namespace: namespace.Name}}
			auditWorker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{ObjectMeta: metav1.ObjectMeta{Name: "byoc-pipeline-audit", Namespace: namespace.Name}}
			indexer := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: namespace.Name}}
			for _, object := range []client.Object{worker, auditWorker, indexer} {
				Eventually(func() error {
					return k8sClient.Get(context.Background(), client.ObjectKeyFromObject(object), object)
				}, timeout, interval).Should(Succeed())
			}

			By("holding the workers and indexer so each deletion phase can be observed")
			addBYOCTestFinalizer(worker)
			addBYOCTestFinalizer(auditWorker)
			addBYOCTestFinalizer(indexer)

			By("deleting the parent")
			currentCluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), currentCluster)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), currentCluster)).To(Succeed())

			By("verifying that worker deletion starts before indexer deletion")
			Eventually(func(g Gomega) {
				for _, desired := range []*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{worker, auditWorker} {
					currentWorker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(desired), currentWorker)).To(Succeed())
					g.Expect(currentWorker.DeletionTimestamp.IsZero()).To(BeFalse())
				}

				currentIndexer := &appsv1.StatefulSet{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(indexer), currentIndexer)).To(Succeed())
				g.Expect(currentIndexer.DeletionTimestamp.IsZero()).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			By("allowing one worker to finish while the other still blocks indexer deletion")
			// envtest does not run the Kubernetes garbage collector, so remove its
			// foreground-deletion finalizer along with the test finalizer.
			removeBYOCTestFinalizers(worker, testDeletionFinalizer, metav1.FinalizerDeleteDependents)
			waitForBYOCTestDeletion(worker)
			Consistently(func(g Gomega) {
				current := &appsv1.StatefulSet{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(indexer), current)).To(Succeed())
				g.Expect(current.DeletionTimestamp.IsZero()).To(BeTrue())
			}, 2*time.Second, interval).Should(Succeed())

			By("allowing the remaining worker to finish")
			removeBYOCTestFinalizers(auditWorker, testDeletionFinalizer, metav1.FinalizerDeleteDependents)
			waitForBYOCTestDeletion(auditWorker)

			By("verifying that indexer deletion starts before parent deletion finishes")
			Eventually(func(g Gomega) {
				currentIndexer := &appsv1.StatefulSet{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(indexer), currentIndexer)).To(Succeed())
				g.Expect(currentIndexer.DeletionTimestamp.IsZero()).To(BeFalse())

				currentCluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), currentCluster)).To(Succeed())
				g.Expect(currentCluster.Finalizers).To(ContainElement(datadogBYOCClusterFinalizer))
			}, timeout, interval).Should(Succeed())

			By("allowing indexer and parent deletion to finish")
			removeBYOCTestFinalizers(indexer, testDeletionFinalizer)
			waitForBYOCTestDeletion(indexer)
			waitForBYOCTestDeletion(cluster)
		})
	})

	Context("when release resolution fails", func() {
		BeforeEach(func() {
			cluster.Spec.Release.Tag = ptr.To(byocFailureReleaseTag)
			createKubernetesObject(k8sClient, cluster)
		})

		AfterEach(func() {
			deleteKubernetesObject(k8sClient, cluster)
		})

		It("reports the failure without creating managed resources", func() {
			want := &datadoghqv1alpha1.DatadogBYOCCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "byoc",
					Namespace:  namespace.Name,
					Generation: 1,
					Finalizers: []string{datadogBYOCClusterFinalizer},
				},
				Spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
					Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
						Tag: ptr.To(byocFailureReleaseTag),
					},
					Datadog: &datadoghqv1alpha1.DatadogBYOCClusterDatadogSpec{
						Site:          ptr.To("datadoghq.com"),
						BYOCTelemetry: ptr.To(true),
						APIKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
							Key:                  "api-key",
						},
						DogstatsdServer: &datadoghqv1alpha1.DatadogBYOCClusterDogstatsdServerSpec{
							Port: ptr.To[int32](8125),
						},
					},
					Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
						Metastore:         &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
						Indexer:           &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Searcher:          &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Pipelines:         []datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{{Name: "logs", DatadogBYOCClusterPipelineComponentSpec: *byocTestPipeline()}},
						ControlPlane:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
						Janitor:           &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
						ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
						Compactor:         &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
					},
				},
				Status: datadoghqv1alpha1.DatadogBYOCClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:               conditionReleaseResolved,
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 1,
							Reason:             "ResolutionFailed",
							Message:            "release unavailable",
						},
						{
							Type:               conditionReconciled,
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 1,
							Reason:             "ResolutionFailed",
							Message:            "release unavailable",
						},
						{
							Type:               conditionAvailable,
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 1,
							Reason:             "ResolutionFailed",
							Message:            "release unavailable",
						},
					},
				},
			}

			Eventually(func() string {
				got := &datadoghqv1alpha1.DatadogBYOCCluster{}
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), got); err != nil {
					return err.Error()
				}
				return cmp.Diff(want, got,
					cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion", "UID", "CreationTimestamp", "ManagedFields"),
					cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
				)
			}, timeout, interval).Should(BeEmpty())

			err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc", Namespace: namespace.Name}, &corev1.ConfigMap{})
			Expect(apierrors.IsNotFound(err)).Should(BeTrue())
		})
	})
})

type fakeBYOCImageResolver struct {
	results map[string]fakeBYOCImageResult
}

type fakeBYOCImageResult struct {
	images *byocimage.ResolvedImages
	err    error
}

func newFakeBYOCImageResolver() byocimage.ImageResolver {
	return &fakeBYOCImageResolver{
		results: map[string]fakeBYOCImageResult{
			byocSuccessReleaseTag: {
				images: &byocimage.ResolvedImages{
					Pomsky: byocimage.ResolvedImage{
						Repository: "registry.invalid/datadog/cloudprem",
						Tag:        "envtest",
					},
					ObservabilityPipelinesWorker: byocimage.ResolvedImage{
						Repository: "registry.invalid/datadog/observability-pipelines-worker",
						Tag:        "envtest",
					},
				},
			},
			byocUpdatedReleaseTag: {
				images: &byocimage.ResolvedImages{
					Pomsky: byocimage.ResolvedImage{
						Repository: "registry.invalid/datadog/cloudprem",
						Tag:        "envtest",
					},
					ObservabilityPipelinesWorker: byocimage.ResolvedImage{
						Repository: "registry.invalid/datadog/observability-pipelines-worker",
						Tag:        "envtest-updated",
					},
				},
			},
			byocFailureReleaseTag: {err: errors.New("release unavailable")},
		},
	}
}

func (r *fakeBYOCImageResolver) Resolve(_ context.Context, spec *datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec, overrides *datadoghqv1alpha1.DatadogBYOCClusterImageOverrides) (*byocimage.ResolvedImages, error) {
	if overrides != nil && completeFakeImageOverride(overrides.BYOC) && completeFakeImageOverride(overrides.ObservabilityPipelinesWorker) {
		return &byocimage.ResolvedImages{
			Pomsky:                       fakeResolvedImage(byocimage.ResolvedImage{}, overrides.BYOC),
			ObservabilityPipelinesWorker: fakeResolvedImage(byocimage.ResolvedImage{}, overrides.ObservabilityPipelinesWorker),
		}, nil
	}
	var tag string
	if spec != nil {
		tag = ptr.Deref(spec.Tag, "")
	}
	result, found := r.results[tag]
	if !found {
		return nil, errors.New("unexpected release")
	}
	if result.err != nil {
		return nil, result.err
	}
	images := *result.images
	if overrides != nil {
		images.Pomsky = fakeResolvedImage(images.Pomsky, overrides.BYOC)
		images.ObservabilityPipelinesWorker = fakeResolvedImage(images.ObservabilityPipelinesWorker, overrides.ObservabilityPipelinesWorker)
	}
	return &images, nil
}

func completeFakeImageOverride(override *datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec) bool {
	return override != nil && ptr.Deref(override.Repository, "") != "" &&
		((ptr.Deref(override.Tag, "") != "") != (ptr.Deref(override.Digest, "") != ""))
}

func fakeResolvedImage(base byocimage.ResolvedImage, override *datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec) byocimage.ResolvedImage {
	if override == nil {
		return base
	}
	if override.Repository != nil {
		base.Repository = *override.Repository
	}
	if override.Tag != nil {
		base.Tag, base.Digest = *override.Tag, ""
	} else if override.Digest != nil {
		base.Tag, base.Digest = "", *override.Digest
	}
	if override.PullPolicy != nil {
		base.ImagePullPolicy = *override.PullPolicy
	}
	base.ImagePullSecrets = slices.Clone(override.ImagePullSecrets)
	return base
}

func byocTestPipeline() *datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec {
	return &datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
		PipelineID: ptr.To(byocPipelineID),
		Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{
			{Name: "otlp-grpc", Port: 4317, Protocol: corev1.ProtocolTCP},
			{Name: "otlp-http", Port: 4318, Protocol: corev1.ProtocolTCP},
		},
	}
}

func addBYOCTestFinalizer(object client.Object) {
	Eventually(func() error {
		current := object.DeepCopyObject().(client.Object)
		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(object), current); err != nil {
			return err
		}
		if slices.Contains(current.GetFinalizers(), testDeletionFinalizer) {
			return nil
		}
		current.SetFinalizers(append(current.GetFinalizers(), testDeletionFinalizer))
		return k8sClient.Update(context.Background(), current)
	}, timeout, interval).Should(Succeed())
}

func removeBYOCTestFinalizers(object client.Object, finalizers ...string) {
	Eventually(func() error {
		current := object.DeepCopyObject().(client.Object)
		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(object), current); err != nil {
			return err
		}
		current.SetFinalizers(slices.DeleteFunc(current.GetFinalizers(), func(finalizer string) bool {
			return slices.Contains(finalizers, finalizer)
		}))
		return k8sClient.Update(context.Background(), current)
	}, timeout, interval).Should(Succeed())
}

func waitForBYOCTestDeletion(object client.Object) {
	Eventually(func() bool {
		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(object), object.DeepCopyObject().(client.Object))
		return apierrors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}
