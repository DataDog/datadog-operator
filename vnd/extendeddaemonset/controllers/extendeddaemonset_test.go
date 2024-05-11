// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

//go:build !e2e
// +build !e2e

package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/conditions"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/strategy"
	"github.com/DataDog/extendeddaemonset/controllers/testutils"
	// +kubebuilder:scaffold:imports
)

// This test may take ~30s to run, check you go test timeout.
var _ = Describe("ExtendedDaemonSet Controller", func() {
	const timeout = time.Second * 30
	const interval = time.Second * 2

	intString10 := intstr.FromInt(10)
	reconcileFrequency := &metav1.Duration{Duration: time.Millisecond * 100}
	namespace := testConfig.namespace
	ctx := context.Background()

	Context("Using ExtendedDaemonsetSetting", func() {
		name := "eds-setting"
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		It("Should create one pod by node", func() {
			edsOptions := &testutils.NewExtendedDaemonsetOptions{
				CanaryStrategy: nil,
				RollingUpdate: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
					MaxPodSchedulerFailure: &intString10,
					MaxUnavailable:         &intString10,
					MaxParallelPodCreation: datadoghqv1alpha1.NewInt32(20),
					SlowStartIntervalDuration: &metav1.Duration{
						Duration: 1 * time.Millisecond,
					},
					SlowStartAdditiveIncrease: &intString10,
				},
				ReconcileFrequency: reconcileFrequency,
			}
			eds := testutils.NewExtendedDaemonset(namespace, name, "registry.k8s.io/pause:latest", edsOptions)
			Expect(k8sClient.Create(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.ActiveReplicaSet != ""
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ActiveReplicaSet should be set EDS: %#v",
					eds.Status,
				)
			},
			)

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			erskey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.ActiveReplicaSet,
			}
			Eventually(withERS(erskey, ers, func() bool {
				return ers.Status.Desired == ers.Status.Current
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ers.Status.Desired should be equal to ers.Status.Current, status: %#v",
					ers.Status,
				)
			},
			)
		})
	})
})

