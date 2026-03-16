//go:build integration
// +build integration

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	datadogV1 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

var (
	cfg        *rest.Config
	k8sClient  client.Client
	testEnv    *envtest.Environment
	reconciler *Reconciler
	httpServer *httptest.Server
	mgrCancel  context.CancelFunc
)

func TestMonitorRecreationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DatadogMonitor Recreation Integration Suite")
}

var _ = BeforeSuite(func() {
	logger := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(logger)

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = datadoghqv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	// Set up HTTP server for mocking Datadog API
	setupMockDatadogAPI()

	// Create reconciler
	setupReconciler(logger)

	// Start controller manager
	startControllerManager(logger)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if mgrCancel != nil {
		mgrCancel()
	}
	if httpServer != nil {
		httpServer.Close()
	}
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func setupMockDatadogAPI() {
	// Track created monitors for recreation testing
	createdMonitors := make(map[int]datadogV1.Monitor)
	nextMonitorID := 10000

	httpServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Handle different API endpoints
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "validate"):
			// Monitor validation endpoint
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))

		case r.Method == "POST" && strings.Contains(r.URL.Path, "monitor"):
			// Monitor creation endpoint
			nextMonitorID++
			monitor := genericMonitor(nextMonitorID)
			createdMonitors[nextMonitorID] = monitor

			jsonMonitor, _ := monitor.MarshalJSON()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMonitor)

		case r.Method == "GET" && strings.Contains(r.URL.Path, "monitor"):
			// Monitor retrieval endpoint
			var monitorID int
			fmt.Sscanf(r.URL.Path, "/api/v1/monitor/%d", &monitorID)

			if monitor, exists := createdMonitors[monitorID]; exists {
				jsonMonitor, _ := monitor.MarshalJSON()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(jsonMonitor)
			} else {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
			}

		case r.Method == "PUT" && strings.Contains(r.URL.Path, "monitor"):
			// Monitor update endpoint
			var monitorID int
			fmt.Sscanf(r.URL.Path, "/api/v1/monitor/%d", &monitorID)

			if monitor, exists := createdMonitors[monitorID]; exists {
				jsonMonitor, _ := monitor.MarshalJSON()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(jsonMonitor)
			} else {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors": ["Monitor not found"]}`))
			}

		case r.Method == "DELETE" && strings.Contains(r.URL.Path, "monitor"):
			// Monitor deletion endpoint (for simulating external deletion)
			var monitorID int
			fmt.Sscanf(r.URL.Path, "/api/v1/monitor/%d", &monitorID)

			delete(createdMonitors, monitorID)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"deleted_monitor_id": ` + fmt.Sprintf("%d", monitorID) + `}`))

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors": ["Endpoint not found"]}`))
		}
	}))
}

func setupReconciler(logger logr.Logger) {
	// Set up Datadog client
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewMonitorsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	// Create reconciler
	reconciler = &Reconciler{
		client:                 k8sClient,
		datadogClient:          client,
		datadogAuth:            testAuth,
		log:                    logger,
		scheme:                 scheme.Scheme,
		recorder:               record.NewFakeRecorder(100),
		operatorMetricsEnabled: false,
		forwarders:             datadog.NewForwardersManager(k8sClient, nil, false, nil),
	}
}

func startControllerManager(logger logr.Logger) {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	err = reconciler.SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	var mgrCtx context.Context
	mgrCtx, mgrCancel = context.WithCancel(context.Background())

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(mgrCtx)
		Expect(err).ToNot(HaveOccurred())
	}()
}

