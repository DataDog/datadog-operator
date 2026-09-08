// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controller

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

var _ = Describe("DatadogBYOCCluster image overrides", func() {
	var cluster *datadoghqv1alpha1.DatadogBYOCCluster
	var namespace *corev1.Namespace
	const repository = "private.example.com/pomsky"
	const hotfix = "hotfix-123"
	digest := "sha256:" + strings.Repeat("a", 64)

	BeforeEach(func() {
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "byoc-images-test-"}}
		createKubernetesObject(k8sClient, namespace)
		cluster = &datadoghqv1alpha1.DatadogBYOCCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: namespace.Name},
			Spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(byocSuccessReleaseTag)},
				ImageOverrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
					BYOC: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec{
						Repository: ptr.To(repository), Tag: ptr.To(hotfix),
						ImagePullSecrets: []corev1.LocalObjectReference{{Name: "pomsky-registry"}},
					},
					ObservabilityPipelinesWorker: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec{
						Repository: ptr.To("private.example.com/worker"), Digest: ptr.To(digest),
						ImagePullSecrets: []corev1.LocalObjectReference{{Name: "worker-registry"}},
					},
				},
				Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
					Metastore:    &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
					Indexer:      &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
					Searcher:     &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
					ControlPlane: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
					Janitor:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
				},
			},
		}
	})
	AfterEach(func() {
		deleteKubernetesObject(k8sClient, namespace)
	})

	DescribeTable("skips release resolution when both images are fully specified",
		func(omitRelease bool) {
			// The fake resolver fails for this tag. Successful reconciliation proves it was bypassed.
			cluster.Spec.Release.Tag = ptr.To(byocFailureReleaseTag)
			if omitRelease {
				cluster.Spec.Release = nil
			}
			createKubernetesObject(k8sClient, cluster)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
				condition := meta.FindStatusCondition(cluster.Status.Conditions, conditionReleaseResolved)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(condition.Reason).To(Equal("Resolved"))
				g.Expect(condition.ObservedGeneration).To(Equal(cluster.Generation))
				g.Expect(meta.IsStatusConditionTrue(cluster.Status.Conditions, conditionReconciled)).To(BeTrue())
				g.Expect(meta.FindStatusCondition(cluster.Status.Conditions, "ImageOverridesActive")).To(BeNil())
				statefulSet := &appsv1.StatefulSet{}
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc-indexer", Namespace: namespace.Name}, statefulSet)).To(Succeed())
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal(repository + ":" + hotfix))
				g.Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{{Name: "pomsky-registry"}}))
			}, timeout, interval).Should(Succeed())
		},
		Entry("without a release", true),
		Entry("with an unreachable release", false),
	)

	It("resumes release resolution for partial overrides and restores release images after removal", func() {
		createKubernetesObject(k8sClient, cluster)
		waitForReason := func(reason string) {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
				condition := meta.FindStatusCondition(cluster.Status.Conditions, conditionReleaseResolved)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Reason).To(Equal(reason))
				g.Expect(condition.ObservedGeneration).To(Equal(cluster.Generation))
			}, timeout, interval).Should(Succeed())
		}
		updateSpec := func(update func(*datadoghqv1alpha1.DatadogBYOCClusterSpec)) {
			Eventually(func() error {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster); err != nil {
					return err
				}
				update(&cluster.Spec)
				return k8sClient.Update(context.Background(), cluster)
			}, timeout, interval).Should(Succeed())
		}
		waitForReason("Resolved")
		updateSpec(func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) {
			spec.Release.Tag = ptr.To(byocFailureReleaseTag)
			spec.ImageOverrides.ObservabilityPipelinesWorker.Digest = nil
		})
		waitForReason("ResolutionFailed")
		updateSpec(func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) {
			spec.Release.Tag = ptr.To(byocSuccessReleaseTag)
			spec.ImageOverrides.BYOC.Tag = nil
		})
		waitForReason("Resolved")
		statefulSet := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc-indexer", Namespace: namespace.Name}, statefulSet)).To(Succeed())
		images, err := newFakeBYOCImageResolver().Resolve(context.Background(), cluster.Spec.Release, nil)
		Expect(err).NotTo(HaveOccurred())
		mirroredImage := images.Pomsky
		mirroredImage.Repository = repository
		Expect(statefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal(mirroredImage.ImageReference()))

		updateSpec(func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) { spec.ImageOverrides = nil })
		waitForReason("Resolved")
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "byoc-indexer", Namespace: namespace.Name}, statefulSet)).To(Succeed())
		Expect(statefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal(images.Pomsky.ImageReference()))
		Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(BeEmpty())
	})

	DescribeTable("rejects invalid image configuration at admission",
		func(mutate func(*datadoghqv1alpha1.DatadogBYOCClusterSpec)) {
			mutate(&cluster.Spec)
			err := k8sClient.Create(context.Background(), cluster)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "error: %v", err)
		},
		Entry("missing release and overrides", func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) { spec.Release, spec.ImageOverrides = nil, nil }),
		Entry("missing release and one image", func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) {
			spec.Release = nil
			spec.ImageOverrides.ObservabilityPipelinesWorker = nil
		}),
		Entry("missing release and incomplete worker", func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) {
			spec.Release = nil
			spec.ImageOverrides.ObservabilityPipelinesWorker.Digest = nil
		}),
		Entry("missing release and incomplete Pomsky", func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) {
			spec.Release = nil
			spec.ImageOverrides.BYOC.Tag = nil
		}),
		Entry("tag and digest", func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) {
			spec.ImageOverrides.BYOC.Digest = ptr.To(digest)
		}),
		Entry("empty repository", func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) {
			spec.ImageOverrides.BYOC.Repository = ptr.To("")
		}),
		Entry("empty tag", func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) { spec.ImageOverrides.BYOC.Tag = ptr.To("") }),
		Entry("invalid release even when bypassed", func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) { spec.Release.Digest = ptr.To(digest) }),
		Entry("duplicate pull secrets", func(spec *datadoghqv1alpha1.DatadogBYOCClusterSpec) {
			spec.ImageOverrides.BYOC.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "duplicate"}, {Name: "duplicate"}}
		}),
	)
})
