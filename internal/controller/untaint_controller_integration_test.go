// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	untaintTimeout  = 15 * time.Second
	untaintInterval = 200 * time.Millisecond
)

var _ = Describe("Untaint Controller", func() {
	ctx := context.Background()
	nodeName := "tainted-node-1"

	Context("When agent pod becomes Ready on a tainted node", func() {
		agentPodName := "agent-pod-integration"
		agentPodNamespace := "default"

		AfterEach(func() {
			// Clean up agent pod
			pod := &corev1.Pod{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: agentPodNamespace, Name: agentPodName}, pod); err == nil {
				_ = k8sClient.Delete(ctx, pod)
			}
			// Restore taint on node for next test
			node := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)).Should(Succeed())
			if !hasTaint(node) {
				node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
					Key:    agentNotReadyTaintKey,
					Value:  agentNotReadyTaintValue,
					Effect: agentNotReadyTaintEffect,
				})
				Expect(k8sClient.Update(ctx, node)).Should(Succeed())
			}
		})

		It("Should remove the taint when an agent pod transitions to Ready", func() {
			// 1. Create agent pod (not ready)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentPodName,
					Namespace: agentPodNamespace,
					Labels: map[string]string{
						common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
					},
				},
				Spec: corev1.PodSpec{
					NodeName:   nodeName,
					Containers: []corev1.Container{{Name: "agent", Image: "fake"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())

			// Set pod status to not-ready
			pod.Status.Conditions = []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			// 2. Verify taint is still present
			node := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)).Should(Succeed())
			Expect(hasTaint(node)).To(BeTrue(), "taint should still be present before agent is ready")

			// 3. Update pod to Ready
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: agentPodNamespace, Name: agentPodName}, pod)).Should(Succeed())
			pod.Status.Conditions = []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			// 4. Eventually: taint should be removed
			Eventually(func() bool {
				fresh := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, fresh); err != nil {
					return false
				}
				return !hasTaint(fresh)
			}, untaintTimeout, untaintInterval).Should(BeTrue(), "taint should be removed after agent becomes ready")
		})

		It("Should not remove the taint while the agent pod is not Ready", func() {
			// Create agent pod, never make it ready
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentPodName,
					Namespace: agentPodNamespace,
					Labels: map[string]string{
						common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
					},
				},
				Spec: corev1.PodSpec{
					NodeName:   nodeName,
					Containers: []corev1.Container{{Name: "agent", Image: "fake"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
			pod.Status.Conditions = []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			// Wait a moment; taint must remain
			Consistently(func() bool {
				fresh := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, fresh); err != nil {
					return false
				}
				return hasTaint(fresh)
			}, 3*time.Second, untaintInterval).Should(BeTrue(), "taint should remain while agent is not ready")
		})
	})

	Context("When agent pod is already Ready at startup (startup catch-up)", func() {
		agentPodName := "agent-pod-catchup"
		agentPodNamespace := "default"

		AfterEach(func() {
			pod := &corev1.Pod{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: agentPodNamespace, Name: agentPodName}, pod); err == nil {
				_ = k8sClient.Delete(ctx, pod)
			}
			// Restore taint
			node := &corev1.Node{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)).Should(Succeed())
			if !hasTaint(node) {
				node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
					Key:    agentNotReadyTaintKey,
					Value:  agentNotReadyTaintValue,
					Effect: agentNotReadyTaintEffect,
				})
				Expect(k8sClient.Update(ctx, node)).Should(Succeed())
			}
		})

		It("Should remove the taint when a Ready pod is created (cache sync)", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentPodName,
					Namespace: agentPodNamespace,
					Labels: map[string]string{
						common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
					},
				},
				Spec: corev1.PodSpec{
					NodeName:   nodeName,
					Containers: []corev1.Container{{Name: "agent", Image: "fake"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())

			// Immediately set to Ready (simulates pod being ready on create)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: agentPodNamespace, Name: agentPodName}, pod)).Should(Succeed())
			pod.Status.Conditions = []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			Eventually(func() bool {
				fresh := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, fresh); err != nil {
					return false
				}
				return !hasTaint(fresh)
			}, untaintTimeout, untaintInterval).Should(BeTrue())
		})
	})
})
