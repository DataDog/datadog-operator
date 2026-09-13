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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

var _ = Describe("DatadogObservabilityPipelinesWorker API", func() {
	var namespace *corev1.Namespace
	var worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker

	BeforeEach(func() {
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "op-worker-api-test-"}}
		createKubernetesObject(k8sClient, namespace)
		worker = &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
			ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: namespace.Name},
			Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
					Site: ptr.To("datadoghq.com"),
					APIKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
						Key:                  "api-key",
					},
					AppKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
						Key:                  "app-key",
					},
				},
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.invalid/datadog/observability-pipelines-worker"),
					Digest:     ptr.To("sha256:" + strings.Repeat("a", 64)),
				},
			},
		}
	})

	AfterEach(func() {
		deleteKubernetesObject(k8sClient, namespace)
	})

	It("allows pipelineID to be set, changed, and removed", func() {
		createKubernetesObject(k8sClient, worker)
		for _, pipelineID := range []*string{ptr.To("pipeline-1"), ptr.To("pipeline-2"), nil} {
			worker.Spec.PipelineID = pipelineID
			Expect(k8sClient.Update(context.Background(), worker)).To(Succeed())
		}
	})

	DescribeTable("rejects invalid configuration at admission",
		func(mutate func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec)) {
			mutate(&worker.Spec)
			err := k8sClient.Create(context.Background(), worker)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "error: %v", err)
		},
		Entry("missing Datadog configuration", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) { spec.Datadog = nil }),
		Entry("missing site", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) { spec.Datadog.Site = nil }),
		Entry("missing API key reference", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) {
			spec.Datadog.APIKeySecretRef = nil
		}),
		Entry("missing application key reference", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) {
			spec.Datadog.AppKeySecretRef = nil
		}),
		Entry("missing image", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) { spec.Image = nil }),
		Entry("image without repository", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) {
			spec.Image.Repository = nil
		}),
		Entry("image without tag or digest", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) { spec.Image.Digest = nil }),
		Entry("image with tag and digest", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) {
			spec.Image.Tag = ptr.To("latest")
		}),
		Entry("invalid pull policy", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) {
			spec.Image.PullPolicy = ptr.To(corev1.PullPolicy("Sometimes"))
		}),
		Entry("empty pipeline ID", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) { spec.PipelineID = ptr.To("") }),
		Entry("empty ServiceAccount name", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) {
			spec.Identity = &datadoghqv1alpha1.DatadogBYOCClusterIdentitySpec{ServiceAccountName: ptr.To("")}
		}),
		Entry("resources without a memory limit", func(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec) {
			spec.Resources = &corev1.ResourceRequirements{}
		}),
	)

	It("is discoverable through the typed client", func() {
		createKubernetesObject(k8sClient, worker)
		got := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), got)).To(Succeed())
	})
})
