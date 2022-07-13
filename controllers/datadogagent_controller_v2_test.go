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

	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	timeout  = time.Second * 30
	interval = time.Second * 2
)

var _ = Describe("DatadogAgent Controller - V2", func() {
	Context("Basic deployment", func() {
		namespace := "default"
		name := "foo"
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		It("should create a DaemonSet for the agent and a deployment for the DCA", func() {
			agent := newDatadogAgent(namespace, name)
			Expect(k8sClient.Create(context.Background(), agent)).Should(Succeed())

			agent = &v2alpha1.DatadogAgent{}
			getObjectAndCheck(agent, key, func() bool {
				return agent.Status.Agent != nil && agent.Status.ClusterAgent != nil
			})

			daemonSet := &appsv1.DaemonSet{}
			daemonSetKey := types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-%s", name, "agent"),
			}

			getObjectAndCheck(daemonSet, daemonSetKey, func() bool {
				// We just verify that it exists
				return true
			})

			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-%s", name, "cluster-agent"),
			}
			getObjectAndCheck(deployment, deploymentKey, func() bool {
				// We just verify that it exists
				return true
			})
		})
	})
})

func newDatadogAgent(namespace string, name string) *v2alpha1.DatadogAgent {
	credentialsSecret := v1.Secret{
		ObjectMeta: controllerruntime.ObjectMeta{
			Namespace: "default",
			Name:      "datadog-secret",
		},
		StringData: map[string]string{
			"api-key": "my-api-key",
			"app-key": "my-app-key",
		},
	}
	err := k8sClient.Create(context.TODO(), &credentialsSecret)
	Expect(err).ToNot(HaveOccurred())

	return &v2alpha1.DatadogAgent{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &common.SecretConfig{
						SecretName: "datadog-secret",
						KeyName:    "api-key",
					},
					AppSecret: &common.SecretConfig{
						SecretName: "datadog-secret",
						KeyName:    "app-key",
					},
				},
			},
		},
	}
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
