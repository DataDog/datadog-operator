// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	splIntegrationID   = "pl-integration-id"
	splIntegrationNS   = "default"
	splTimeout         = "20s"
	splPollingInterval = "200ms"
)

// splDDMock is a mock Datadog Synthetics API backing the whole integration
// suite. It is started in init() so that DD_URL is set before the suite's
// CredentialManager is constructed, and lives for the whole test process.
var splDDMock = newSPLDDMock()

func init() {
	_ = os.Setenv("DD_URL", splDDMock.server.URL)
}

type splDDMockServer struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []string
}

func newSPLDDMock() *splDDMockServer {
	m := &splDDMockServer{}

	creationResponse := `{"config": {"id": "` + splIntegrationID + `", "accessKey": "ak", "secretAccessKey": "sk", "publicKey": "pk", "privateKey": "priv"},
		"private_location": {"id": "` + splIntegrationID + `", "name": "my private location", "description": "integration test", "tags": ["generated:kubernetes"]}}`
	privateLocationResponse := `{"id": "` + splIntegrationID + `", "name": "my private location", "description": "integration test", "tags": ["generated:kubernetes"]}`

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.calls = append(m.calls, r.Method+" "+r.URL.Path)
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(creationResponse))
		case r.Method == http.MethodGet || r.Method == http.MethodPut:
			_, _ = w.Write([]byte(privateLocationResponse))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))

	return m
}

func (m *splDDMockServer) callCount(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, c := range m.calls {
		if strings.HasPrefix(c, method+" ") {
			count++
		}
	}
	return count
}

func newSPLIntegrationInstance(name string) *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation {
	return &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   splIntegrationNS,
			Annotations: map[string]string{datadoghqv1alpha1.DatadogSPLStatusProbesEnabledAnnotation: "true"},
		},
		Spec: datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSpec{
			Name:        "my private location",
			Description: "integration test",
			Tags:        []string{"env:integration"},
			Worker: &datadoghqv1alpha1.DatadogSPLWorker{
				Replicas: ptr.To(int32(2)),
				Config: &datadoghqv1alpha1.DatadogSPLWorkerConfig{
					Concurrency: ptr.To(int32(10)),
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				PodDisruptionBudget: &datadoghqv1alpha1.DatadogSPLPodDisruptionBudget{
					Enabled:      true,
					MinAvailable: ptr.To(intstr.FromInt(1)),
				},
			},
		},
	}
}

