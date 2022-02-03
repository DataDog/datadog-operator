// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/testutils"
	// +kubebuilder:scaffold:imports
)

const (
	timeout  = time.Second * 30
	interval = time.Second * 2

	confdConfigMapName   = "confd-config"
	checksdConfigMapName = "checksd-config"
)

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

func checkAgentUpdateOnObject(agentKey, objKey types.NamespacedName, obj client.Object,
	getAgentHash func(agent *datadoghqv1alpha1.DatadogAgent) string,
	getAnnotationHash func() string,
	updateAgent func(agent *datadoghqv1alpha1.DatadogAgent),
	check func(agent *datadoghqv1alpha1.DatadogAgent) bool) {
	var beforeHash string
	var agent *datadoghqv1alpha1.DatadogAgent

	Eventually(func() bool {
		// Getting Agent object to fetch hash before update
		agent = &datadoghqv1alpha1.DatadogAgent{}
		err := k8sClient.Get(context.Background(), agentKey, agent)
		if err != nil {
			fmt.Fprint(GinkgoWriter, err)
			return false
		}
		beforeHash = getAgentHash(agent)

		// Update agent
		updateAgent(agent)
		err = k8sClient.Update(context.Background(), agent)
		if err != nil {
			fmt.Fprint(GinkgoWriter, err)
			return false
		}

		return true
	}, 5*time.Second, time.Second).Should(BeTrue())

	getObjectAndCheck(obj, objKey, func() bool {
		currentHash := getAnnotationHash()
		if currentHash == beforeHash || currentHash == "" {
			return false
		}
		if check != nil {
			return check(agent)
		}
		return true
	})
}

func checkAgentUpdateOnDaemonSet(agentKey, dsKey types.NamespacedName, updateAgent func(agent *datadoghqv1alpha1.DatadogAgent), check func(agent *datadoghqv1alpha1.DatadogAgent) bool) {
	obj := &appsv1.DaemonSet{}
	checkAgentUpdateOnObject(agentKey, dsKey, obj, func(agent *datadoghqv1alpha1.DatadogAgent) string {
		if agent.Status.Agent != nil {
			return agent.Status.Agent.CurrentHash
		}
		return ""
	}, func() string {
		return obj.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
	}, updateAgent, check)
}

func checkAgentUpdateOnClusterAgent(agentKey, dsKey types.NamespacedName, updateAgent func(agent *datadoghqv1alpha1.DatadogAgent), check func(agent *datadoghqv1alpha1.DatadogAgent) bool) {
	obj := &appsv1.Deployment{}
	checkAgentUpdateOnObject(agentKey, dsKey, obj, func(agent *datadoghqv1alpha1.DatadogAgent) string {
		if agent.Status.ClusterAgent != nil {
			return agent.Status.ClusterAgent.CurrentHash
		}
		return ""
	}, func() string {
		return obj.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
	}, updateAgent, check)
}

