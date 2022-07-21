// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration_v2
// +build integration_v2

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	timeout  = 10 * time.Second
	interval = 100 * time.Millisecond
)

// These tests verify that a DatadogAgent deployment is successful.
//
// The function that checks if a deployment is successful is
// checkAgentDeployment(). At the moment, it checks these things:
// 	- The DatadogAgent status contains information about the agent and the DCA.
//  - The Agent DaemonSet has been deployed.
//  - The DCA Deployment has been deployed.
//
// These tests allow us to catch errors like the operator trying to create an
// invalid Kubernetes resource (RBAC, deployment without a name, etc.). However,
// these tests don't use a container runtime by default (they run with
// USE_EXISTING_CLUSTER=false). Therefore, these tests are not useful to catch
// errors that crash containers and keep them in "CrashLoopBackOff" state.
var _ = Describe("V2 Controller - DatadogAgent Deployment", func() {
	namespace := "default"
	agent := v2alpha1.DatadogAgent{}
	var name string

	Context("with no features enabled", func() {
		BeforeEach(func() {
			name = "basic"
			agent = testutils.NewDatadogAgentWithoutFeatures(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with cluster checks enabled", func() {
		BeforeEach(func() {
			name = "with-cluster-checks"
			agent = testutils.NewDatadogAgentWithClusterChecks(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with CSPM enabled", func() {
		BeforeEach(func() {
			name = "with-cspm"
			agent = testutils.NewDatadogAgentWithCSPM(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with CWS enabled", func() {
		BeforeEach(func() {
			name = "with-cws"
			agent = testutils.NewDatadogAgentWithCWS(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with Dogstatsd enabled", func() {
		BeforeEach(func() {
			name = "with-dogstatsd"
			agent = testutils.NewDatadogAgentWithDogstatsd(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with event collection", func() {
		BeforeEach(func() {
			name = "with-event-collection"
			agent = testutils.NewDatadogAgentWithEventCollection(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with KSM core", func() {
		BeforeEach(func() {
			name = "with-ksm"
			agent = testutils.NewDatadogAgentWithKSM(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with log collection", func() {
		BeforeEach(func() {
			name = "with-log-collection"
			agent = testutils.NewDatadogAgentWithLogCollection(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with NPM", func() {
		BeforeEach(func() {
			name = "with-npm"
			agent = testutils.NewDatadogAgentWithNPM(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with OOM Kill", func() {
		BeforeEach(func() {
			name = "with-oom-kill"
			agent = testutils.NewDatadogAgentWithOOMKill(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with orchestrator explorer", func() {
		BeforeEach(func() {
			name = "with-orchestrator-explorer"
			agent = testutils.NewDatadogAgentWithOrchestratorExplorer(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with Prometheus scrape", func() {
		BeforeEach(func() {
			name = "with-prometheus-scrape"
			agent = testutils.NewDatadogAgentWithPrometheusScrape(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with TCP queue length", func() {
		BeforeEach(func() {
			name = "with-tcp-queue-length"
			agent = testutils.NewDatadogAgentWithTCPQueueLength(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})

	Context("with USM", func() {
		BeforeEach(func() {
			name = "with-usm"
			agent = testutils.NewDatadogAgentWithUSM(namespace, name)
			createAgent(&agent)
		})

		AfterEach(func() {
			deleteAgent(&agent)
		})

		It("should deploy successfully", func() {
			checkAgentDeployment(namespace, name)
		})
	})
})

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

// createAgent creates an agent and waits until it is accessible
func createAgent(agent *v2alpha1.DatadogAgent) {
	Expect(k8sClient.Create(context.TODO(), agent)).Should(Succeed())

	Eventually(func() bool {
		err := k8sClient.Get(context.Background(), types.NamespacedName{
			Namespace: agent.Namespace,
			Name:      agent.Name,
		}, agent)
		return err == nil
	}, timeout, interval).Should(BeTrue())
}

// deleteAgent deletes an agent and waits until it is no longer accessible
func deleteAgent(agent *v2alpha1.DatadogAgent) {
	_ = k8sClient.Delete(context.TODO(), agent)

	Eventually(func() bool {
		err := k8sClient.Get(context.Background(), types.NamespacedName{
			Namespace: agent.Namespace,
			Name:      agent.Name,
		}, agent)
		return err != nil && errors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}