var _ = Describe("DatadogSyntheticsPrivateLocation Controller", func() {
	ctx := context.Background()
	splName := "integration-pl"
	splKey := types.NamespacedName{Namespace: splIntegrationNS, Name: splName}

	AfterEach(func() {
		instance := &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{}
		if err := k8sClient.Get(ctx, splKey, instance); err == nil {
			Expect(k8sClient.Delete(ctx, instance)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, splKey, &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{})
				return apierrors.IsNotFound(err)
			}, splTimeout, splPollingInterval).Should(BeTrue())
		}
	})

	It("should create the remote private location and owned worker resources", func() {
		By("creating the DatadogSyntheticsPrivateLocation")
		instance := newSPLIntegrationInstance(splName)
		Expect(k8sClient.Create(ctx, instance)).To(Succeed())

		By("creating the private location in Datadog")
		Eventually(func() int {
			return splDDMock.callCount(http.MethodPost)
		}, splTimeout, splPollingInterval).Should(BeNumerically(">=", 1))

		By("reporting the remote ID in the status")
		Eventually(func() string {
			fresh := &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{}
			if err := k8sClient.Get(ctx, splKey, fresh); err != nil {
				return ""
			}
			return fresh.Status.ID
		}, splTimeout, splPollingInterval).Should(Equal(splIntegrationID))

		By("creating the worker ServiceAccount with an owner reference")
		sa := &corev1.ServiceAccount{}
		Eventually(func() error {
			return k8sClient.Get(ctx, splKey, sa)
		}, splTimeout, splPollingInterval).Should(Succeed())
		Expect(sa.OwnerReferences).To(HaveLen(1))
		Expect(sa.OwnerReferences[0].Name).To(Equal(splName))

		By("creating the worker config Secret with the Datadog config")
		secret := &corev1.Secret{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Namespace: splIntegrationNS, Name: splName + "-config"}, secret)
		}, splTimeout, splPollingInterval).Should(Succeed())
		Expect(secret.Data).To(HaveKey(datadoghqv1alpha1.DatadogSPLConfigSecretDataKey))
		Expect(secret.OwnerReferences).To(HaveLen(1))

		By("creating the worker Deployment with the spec replicas and image")
		deployment := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, splKey, deployment)
		}, splTimeout, splPollingInterval).Should(Succeed())
		Expect(int32PtrValue(deployment.Spec.Replicas)).To(Equal(int32(2)))
		Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("gcr.io/datadoghq/synthetics-private-location-worker:1.73.0"))
		Expect(deployment.OwnerReferences).To(HaveLen(1))
		Expect(deployment.Spec.Template.Spec.Containers[0].LivenessProbe).NotTo(BeNil())
		Expect(deployment.Spec.Template.Spec.Containers[0].ReadinessProbe).NotTo(BeNil())

		By("creating the PodDisruptionBudget when enabled")
		pdb := &policyv1.PodDisruptionBudget{}
		Eventually(func() error {
			return k8sClient.Get(ctx, splKey, pdb)
		}, splTimeout, splPollingInterval).Should(Succeed())
		Expect(pdb.OwnerReferences).To(HaveLen(1))
	})

	It("should update the worker Deployment when the spec changes", func() {
		By("recreating the DatadogSyntheticsPrivateLocation")
		instance := newSPLIntegrationInstance(splName)
		Expect(k8sClient.Create(ctx, instance)).To(Succeed())

		By("waiting for the initial creation to settle")
		Eventually(func() string {
			fresh := &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{}
			if err := k8sClient.Get(ctx, splKey, fresh); err != nil {
				return ""
			}
			return fresh.Status.ID
		}, splTimeout, splPollingInterval).Should(Equal(splIntegrationID))

		putCallsBefore := splDDMock.callCount(http.MethodPut)

		By("bumping the replica count")
		Eventually(func() error {
			fresh := &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{}
			if err := k8sClient.Get(ctx, splKey, fresh); err != nil {
				return err
			}
			fresh.Spec.Worker.Replicas = ptr.To(int32(3))
			return k8sClient.Update(ctx, fresh)
		}, splTimeout, splPollingInterval).Should(Succeed())

		By("updating the private location in Datadog")
		Eventually(func() int {
			return splDDMock.callCount(http.MethodPut)
		}, splTimeout, splPollingInterval).Should(BeNumerically(">", putCallsBefore))

		By("updating the worker Deployment replicas")
		deployment := &appsv1.Deployment{}
		Eventually(func() int32 {
			if err := k8sClient.Get(ctx, splKey, deployment); err != nil {
				return 0
			}
			return int32PtrValue(deployment.Spec.Replicas)
		}, splTimeout, splPollingInterval).Should(Equal(int32(3)))
	})

	It("should delete the remote private location and clear the finalizer", func() {
		By("recreating the DatadogSyntheticsPrivateLocation")
		instance := newSPLIntegrationInstance(splName)
		Expect(k8sClient.Create(ctx, instance)).To(Succeed())

		By("waiting for the initial creation to settle")
		Eventually(func() string {
			fresh := &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{}
			if err := k8sClient.Get(ctx, splKey, fresh); err != nil {
				return ""
			}
			return fresh.Status.ID
		}, splTimeout, splPollingInterval).Should(Equal(splIntegrationID))

		By("deleting the DatadogSyntheticsPrivateLocation")
		Expect(k8sClient.Delete(ctx, instance)).To(Succeed())

		By("deleting the private location in Datadog")
		Eventually(func() int {
			return splDDMock.callCount(http.MethodDelete)
		}, splTimeout, splPollingInterval).Should(BeNumerically(">=", 1))

		By("letting the object go away once the finalizer clears")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, splKey, &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{})
			return apierrors.IsNotFound(err)
		}, splTimeout, splPollingInterval).Should(BeTrue())
	})
})

func int32PtrValue(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}