var _ = Describe("ExtendedDaemonSet Rolling Update Pause", func() {
	const timeout = time.Second * 30
	const interval = time.Second * 2

	intString10 := intstr.FromInt(10)
	reconcileFrequency := &metav1.Duration{Duration: time.Millisecond * 100}
	namespace := testConfig.namespace
	ctx := context.Background()

	Context("Initial deployment", func() {
		var firstERSName, secondERSName string
		name := "eds-pause"
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		It("Should deploy", func() {
			edsOptions := &testutils.NewExtendedDaemonsetOptions{
				CanaryStrategy: nil,
				RollingUpdate: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
					MaxPodSchedulerFailure: &intString10,
					MaxUnavailable:         &intString10,
					MaxParallelPodCreation: datadoghqv1alpha1.NewInt32(20),
					SlowStartIntervalDuration: &metav1.Duration{
						Duration: 1 * time.Millisecond,
					},
					SlowStartAdditiveIncrease: &intString10,
				},
				ReconcileFrequency: reconcileFrequency,
			}
			eds := testutils.NewExtendedDaemonset(namespace, name, "registry.k8s.io/pause:latest", edsOptions)
			Expect(k8sClient.Create(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.ActiveReplicaSet != ""
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ActiveReplicaSet should be set: %#v",
					eds.Status,
				)
			},
			)

			firstERSName = eds.Status.ActiveReplicaSet

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			erskey := types.NamespacedName{
				Namespace: namespace,
				Name:      firstERSName,
			}
			Eventually(withERS(erskey, ers, func() bool {
				return ers.Status.Desired == ers.Status.Current
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ers.Status.Desired should be equal to ers.Status.Current, status: %#v",
					ers.Status,
				)
			},
			)
		})

		It("Should not deploy with paused EDS", func() {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			eds.Spec.Template.Spec.Containers[0].Image = "registry.k8s.io/pause:3.0" // Trigger a new ERS
			if eds.Annotations == nil {
				eds.Annotations = make(map[string]string)
			}
			eds.Annotations["extendeddaemonset.datadoghq.com/rolling-update-paused"] = "true" // Pause the rolling update
			Expect(k8sClient.Update(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.ActiveReplicaSet != "" && eds.Status.ActiveReplicaSet != firstERSName
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ActiveReplicaSet should be updated: %#v, old ers: %s",
					eds.Status,
					firstERSName,
				)
			},
			)

			secondERSName = eds.Status.ActiveReplicaSet

			Expect(func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRollingUpdatePaused
			}()).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"EDS status should be updated: %#v",
					eds.Status,
				)
			})

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			erskey := types.NamespacedName{
				Namespace: namespace,
				Name:      secondERSName,
			}
			Eventually(withERS(erskey, ers, func() bool {
				return ers.Status.Desired != ers.Status.Current
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ers.Status.Desired should not be equal to ers.Status.Current, status: %#v",
					ers.Status,
				)
			},
			)

			Expect(func() bool {
				expectedStatus := ers.Status.Status == string(strategy.ReplicaSetStatusActive)
				expectedCondition := conditions.IsConditionTrue(&ers.Status, datadoghqv1alpha1.ConditionTypeRollingUpdatePaused)

				return expectedStatus && expectedCondition
			}()).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ERS status should be updated: %#v",
					ers.Status,
				)
			})
		})

		It("Should not let nodes without pods even when paused", func() {
			podList := &corev1.PodList{}
			listOptions := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					"extendeddaemonset.datadoghq.com/name": name,
				},
			}
			Eventually(withList(listOptions, podList, "pods", func() bool {
				return len(podList.Items) == fakeNodesCount
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"Should get all the pods, got: %d",
					len(podList.Items),
				)
			},
			)

			extraNode := testutils.NewNode(fmt.Sprintf("node%d", fakeNodesCount+1), nil)
			Expect(k8sClient.Create(context.Background(), extraNode)).Should(Succeed())

			podList = &corev1.PodList{}
			Eventually(withList(listOptions, podList, "pods", func() bool {
				return len(podList.Items) == fakeNodesCount+1
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"Should get an extra pod, got: %d",
					len(podList.Items),
				)
			},
			)

			Expect(k8sClient.Delete(context.Background(), extraNode)).Should(Succeed())
		})

		It("Should continue the rolling update when unpaused", func() {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			eds.Annotations["extendeddaemonset.datadoghq.com/rolling-update-paused"] = "false" // Unpause the rolling update
			Expect(k8sClient.Update(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRunning
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"eds.Status should be updated: %#v",
					eds.Status,
				)
			},
			)

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			erskey := types.NamespacedName{
				Namespace: namespace,
				Name:      secondERSName,
			}

			Eventually(withERS(erskey, ers, func() bool {
				expectedStatus := ers.Status.Status == string(strategy.ReplicaSetStatusActive)
				expectedCondition := conditions.IsConditionTrue(&ers.Status, datadoghqv1alpha1.ConditionTypeActive)
				expectedDesired := ers.Status.Desired == fakeNodesCount

				return expectedStatus && expectedCondition && expectedDesired
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ERS status should be updated: %#v",
					ers.Status,
				)
			},
			)
		})
	})
})

