// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/testutils"
)

const (
	timeout  = 10 * time.Second
	interval = 100 * time.Millisecond
)

// These tests verify that a DatadogAgent deployment is successful.
//
// The function that checks if a deployment is successful is
// checkAgentDeployment(). At the moment, it checks these things:
//   - The DatadogAgent status contains information about the agent and the DCA.
//   - The Agent DaemonSet has been deployed.
//   - The DCA Deployment has been deployed.
//
// These tests allow us to catch errors like the operator trying to create an
// invalid Kubernetes resource (RBAC, deployment without a name, etc.). However,
// these tests don't use a container runtime by default (they run with
// USE_EXISTING_CLUSTER=false). Therefore, these tests are not useful to catch
// errors that crash containers and keep them in "CrashLoopBackOff" state.
var _ = Describe("V2 Controller - DatadogAgent Deployment", func() {
	namespace := "default"

	Context(
		"with no features enabled",
		testFunction(testutils.NewDatadogAgentWithoutFeatures(namespace, "basic")),
	)

	Context(
		"with admission controller enabled",
		testFunction(testutils.NewDatadogAgentWithAdmissionController(namespace, "with-admission-controller")),
	)

	Context(
		"with admission controller enabled, CWS Instrumentation enabled",
		testFunction(testutils.NewDatadogAgentWithCWSInstrumentation(namespace, "with-cws-instrumentation")),
	)

	Context(
		"with APM enabled",
		testFunction(testutils.NewDatadogAgentWithAPM(namespace, "with-apm")),
	)

	Context(
		"with cluster checks enabled",
		testFunction(testutils.NewDatadogAgentWithClusterChecks(namespace, "with-cluster-checks")),
	)

	Context(
		"with CSPM enabled",
		testFunction(testutils.NewDatadogAgentWithCSPM(namespace, "with-cspm")),
	)

	Context(
		"with CWS enabled",
		testFunction(testutils.NewDatadogAgentWithCWS(namespace, "with-cws")),
	)

	Context(
		"with Dogstatsd enabled",
		testFunction(testutils.NewDatadogAgentWithDogstatsd(namespace, "with-dogstatsd")),
	)

	Context(
		"with eBPF check enabled",
		testFunction(testutils.NewDatadogAgentWithEBPFCheck(namespace, "with-ebpfcheck")),
	)

	Context(
		"with service discovery enabled",
		testFunction(testutils.NewDatadogAgentWithServiceDiscovery(namespace, "with-service-discovery")),
	)

	Context(
		"with Event Collection",
		testFunction(testutils.NewDatadogAgentWithEventCollection(namespace, "with-event-collection")),
	)

	Context(
		"with External Metrics Server",
		testFunction(testutils.NewDatadogAgentWithExternalMetrics(namespace, "with-external-metrics")),
	)

	Context(
		"with KSM core",
		testFunction(testutils.NewDatadogAgentWithKSM(namespace, "with-ksm")),
	)

	Context(
		"with live container collection",
		testFunction(testutils.NewDatadogAgentWithLiveContainerCollection(namespace, "with-live-container-collection")),
	)

	Context(
		"with live process collection",
		testFunction(testutils.NewDatadogAgentWithLiveProcessCollection(namespace, "with-live-process-collection")),
	)

	Context(
		"with log collection",
		testFunction(testutils.NewDatadogAgentWithLogCollection(namespace, "with-log-collection")),
	)

	Context(
		"with NPM",
		testFunction(testutils.NewDatadogAgentWithNPM(namespace, "with-npm")),
	)

	Context(
		"with OOM Kill",
		testFunction(testutils.NewDatadogAgentWithOOMKill(namespace, "with-oom-kill")),
	)

	Context(
		"with orchestrator explorer",
		testFunction(testutils.NewDatadogAgentWithOrchestratorExplorer(namespace, "with-orchestrator-explorer")),
	)

	Context(
		"with OTLP",
		testFunction(testutils.NewDatadogAgentWithOTLP(namespace, "with-otlp")),
	)

	Context(
		"with Prometheus scrape",
		testFunction(testutils.NewDatadogAgentWithPrometheusScrape(namespace, "with-prometheus-scrape")),
	)

	Context(
		"with TCP queue length",
		testFunction(testutils.NewDatadogAgentWithTCPQueueLength(namespace, "with-tcp-queue-length")),
	)

	Context(
		"with USM",
		testFunction(testutils.NewDatadogAgentWithUSM(namespace, "with-usm")),
	)

	Context(
		"with some global settings set",
		testFunction(testutils.NewDatadogAgentWithGlobalConfigSettings(namespace, "with-global-settings")),
	)

	Context(
		"with overrides",
		testFunction(testutils.NewDatadogAgentWithOverrides(namespace, "with-overrides")),
	)

	Context(
		"with GPU monitoring",
		testFunction(testutils.NewDatadogAgentWithGPUMonitoring(namespace, "with-gpu-monitoring")),
	)
})

func testFunction(agent v2alpha1.DatadogAgent) func() {
	return func() {
		BeforeEach(func() {
			createKubernetesObject(k8sClient, &agent)
		})

		AfterEach(func() {
			deleteKubernetesObject(k8sClient, &agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(agent.Namespace, agent.Name)
		})
	}
}

func checkAgentDeployment(namespace string, name string) {
	checkAgentStatus(namespace, name)
	checkAgentDaemonSet(namespace, name)
	checkDCADeployment(namespace, name)
}

func checkAgentStatus(namespace string, ddaName string) {
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      ddaName,
	}

	agent := &v2alpha1.DatadogAgent{}
	getObjectAndCheck(agent, key, func() bool {
		return agent.Status.Agent != nil && agent.Status.ClusterAgent != nil
	})
}

func checkAgentDaemonSet(namespace string, ddaName string) {
	daemonSet := &appsv1.DaemonSet{}
	daemonSetKey := types.NamespacedName{
		Namespace: namespace,
		Name:      fmt.Sprintf("%s-%s", ddaName, "agent"),
	}

	getObjectAndCheck(daemonSet, daemonSetKey, func() bool {
		// We just verify that it exists
		return true
	})
}

func checkDCADeployment(namespace string, ddaName string) {
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{
		Namespace: namespace,
		Name:      fmt.Sprintf("%s-%s", ddaName, "cluster-agent"),
	}
	getObjectAndCheck(deployment, deploymentKey, func() bool {
		// We just verify that it exists
		return true
	})
}

func getObjectAndCheck(obj client.Object, key types.NamespacedName, check func() bool) {
	Eventually(func() bool {
		err := k8sClient.Get(context.Background(), key, obj)
		if err != nil {
			fmt.Fprint(GinkgoWriter, err)
			return false
		}

		return check()
	}, timeout, interval).Should(BeTrue())
}

// createKubernetesObject creates a kubernetes object and waits until it is accessible
func createKubernetesObject(k8sClient client.Client, obj client.Object) {
	Expect(k8sClient.Create(context.TODO(), obj)).Should(Succeed())

	Eventually(func() bool {
		key := types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}
		err := k8sClient.Get(context.Background(), key, obj)
		return err == nil
	}, timeout, interval).Should(BeTrue())
}

// deleteKubernetesObject deletes a kubernetes object and waits until it is no longer accessible
func deleteKubernetesObject(k8sClient client.Client, obj client.Object) {
	Expect(k8sClient.Delete(context.Background(), obj)).Should(Succeed())

	Eventually(func() bool {
		key := types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}
		err := k8sClient.Get(context.Background(), key, obj)
		return client.IgnoreNotFound(err) == nil
	}, timeout, interval).Should(BeTrue())
}
