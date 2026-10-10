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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
)

const (
	byocSuccessReleaseTag = "success"
	byocUpdatedReleaseTag = "updated"
	byocFailureReleaseTag = "failure"
	byocPipelineID        = "pipeline-id"
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
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
				Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
					Metastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
						Database: &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{
							URISecretRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "byoc-postgres"},
								Key:                  "primary-uri",
							},
						},
					},
					Indexer:      &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
					Searcher:     &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
					Pipelines:    []datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{{Name: "logs", DatadogBYOCClusterPipelineComponentSpec: *byocTestPipeline()}},
					ControlPlane: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
					Janitor:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
					ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
						Database: &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{
							URISecretRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "byoc-postgres"},
								Key:                  "read-only-metastore-uri",
							},
						},
					},
					Compactor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
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
					Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
						AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
					},
					Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
						Metastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
							Database: &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{
								URISecretRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "byoc-postgres"},
									Key:                  "primary-uri",
								},
							},
						},
						Indexer:      &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Searcher:     &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Pipelines:    []datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{{Name: "logs", DatadogBYOCClusterPipelineComponentSpec: *byocTestPipeline()}},
						ControlPlane: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
						Janitor:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
						ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
							Database: &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{
								URISecretRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "byoc-postgres"},
									Key:                  "read-only-metastore-uri",
								},
							},
						},
						Compactor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
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

			resources := []client.Object{
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
		})

		It("becomes available once all workloads are available", func() {
			By("making the workloads ready")
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
			for _, name := range []string{"byoc-indexer", "byoc-searcher"} {
				markStatefulSetReady(name)
			}

			By("reporting Reconciled=True and Available=True")
			Eventually(func(g Gomega) {
				current := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(current.Status.Conditions, conditionReconciled)).To(BeTrue())
				g.Expect(meta.IsStatusConditionTrue(current.Status.Conditions, conditionAvailable)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

	})

	Context("when the cluster is deleted", func() {
		BeforeEach(func() {
			createKubernetesObject(k8sClient, cluster)
		})

		It("deletes the indexer before the parent", func() {
			indexer := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: namespace.Name}}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKeyFromObject(indexer), indexer)
			}, timeout, interval).Should(Succeed())

			By("deleting the parent")
			currentCluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), currentCluster)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), currentCluster)).To(Succeed())

			By("verifying that foreground indexer deletion starts before parent deletion finishes")
			Eventually(func(g Gomega) {
				currentIndexer := &appsv1.StatefulSet{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(indexer), currentIndexer)).To(Succeed())
				g.Expect(currentIndexer.DeletionTimestamp.IsZero()).To(BeFalse())
				g.Expect(currentIndexer.Finalizers).To(ContainElement(metav1.FinalizerDeleteDependents))

				currentCluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), currentCluster)).To(Succeed())
				g.Expect(currentCluster.Finalizers).To(ContainElement(datadogBYOCClusterFinalizer))
			}, timeout, interval).Should(Succeed())

			By("finishing the foreground deletion as the garbage collector would")
			removeBYOCTestFinalizers(indexer, metav1.FinalizerDeleteDependents)
			waitForBYOCTestDeletion(indexer)
			waitForBYOCTestDeletion(cluster)
		})
	})

	Context("when an unowned object has a managed name", func() {
		var configMap *corev1.ConfigMap

		BeforeEach(func() {
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: namespace.Name},
				Data:       map[string]string{"key": "value"},
			}
			createKubernetesObject(k8sClient, configMap)
			createKubernetesObject(k8sClient, cluster)
		})

		AfterEach(func() {
			deleteKubernetesObject(k8sClient, cluster)
		})

		It("reports a conflict without changing the object", func() {
			By("reporting Reconciled=False with reason Conflict")
			Eventually(func(g Gomega) {
				current := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
				g.Expect(meta.IsStatusConditionFalse(current.Status.Conditions, conditionReconciled)).To(BeTrue())
				g.Expect(meta.FindStatusCondition(current.Status.Conditions, conditionReconciled).Reason).To(Equal("Conflict"))
			}, timeout, interval).Should(Succeed())

			By("keeping the unowned ConfigMap unchanged")
			current := &corev1.ConfigMap{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(configMap), current)).To(Succeed())
			Expect(current.Data).To(Equal(map[string]string{"key": "value"}))
			Expect(current.OwnerReferences).To(BeEmpty())
		})
	})

	Context("when an unowned indexer exists", func() {
		var indexer *appsv1.StatefulSet

		BeforeEach(func() {
			// An invalid configuration keeps the controller from applying its own indexer.
			cluster.Spec.Provider.AWS = nil
			indexer = &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: namespace.Name},
				Spec: appsv1.StatefulSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "unrelated"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "unrelated"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "registry.invalid/app"}}},
					},
				},
			}
			createKubernetesObject(k8sClient, indexer)
			createKubernetesObject(k8sClient, cluster)
		})

		It("keeps the indexer when the cluster is deleted", func() {
			By("waiting for the finalizer")
			Eventually(func(g Gomega) {
				current := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
				g.Expect(current.Finalizers).To(ContainElement(datadogBYOCClusterFinalizer))
			}, timeout, interval).Should(Succeed())

			By("deleting the cluster")
			deleteKubernetesObject(k8sClient, cluster)

			By("verifying that the indexer is kept")
			current := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(indexer), current)).To(Succeed())
			Expect(current.DeletionTimestamp.IsZero()).To(BeTrue())
		})
	})

	Context("when the configuration is invalid", func() {
		BeforeEach(func() {
			cluster.Spec.Provider.AWS = nil
		})

		AfterEach(func() {
			deleteKubernetesObject(k8sClient, cluster)
		})

		It("reports the failure without retrying", func() {
			terminalErrorsBefore := byocTerminalReconcileErrors()
			createKubernetesObject(k8sClient, cluster)

			Eventually(func(g Gomega) {
				current := &datadoghqv1alpha1.DatadogBYOCCluster{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current)).To(Succeed())
				g.Expect(meta.IsStatusConditionFalse(current.Status.Conditions, conditionReconciled)).To(BeTrue())
				g.Expect(meta.FindStatusCondition(current.Status.Conditions, conditionReconciled).Reason).To(Equal("InvalidConfiguration"))
				g.Expect(meta.IsStatusConditionFalse(current.Status.Conditions, conditionAvailable)).To(BeTrue())
				g.Expect(byocTerminalReconcileErrors()).To(BeNumerically(">", terminalErrorsBefore))
			}, timeout, interval).Should(Succeed())
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
					Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
						AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
					},
					Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
						Metastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
							Database: &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{
								URISecretRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "byoc-postgres"},
									Key:                  "primary-uri",
								},
							},
						},
						Indexer:      &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Searcher:     &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Pipelines:    []datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{{Name: "logs", DatadogBYOCClusterPipelineComponentSpec: *byocTestPipeline()}},
						ControlPlane: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
						Janitor:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
						ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
							Database: &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{
								URISecretRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "byoc-postgres"},
									Key:                  "read-only-metastore-uri",
								},
							},
						},
						Compactor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
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

func completeFakeImageOverride(override *datadoghqv1alpha1.DatadogBYOCImageSpec) bool {
	return override != nil && ptr.Deref(override.Repository, "") != "" &&
		((ptr.Deref(override.Tag, "") != "") != (ptr.Deref(override.Digest, "") != ""))
}

func fakeResolvedImage(base byocimage.ResolvedImage, override *datadoghqv1alpha1.DatadogBYOCImageSpec) byocimage.ResolvedImage {
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

// byocTerminalReconcileErrors returns the number of reconcile errors that controller-runtime did not retry.
func byocTerminalReconcileErrors() float64 {
	families, err := metrics.Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, family := range families {
		if family.GetName() != "controller_runtime_terminal_reconcile_errors_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "controller" && label.GetValue() == "datadogbyoccluster" {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}
