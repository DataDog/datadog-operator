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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
)

const (
	byocSuccessReleaseTag = "success"
	byocFailureReleaseTag = "failure"
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
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{},
				Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
					Metastore:         &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
					Indexer:           &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
					Searcher:          &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
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
			cluster.Spec.Release.Tag = ptr.To(byocSuccessReleaseTag)
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
						DogstatsdServer: &datadoghqv1alpha1.DatadogBYOCClusterDogstatsdServerSpec{
							Port: ptr.To[int32](8125),
						},
					},
					Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
						Metastore:         &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
						Indexer:           &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Searcher:          &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
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
							Message:            "Release artifact resolved successfully",
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
						DogstatsdServer: &datadoghqv1alpha1.DatadogBYOCClusterDogstatsdServerSpec{
							Port: ptr.To[int32](8125),
						},
					},
					Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
						Metastore:         &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
						Indexer:           &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
						Searcher:          &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}},
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
	base.ImagePullSecrets = slices.Clone(override.ImagePullSecrets)
	return base
}