var _ = Describe("ExtendedDaemonSet Rollout Freeze", func() {
	const timeout = time.Second * 30
	const interval = time.Second * 2

	intString10 := intstr.FromInt(10)
	reconcileFrequency := &metav1.Duration{Duration: time.Millisecond * 100}
	namespace := testConfig.namespace
	ctx := context.Background()

	Context("Initial deployment", func() {
		name := "eds-freeze"
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		It("Should not deploy with frozen EDS", func() {
			edsOptions := &testutils.NewExtendedDaemonsetOptions{
				CanaryStrategy: nil,
				RollingUpdate: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
					MaxPodSchedulerFailure: &intString10,
					MaxUnavailable:         &intString10,
					MaxParallelPodCreation: datadoghqv1alpha1.NewInt32(20),
					SlowStartIntervalDuration: &metav1.Duration{
						Duration: 1 * time.Millisecond,
					},
					SlowStartAdditiveIncrease: &intString10,
				},
				ReconcileFrequency: reconcileFrequency,
				ExtraAnnotations:   map[string]string{"extendeddaemonset.datadoghq.com/rollout-frozen": "true"},
			}
			eds := testutils.NewExtendedDaemonset(namespace, name, "registry.k8s.io/pause:latest", edsOptions)
			Expect(k8sClient.Create(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.ActiveReplicaSet != ""
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ActiveReplicaSet should be set: %#v",
					eds.Status,
				)
			},
			)

			Expect(func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRolloutFrozen
			}()).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"EDS status should be updated: %#v",
					eds.Status,
				)
			})

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			erskey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.ActiveReplicaSet,
			}
			Eventually(withERS(erskey, ers, func() bool {
				expectedStatus := ers.Status.Status == string(strategy.ReplicaSetStatusActive)
				expectedCondition := conditions.IsConditionTrue(&ers.Status, datadoghqv1alpha1.ConditionTypeRolloutFrozen)
				expectedCurrent := (ers.Status.Current == 0) && (ers.Status.Desired != ers.Status.Current)

				return expectedStatus && expectedCondition && expectedCurrent
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ERS status should be updated: %#v",
					ers.Status,
				)
			},
			)
		})

		It("Should continue the rollout when unfrozen", func() {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			eds.Annotations["extendeddaemonset.datadoghq.com/rollout-frozen"] = "false" // Unfreeze the rollout
			Expect(k8sClient.Update(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRunning
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"EDS should be updated: %#v",
					eds.Status,
				)
			},
			)

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			erskey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.ActiveReplicaSet,
			}

			Eventually(withERS(erskey, ers, func() bool {
				expectedStatus := ers.Status.Status == string(strategy.ReplicaSetStatusActive)
				expectedCondition := conditions.IsConditionTrue(&ers.Status, datadoghqv1alpha1.ConditionTypeActive)
				expectedDesired := ers.Status.Desired == ers.Status.Current

				return expectedStatus && expectedCondition && expectedDesired
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ERS status should be updated: %#v",
					eds.Status,
				)
			},
			)
		})

		It("Should not deploy to new nodes when frozen", func() {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			eds.Annotations["extendeddaemonset.datadoghq.com/rollout-frozen"] = "true" // Freeze the rollout again
			Expect(k8sClient.Update(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRolloutFrozen
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"EDS should be updated: %#v",
					eds.Status,
				)
			},
			)

			extraNode := testutils.NewNode(fmt.Sprintf("node%d", fakeNodesCount+1), nil)
			Expect(k8sClient.Create(context.Background(), extraNode)).Should(Succeed())

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			erskey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.ActiveReplicaSet,
			}

			Eventually(withERS(erskey, ers, func() bool {
				expectedStatus := ers.Status.Status == string(strategy.ReplicaSetStatusActive)
				expectedCondition := conditions.IsConditionTrue(&ers.Status, datadoghqv1alpha1.ConditionTypeRolloutFrozen)

				return expectedStatus && expectedCondition
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ERS status should be updated: %#v",
					ers.Status,
				)
			},
			)

			podList := &corev1.PodList{}
			listOptions := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					"extendeddaemonset.datadoghq.com/name": name,
				},
			}
			Eventually(withList(listOptions, podList, "pods", func() bool {
				return len(podList.Items) == fakeNodesCount
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"Should not get all the pods, got: %d",
					len(podList.Items),
				)
			},
			)

			Expect(k8sClient.Delete(context.Background(), extraNode)).Should(Succeed())
		})
	})
})