var _ = Describe("DatadogMonitor Recreation Integration Tests", func() {
	var (
		namespace string
		monitor   *datadoghqv1alpha1.DatadogMonitor
	)

	BeforeEach(func() {
		namespace = "test-namespace-" + fmt.Sprintf("%d", time.Now().UnixNano())

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).Should(Succeed())

		// Create DatadogMonitor
		monitor = &datadoghqv1alpha1.DatadogMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-monitor",
				Namespace: namespace,
			},
			Spec: datadoghqv1alpha1.DatadogMonitorSpec{
				Name:    "Integration Test Monitor",
				Message: "Test monitor for integration testing",
				Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
				Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
				Tags:    []string{"env:test", "integration:true"},
			},
		}
	})

	AfterEach(func() {
		// Clean up namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		_ = k8sClient.Delete(context.Background(), ns)
	})

	Context("End-to-End Monitor Recreation Workflow", func() {
		It("should create monitor, detect drift, and recreate successfully", func() {
			By("Creating DatadogMonitor resource")
			Expect(k8sClient.Create(context.Background(), monitor)).Should(Succeed())

			By("Waiting for monitor to be created in Datadog")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				if err != nil {
					return false
				}
				return monitor.Status.ID != 0
			}, time.Minute, time.Second).Should(BeTrue())

			originalMonitorID := monitor.Status.ID
			Expect(originalMonitorID).To(BeNumerically(">", 0))

			By("Verifying monitor status is healthy")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				if err != nil {
					return false
				}

				// Check for Created condition
				for _, condition := range monitor.Status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeCreated &&
						condition.Status == corev1.ConditionTrue {
						return true
					}
				}
				return false
			}, time.Minute, time.Second).Should(BeTrue())

			By("Simulating external monitor deletion")
			// Delete monitor directly from mock API to simulate external deletion
			req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/monitor/%d", httpServer.URL, originalMonitorID), nil)
			resp, err := httpServer.Client().Do(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			By("Waiting for drift detection and recreation")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				if err != nil {
					return false
				}

				// Check if monitor was recreated (new ID)
				return monitor.Status.ID != 0 && monitor.Status.ID != originalMonitorID
			}, 2*time.Minute, 5*time.Second).Should(BeTrue())

			By("Verifying recreation conditions are set")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				if err != nil {
					return false
				}

				hasDriftDetected := false
				hasRecreated := false

				for _, condition := range monitor.Status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected &&
						condition.Status == corev1.ConditionTrue {
						hasDriftDetected = true
					}
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeRecreated &&
						condition.Status == corev1.ConditionTrue {
						hasRecreated = true
					}
				}

				return hasDriftDetected && hasRecreated
			}, time.Minute, time.Second).Should(BeTrue())

			By("Verifying monitor configuration is preserved")
			Expect(monitor.Spec.Name).To(Equal("Integration Test Monitor"))
			Expect(monitor.Spec.Query).To(Equal("avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1"))
			Expect(monitor.Spec.Type).To(Equal(datadoghqv1alpha1.DatadogMonitorTypeMetric))
			Expect(monitor.Spec.Tags).To(ContainElements("env:test", "integration:true"))

			By("Verifying status fields are updated correctly")
			Expect(monitor.Status.Primary).To(BeTrue())
			Expect(monitor.Status.CurrentHash).ToNot(BeEmpty())
			Expect(monitor.Status.Created).ToNot(BeNil())
		})

		It("should handle multiple monitors independently", func() {
			By("Creating multiple DatadogMonitor resources")
			monitors := make([]*datadoghqv1alpha1.DatadogMonitor, 3)
			for i := 0; i < 3; i++ {
				monitors[i] = &datadoghqv1alpha1.DatadogMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test-monitor-%d", i),
						Namespace: namespace,
					},
					Spec: datadoghqv1alpha1.DatadogMonitorSpec{
						Name:    fmt.Sprintf("Integration Test Monitor %d", i),
						Message: fmt.Sprintf("Test monitor %d for integration testing", i),
						Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
						Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
						Tags:    []string{fmt.Sprintf("monitor:%d", i), "integration:true"},
					},
				}
				Expect(k8sClient.Create(context.Background(), monitors[i])).Should(Succeed())
			}

			By("Waiting for all monitors to be created")
			for i := 0; i < 3; i++ {
				Eventually(func() bool {
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      monitors[i].Name,
						Namespace: monitors[i].Namespace,
					}, monitors[i])
					if err != nil {
						return false
					}
					return monitors[i].Status.ID != 0
				}, time.Minute, time.Second).Should(BeTrue())
			}

			By("Simulating external deletion of one monitor")
			originalID := monitors[1].Status.ID
			req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/monitor/%d", httpServer.URL, originalID), nil)
			resp, err := httpServer.Client().Do(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			By("Verifying only the affected monitor is recreated")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitors[1].Name,
					Namespace: monitors[1].Namespace,
				}, monitors[1])
				if err != nil {
					return false
				}
				return monitors[1].Status.ID != 0 && monitors[1].Status.ID != originalID
			}, 2*time.Minute, 5*time.Second).Should(BeTrue())

			By("Verifying other monitors are unaffected")
			for i := 0; i < 3; i++ {
				if i == 1 {
					continue // Skip the recreated monitor
				}

				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitors[i].Name,
					Namespace: monitors[i].Namespace,
				}, monitors[i])
				Expect(err).ToNot(HaveOccurred())

				// Verify no drift detected condition for unaffected monitors
				hasDriftDetected := false
				for _, condition := range monitors[i].Status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected &&
						condition.Status == corev1.ConditionTrue {
						hasDriftDetected = true
						break
					}
				}
				Expect(hasDriftDetected).To(BeFalse())
			}
		})

		It("should handle recreation failures gracefully", func() {
			By("Creating DatadogMonitor resource")
			Expect(k8sClient.Create(context.Background(), monitor)).Should(Succeed())

			By("Waiting for monitor to be created")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				if err != nil {
					return false
				}
				return monitor.Status.ID != 0
			}, time.Minute, time.Second).Should(BeTrue())

			originalMonitorID := monitor.Status.ID

			By("Updating monitor with invalid configuration")
			monitor.Spec.Query = "invalid query syntax"
			Expect(k8sClient.Update(context.Background(), monitor)).Should(Succeed())

			By("Simulating external monitor deletion")
			req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/monitor/%d", httpServer.URL, originalMonitorID), nil)
			resp, err := httpServer.Client().Do(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			By("Verifying drift is detected but recreation fails")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				if err != nil {
					return false
				}

				// Check for drift detected condition
				for _, condition := range monitor.Status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected &&
						condition.Status == corev1.ConditionTrue {
						return true
					}
				}
				return false
			}, 2*time.Minute, 5*time.Second).Should(BeTrue())

			By("Verifying error condition is set for failed recreation")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				if err != nil {
					return false
				}

				// Check for error condition
				for _, condition := range monitor.Status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError &&
						condition.Status == corev1.ConditionTrue {
						return true
					}
				}
				return false
			}, time.Minute, time.Second).Should(BeTrue())

			By("Verifying original monitor ID is preserved on failure")
			Expect(monitor.Status.ID).To(Equal(originalMonitorID))
		})
	})

	Context("Event Emission Integration", func() {
		It("should emit Kubernetes events for recreation", func() {
			By("Creating DatadogMonitor resource")
			Expect(k8sClient.Create(context.Background(), monitor)).Should(Succeed())

			By("Waiting for monitor creation")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				return err == nil && monitor.Status.ID != 0
			}, time.Minute, time.Second).Should(BeTrue())

			originalMonitorID := monitor.Status.ID

			By("Simulating external monitor deletion")
			req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/monitor/%d", httpServer.URL, originalMonitorID), nil)
			resp, err := httpServer.Client().Do(req)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			By("Waiting for recreation")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				return err == nil && monitor.Status.ID != 0 && monitor.Status.ID != originalMonitorID
			}, 2*time.Minute, 5*time.Second).Should(BeTrue())

			By("Verifying Kubernetes events are created")
			eventList := &corev1.EventList{}
			Eventually(func() bool {
				err := k8sClient.List(context.Background(), eventList, client.InNamespace(namespace))
				if err != nil {
					return false
				}

				// Look for recreation event
				for _, event := range eventList.Items {
					if event.InvolvedObject.Name == monitor.Name &&
						event.InvolvedObject.Kind == "DatadogMonitor" &&
						strings.Contains(event.Reason, "Recreate") {
						return true
					}
				}
				return false
			}, time.Minute, time.Second).Should(BeTrue())
		})
	})

	Context("Status Consistency Integration", func() {
		It("should maintain consistent status across reconciliation cycles", func() {
			By("Creating DatadogMonitor resource")
			Expect(k8sClient.Create(context.Background(), monitor)).Should(Succeed())

			By("Waiting for initial creation and status update")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				if err != nil {
					return false
				}

				return monitor.Status.ID != 0 &&
					monitor.Status.Primary &&
					monitor.Status.CurrentHash != "" &&
					monitor.Status.Created != nil
			}, time.Minute, time.Second).Should(BeTrue())

			By("Capturing initial status")
			initialStatus := monitor.Status.DeepCopy()

			By("Triggering multiple reconciliation cycles")
			for i := 0; i < 5; i++ {
				// Update a label to trigger reconciliation
				if monitor.Labels == nil {
					monitor.Labels = make(map[string]string)
				}
				monitor.Labels["test-cycle"] = fmt.Sprintf("%d", i)
				Expect(k8sClient.Update(context.Background(), monitor)).Should(Succeed())

				time.Sleep(2 * time.Second)
			}

			By("Verifying status consistency")
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      monitor.Name,
					Namespace: monitor.Namespace,
				}, monitor)
				if err != nil {
					return false
				}

				// Verify key status fields remain consistent
				return monitor.Status.ID == initialStatus.ID &&
					monitor.Status.Primary == initialStatus.Primary &&
					monitor.Status.Creator == initialStatus.Creator &&
					monitor.Status.Created.Equal(initialStatus.Created)
			}, time.Minute, time.Second).Should(BeTrue())
		})
	})
})