// This test may take ~30s to run, check you go test timeout
var _ = Describe("DatadogAgent Controller", func() {
	Context("Initial deployment", func() {
		namespace := "default"
		name := "foo"
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}
		dsName := fmt.Sprintf("%s-%s", name, "agent")
		dsKey := types.NamespacedName{
			Namespace: namespace,
			Name:      dsName,
		}

		It("It should create DaemonSet", func() {
			options := &testutils.NewDatadogAgentOptions{
				UseEDS:                       false,
				ClusterAgentDisabled:         true,
				APIKey:                       "xnfdsjgdjcxlg42rqmzxzvdsgjdfklg",
				AppKey:                       "xnfdsjgdjcxlg42rqmzxzvdsgjdfklg-dsddsd",
				OrchestratorExplorerDisabled: true,
			}

			agent := testutils.NewDatadogAgent(namespace, name, "", options)
			Expect(k8sClient.Create(context.Background(), agent)).Should(Succeed())

			agent = &datadoghqv1alpha1.DatadogAgent{}
			getObjectAndCheck(agent, key, func() bool {
				if agent.Status.Agent == nil || agent.Status.Agent.CurrentHash == "" {
					return false
				}
				for _, condition := range agent.Status.Conditions {
					if condition.Type == datadoghqv1alpha1.DatadogAgentConditionTypeActive && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
				return false
			})

			ds := &appsv1.DaemonSet{}
			getObjectAndCheck(ds, dsKey, func() bool {
				// We just verify we are able to find a DS with ns/name
				return true
			})
		})

		It("Should update DaemonSet", func() {
			agent := &datadoghqv1alpha1.DatadogAgent{}
			Expect(k8sClient.Get(context.Background(), key, agent)).ToNot(HaveOccurred())

			By("Updating on image change", func() {
				checkAgentUpdateOnDaemonSet(key, dsKey, func(agent *datadoghqv1alpha1.DatadogAgent) {
					agent.Spec.Agent.Image.Name = "datadog/agent:7.22.0"
				}, nil)
			})

			By("Activating APM", func() {
				checkAgentUpdateOnDaemonSet(key, dsKey, func(agent *datadoghqv1alpha1.DatadogAgent) {
					agent.Spec.Agent.Apm.Enabled = apiutils.NewBoolPointer(true)
				}, nil)
			})

			By("Disabling OrchestratorExplorer", func() {
				checkAgentUpdateOnDaemonSet(key, dsKey, func(agent *datadoghqv1alpha1.DatadogAgent) {
					agent.Spec.Features.OrchestratorExplorer = &datadoghqv1alpha1.OrchestratorExplorerConfig{
						Enabled: apiutils.NewBoolPointer(false),
					}
				}, nil)
			})

			By("Activating System Probe", func() {
				checkAgentUpdateOnDaemonSet(key, dsKey, func(agent *datadoghqv1alpha1.DatadogAgent) {
					agent.Spec.Agent.SystemProbe.Enabled = apiutils.NewBoolPointer(true)
				}, nil)
			})

			By("Update the DatadogAgent with custom conf.d and checks.d", func() {
				checkAgentUpdateOnDaemonSet(key, dsKey, func(agent *datadoghqv1alpha1.DatadogAgent) {
					agent.Spec.Agent.Config.Confd = &datadoghqv1alpha1.ConfigDirSpec{
						ConfigMapName: confdConfigMapName,
					}
					agent.Spec.Agent.Config.Checksd = &datadoghqv1alpha1.ConfigDirSpec{
						ConfigMapName: checksdConfigMapName,
					}
				}, nil)
			})

			By("Enabled Process", func() {
				checkAgentUpdateOnDaemonSet(key, dsKey, func(agent *datadoghqv1alpha1.DatadogAgent) {
					agent.Spec.Agent.Process.Enabled = apiutils.NewBoolPointer(true)
				}, nil)
			})
		})
	})

	Context("Cluster Agent Deployment", func() {
		namespace := "default"
		name := "foo-dca"
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}
		dcaName := fmt.Sprintf("%s-%s", name, "cluster-agent")
		dcaKey := types.NamespacedName{
			Namespace: namespace,
			Name:      dcaName,
		}

		It("It should create Deployment", func() {
			options := &testutils.NewDatadogAgentOptions{APIKey: "xnfdsjgdjcxlg42rqmzxzvdsgjdfklg", AppKey: "xnfdsjgdjcxlg42rqmzxzvdsgjdfklg-23678264"}
			agent := testutils.NewDatadogAgent(namespace, name, "datadog/agent:7.22.0", options)
			Expect(k8sClient.Create(context.Background(), agent)).Should(Succeed())

			var agentClusterAgentHash string
			agent = &datadoghqv1alpha1.DatadogAgent{}
			getObjectAndCheck(agent, key, func() bool {
				if agent.Status.ClusterAgent == nil {
					return false
				}
				if agent.Status.ClusterAgent.CurrentHash == "" {
					return false
				}

				agentClusterAgentHash = agent.Status.ClusterAgent.CurrentHash
				return true
			})

			clusterAgent := &appsv1.Deployment{}
			getObjectAndCheck(clusterAgent, dcaKey, func() bool {
				return clusterAgent.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey] == agentClusterAgentHash
			})
		})

		It("Should update ClusterAgent", func() {
			checkAgentUpdateOnClusterAgent(key, dcaKey, func(agent *datadoghqv1alpha1.DatadogAgent) {
				agent.Spec.ClusterAgent.Image.Name = "datadog/cluster-agent:1.0.0"
				agent.Spec.ClusterAgent.Config.ClusterChecksEnabled = apiutils.NewBoolPointer(true)
				agent.Spec.ClusterChecksRunner = datadoghqv1alpha1.DatadogAgentSpecClusterChecksRunnerSpec{
					Image: &commonv1.AgentImageConfig{
						Name: "datadog/agent:7.22.0",
					},
				}
			}, nil)
		})
	})
})
