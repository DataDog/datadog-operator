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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/untaint"
)

// Suite-level timeouts (see suite_v2_test.go):
//   ReadinessTimeout  = 4s
//   SchedulingTimeout = 4s
// `untaintTimeout` must comfortably exceed both so that Eventually has room.

const (
	untaintTimeout  = 20 * time.Second
	untaintInterval = 200 * time.Millisecond
)

// makeTaintedNode creates a Node carrying the agent-not-ready taint with the
// given name. The returned function deletes the Node — call it in AfterEach
// (or defer) to clean up.
func makeTaintedNode(ctx context.Context, name string) func() {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.NodeSpec{Taints: []corev1.Taint{untaint.AgentNotReadyTaint()}},
	}
	Expect(k8sClient.Create(ctx, node)).Should(Succeed())
	return func() { _ = k8sClient.Delete(ctx, node) }
}

func makeAgentPod(ctx context.Context, name, ns, nodeName string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
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
	return pod
}

var _ = Describe("Untaint Controller", func() {
	ctx := context.Background()

	Context("When agent pod becomes Ready on a tainted node", func() {
		// Each test creates a fresh node so it does not race with the
		// controller's timeout paths (configured to seconds in the suite).
		var (
			nodeName string
			cleanup  func()
			podName  = "agent-pod-becomes-ready"
			podNS    = "default"
		)

		BeforeEach(func() {
			nodeName = fmt.Sprintf("ready-path-node-%d", time.Now().UnixNano())
			cleanup = makeTaintedNode(ctx, nodeName)
		})

		AfterEach(func() {
			pod := &corev1.Pod{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: podNS, Name: podName}, pod); err == nil {
				_ = k8sClient.Delete(ctx, pod)
			}
			cleanup()
		})

		It("Should remove the taint when an agent pod transitions to Ready", func() {
			pod := makeAgentPod(ctx, podName, podNS, nodeName)

			// Set pod NotReady WITH a Status.StartTime in the recent past so
			// the controller stays on the readiness path and does not fire the
			// readiness timeout before we make the pod Ready below.
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: podNS, Name: podName}, pod)).Should(Succeed())
			start := metav1.Now()
			pod.Status.StartTime = &start
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			// Transition to Ready.
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: podNS, Name: podName}, pod)).Should(Succeed())
			pod.Status.Conditions = []corev1.PodCondition{{
				Type:               corev1.PodReady,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
			}}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			Eventually(func() bool {
				fresh := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, fresh); err != nil {
					return false
				}
				return !hasTaint(fresh)
			}, untaintTimeout, untaintInterval).Should(BeTrue(), "taint should be removed after agent becomes ready")
		})
	})

	Context("Startup catch-up (Ready pod created on cache sync)", func() {
		var (
			nodeName string
			cleanup  func()
			podName  = "agent-pod-catchup"
			podNS    = "default"
		)

		BeforeEach(func() {
			nodeName = fmt.Sprintf("catchup-node-%d", time.Now().UnixNano())
			cleanup = makeTaintedNode(ctx, nodeName)
		})

		AfterEach(func() {
			pod := &corev1.Pod{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: podNS, Name: podName}, pod); err == nil {
				_ = k8sClient.Delete(ctx, pod)
			}
			cleanup()
		})

		It("Should remove the taint when a Ready pod is created", func() {
			pod := makeAgentPod(ctx, podName, podNS, nodeName)

			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: podNS, Name: podName}, pod)).Should(Succeed())
			start := metav1.Now()
			pod.Status.StartTime = &start
			pod.Status.Conditions = []corev1.PodCondition{{
				Type:               corev1.PodReady,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
			}}
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

	Context("Scheduling timeout (no agent pod ever scheduled)", func() {
		// Configured scheduling timeout in the suite is 4s; expect untaint
		// within Eventually's window.
		It("Should untaint a node that never gets an agent pod", func() {
			nodeName := fmt.Sprintf("scheduling-timeout-node-%d", time.Now().UnixNano())
			cleanup := makeTaintedNode(ctx, nodeName)
			defer cleanup()

			Eventually(func() bool {
				fresh := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, fresh); err != nil {
					return false
				}
				return !hasTaint(fresh)
			}, untaintTimeout, untaintInterval).Should(BeTrue(), "scheduling timeout should untaint the node")
		})
	})

	Context("Readiness timeout (agent pod exists but never becomes Ready)", func() {
		It("Should untaint the node after the readiness timeout (policy=remove)", func() {
			nodeName := fmt.Sprintf("readiness-timeout-node-%d", time.Now().UnixNano())
			cleanup := makeTaintedNode(ctx, nodeName)
			defer cleanup()

			podName := "agent-pod-stuck-not-ready"
			podNS := "default"
			pod := makeAgentPod(ctx, podName, podNS, nodeName)
			defer func() { _ = k8sClient.Delete(ctx, pod) }()

			// Set Status.StartTime to a past moment so we are already over
			// the suite-configured readiness window (4s).
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: podNS, Name: podName}, pod)).Should(Succeed())
			past := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			pod.Status.StartTime = &past
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())

			Eventually(func() bool {
				fresh := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, fresh); err != nil {
					return false
				}
				return !hasTaint(fresh)
			}, untaintTimeout, untaintInterval).Should(BeTrue(), "readiness timeout should untaint the node")
		})
	})
})
