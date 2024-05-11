// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

//go:build e2e
// +build e2e

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	edsconditions "github.com/DataDog/extendeddaemonset/controllers/extendeddaemonset/conditions"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/conditions"
	"github.com/DataDog/extendeddaemonset/controllers/testutils"
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils/pod"
	// +kubebuilder:scaffold:imports
)

const (
	timeout     = 1 * time.Minute
	longTimeout = 5 * time.Minute
	interval    = 2 * time.Second

	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Purple = "\033[35m"
	Bold   = "\x1b[1m"
)

var (
	intString1  = intstr.FromInt(1)
	intString2  = intstr.FromInt(2)
	intString10 = intstr.FromInt(10)
	namespace   = testConfig.namespace
	ctx         = context.Background()
)

func logPreamble() string {
	return Bold + "E2E >> " + Reset
}

func info(format string, a ...interface{}) {
	ginkgoLog(logPreamble()+Purple+Bold+format+Reset, a...)
}

func warn(format string, a ...interface{}) {
	ginkgoLog(logPreamble()+Red+Bold+format+Reset, a...)
}

func ginkgoLog(format string, a ...interface{}) {
	fmt.Fprintf(GinkgoWriter, format, a...)
}

// These tests may take several minutes to run, check your go test timeout
var _ = Describe("ExtendedDaemonSet e2e updates and recovery", func() {
	Context("Initial deployment", func() {
		name := "eds-fail"

		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		nodeList := &corev1.NodeList{}

		It("Should deploy EDS", func() {
			Expect(k8sClient.List(ctx, nodeList)).Should(Succeed())

			edsOptions := &testutils.NewExtendedDaemonsetOptions{
				CanaryStrategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					Duration: &metav1.Duration{Duration: 1 * time.Minute},
					Replicas: &intString2,
				},
				RollingUpdate: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
					MaxUnavailable:         &intString10,
					MaxParallelPodCreation: datadoghqv1alpha1.NewInt32(20),
				},
			}

			eds := testutils.NewExtendedDaemonset(namespace, name, "registry.k8s.io/pause:3.0", edsOptions)
			Expect(k8sClient.Create(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.ActiveReplicaSet != ""
			}), timeout, interval).Should(BeTrue())

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			ersKey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.ActiveReplicaSet,
			}
			Eventually(withERS(ersKey, ers, func() bool {
				return ers.Status.Status == "active" && int(ers.Status.Available) == len(nodeList.Items)
			}), timeout, interval).Should(BeTrue())
		})

		It("Should auto-pause and auto-fail canary on restarts", func() {
			updateFunc := func(eds *datadoghqv1alpha1.ExtendedDaemonSet) {
				eds.Spec.Strategy.Canary = &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					Duration:           &metav1.Duration{Duration: 1 * time.Minute},
					Replicas:           &intString1,
					NoRestartsDuration: &metav1.Duration{Duration: 1 * time.Minute},
					AutoPause: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
						Enabled:     datadoghqv1alpha1.NewBool(true),
						MaxRestarts: datadoghqv1alpha1.NewInt32(1),
					},
					AutoFail: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{
						Enabled:     datadoghqv1alpha1.NewBool(true),
						MaxRestarts: datadoghqv1alpha1.NewInt32(3),
					},
				}
				eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("gcr.io/google-containers/alpine-with-bash:1.0")
				eds.Spec.Template.Spec.Containers[0].Command = []string{
					"does-not-exist", // command that does not exist
				}
			}

			Eventually(updateEDS(k8sClient, key, updateFunc), timeout, interval).Should(
				BeTrue(),
				func() string { return "Unable to update the EDS" },
			)

			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanaryPaused
			}), longTimeout, interval).Should(
				BeTrue(),
				func() string {
					return fmt.Sprintf(
						"EDS should be in [%s] state but is currently in [%s]",
						datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanaryPaused,
						eds.Status.State,
					)
				},
			)
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.Reason == "StartError"
			}), timeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"EDS should be in [%s] state reason but is currently in [%s]",
					"StartError",
					eds.Status.Reason,
				)
			})

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRunning
			}), longTimeout, interval).Should(
				BeTrue(),
				func() string {
					return fmt.Sprintf(
						"EDS should be in [%s] state but is currently in [%s]",
						datadoghqv1alpha1.ExtendedDaemonSetStatusStateRunning,
						eds.Status.State,
					)
				},
			)

			Eventually(withEDS(key, eds, func() bool {
				if edsconditions.GetExtendedDaemonSetStatusCondition(&eds.Status, datadoghqv1alpha1.ConditionTypeEDSCanaryFailed) != nil {
					return true
				}
				return false
			}), longTimeout, interval).ShouldNot(
				BeNil(),
				func() string {
					return fmt.Sprintf(
						"EDS canary failure should be present in the EDS.Status.Conditions: %v",
						eds.Status.Conditions,
					)
				},
			)
		})

		It("Should recover from failed on update", func() {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			info("EDS status:\n%s\n", spew.Sdump(eds.Status))

			_ = clearCanaryAnnotations(eds)

			eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("gcr.io/google-containers/alpine-with-bash:1.0")
			eds.Spec.Template.Spec.Containers[0].Command = []string{
				"tail", "-f", "/dev/null",
			}
			Eventually(withUpdate(eds, "EDS"),
				timeout, interval).Should(BeTrue())

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanary
			}), timeout, interval).Should(BeTrue())

			canaryReplicaSet := eds.Status.Canary.ReplicaSet
			if eds.Annotations == nil {
				eds.Annotations = make(map[string]string)
			}
			eds.Annotations[datadoghqv1alpha1.ExtendedDaemonSetCanaryValidAnnotationKey] = canaryReplicaSet
			Eventually(withUpdate(eds, "EDS"),
				timeout, interval).Should(BeTrue())

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRunning
			}), timeout, interval).Should(BeTrue())
		})

		It("Should delete EDS", func() {
			Eventually(deleteEDS(k8sClient, key), timeout, interval).Should(BeTrue(), "EDS should be deleted")

			pods := &corev1.PodList{}
			listOptions := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: name,
				},
			}
			Eventually(withList(listOptions, pods, "EDS pods", func() bool {
				return len(pods.Items) == 0
			}), longTimeout, interval).Should(BeTrue(), "All EDS pods should be destroyed")
		})

	})
})

var _ = Describe("ExtendedDaemonSet e2e PodCannotStart condition", func() {
	var (
		name string
		key  types.NamespacedName
	)

	BeforeEach(func() {
		name = fmt.Sprintf("eds-foo-%d", time.Now().Unix())
		key = types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		info("BeforeEach: Creating EDS %s\n", name)

		edsOptions := &testutils.NewExtendedDaemonsetOptions{
			CanaryStrategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
				Duration: &metav1.Duration{Duration: 1 * time.Minute},
				Replicas: &intString2,
				AutoPause: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
					Enabled:              datadoghqv1alpha1.NewBool(true),
					MaxSlowStartDuration: &metav1.Duration{Duration: 10 * time.Second},
				},
			},
			RollingUpdate: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
				MaxUnavailable:         &intString10,
				MaxParallelPodCreation: datadoghqv1alpha1.NewInt32(20),
			},
		}

		eds := testutils.NewExtendedDaemonset(namespace, name, "registry.k8s.io/pause:3.0", edsOptions)
		Expect(k8sClient.Create(ctx, eds)).Should(Succeed())

		eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
		Eventually(withEDS(key, eds, func() bool {
			return eds.Status.ActiveReplicaSet != ""
		}), timeout, interval).Should(BeTrue())

		info("BeforeEach: Done creating EDS %s - active replicaset: %s\n", name, eds.Status.ActiveReplicaSet)
	})

	AfterEach(func() {
		info("AfterEach: Destroying EDS %s\n", name)
		eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
		Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
		info("AfterEach: Destroying EDS %s - canary replicaset: %s\n", name, eds.Status.Canary.ReplicaSet)
		info("AfterEach: Destroying EDS %s - active replicaset: %s\n", name, eds.Status.ActiveReplicaSet)

		Eventually(deleteEDS(k8sClient, key), timeout, interval).Should(BeTrue(), "EDS should be deleted")

		pods := &corev1.PodList{}
		listOptions := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: name,
			},
		}
		Eventually(withList(listOptions, pods, "EDS pods", func() bool {
			return len(pods.Items) == 0
		}), longTimeout, interval).Should(BeTrue(), "All EDS pods should be destroyed")

		erslist := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList{}
		Eventually(withList(listOptions, erslist, "ERS instances", func() bool {
			return len(erslist.Items) == 0
		}), timeout, interval).Should(BeTrue(), "All ERS instances should be destroyed")

		info("AfterEach: Done destroying EDS %s\n", name)
	})

	JustAfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			warn("%s - FAILED: EDS status:\n%s\n\n", CurrentGinkgoTestDescription().TestText, spew.Sdump(eds.Status))
		}
	})

	pauseOnCannotStart := func(configureEDS func(eds *datadoghqv1alpha1.ExtendedDaemonSet), expectedReasons ...string) {
		Eventually(updateEDS(k8sClient, key, configureEDS), timeout, interval).Should(
			BeTrue(),
			func() string { return "Unable to update the EDS" },
		)

		eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
		Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
		info("%s: %s - active replicaset: %s\n",
			CurrentGinkgoTestDescription().TestText,
			name, eds.Status.ActiveReplicaSet,
		)

		info("EDS %s - waiting for canary to be paused\n", name)
		eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
		Eventually(withEDS(key, eds, func() bool {
			return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanaryPaused
		}), timeout, interval).Should(BeTrue())

		info("EDS status:\n%s\n", spew.Sdump(eds.Status))

		cond := edsconditions.GetExtendedDaemonSetStatusCondition(&eds.Status, datadoghqv1alpha1.ConditionTypeEDSCanaryPaused)
		Expect(cond).ShouldNot(BeNil())
		Expect(cond.Status).Should(Equal(corev1.ConditionTrue))
		pausedReason := cond.Reason
		var matchers []gomegatypes.GomegaMatcher
		for _, reason := range expectedReasons {
			matchers = append(matchers, Equal(reason))
		}

		Expect(pausedReason).Should(Or(matchers...), "EDS canary should be paused with expected reason")

		ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
		ersKey := types.NamespacedName{
			Namespace: namespace,
			Name:      eds.Status.Canary.ReplicaSet,
		}

		var cannotStartCondition *datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCondition
		Eventually(withERS(ersKey, ers, func() bool {
			cannotStartCondition = conditions.GetExtendedDaemonSetReplicaSetStatusCondition(&ers.Status, datadoghqv1alpha1.ConditionTypePodCannotStart)
			return cannotStartCondition != nil
		}), timeout, interval).Should(BeTrue())

		Expect(cannotStartCondition.Status).Should(Equal(corev1.ConditionTrue))

		matchers = []gomegatypes.GomegaMatcher{}
		for _, reason := range expectedReasons {
			matchers = append(matchers, MatchRegexp(fmt.Sprintf("Pod eds-foo-.*? cannot start with reason: %s", reason)))
		}
		Expect(cannotStartCondition.Message).Should(Or(matchers...))
		info("%s: %s - done\n",
			CurrentGinkgoTestDescription().TestText,
			name,
		)
	}

	Context("When pod has image pull error", func() {
		It("Should promptly auto-pause canary", func() {
			pauseOnCannotStart(func(eds *datadoghqv1alpha1.ExtendedDaemonSet) {
				eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("gcr.io/missing")
				eds.Spec.Template.Spec.Containers[0].Command = []string{
					"does-not-matter",
				}
			}, "ErrImagePull", "ImagePullBackOff")
		})
	})

	Context("When pod has container config error", func() {
		It("Should promptly auto-pause canary", func() {
			pauseOnCannotStart(func(eds *datadoghqv1alpha1.ExtendedDaemonSet) {
				eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("gcr.io/google-containers/alpine-with-bash:1.0")
				eds.Spec.Template.Spec.Containers[0].Command = []string{
					"tail", "-f", "/dev/null",
				}
				eds.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
					{
						Name: "missing",
						ValueFrom: &corev1.EnvVarSource{
							ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "missing",
								},
								Key: "missing",
							},
						},
					},
				}
			}, "CreateContainerConfigError")
		})
	})

	Context("When pod has missing volume", func() {
		It("Should promptly auto-pause canary", func() {
			pauseOnCannotStart(func(eds *datadoghqv1alpha1.ExtendedDaemonSet) {
				eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("gcr.io/google-containers/alpine-with-bash:1.0")
				eds.Spec.Template.Spec.Containers[0].Command = []string{
					"tail", "-f", "/dev/null",
				}
				eds.Spec.Template.Spec.Volumes = []corev1.Volume{
					{
						Name: "missing-config-map",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "missing",
								},
							},
						},
					},
				}
				eds.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
					{
						Name:      "missing-config-map",
						MountPath: "/etc/missing",
					},
				}
			}, "SlowStartTimeoutExceeded")
		})
	})
})

// These tests may take several minutes to run, check your go test timeout
var _ = Describe("ExtendedDaemonSet e2e successful canary deployment update", func() {
	Context("Initial deployment", func() {
		name := fmt.Sprintf("eds-foo-%d", time.Now().Unix())
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		nodeList := &corev1.NodeList{}

		It("Should deploy EDS", func() {
			Expect(k8sClient.List(ctx, nodeList)).Should(Succeed())

			edsOptions := &testutils.NewExtendedDaemonsetOptions{
				CanaryStrategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					Duration: &metav1.Duration{Duration: 1 * time.Minute},
					Replicas: &intString1,
				},
				RollingUpdate: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
					MaxUnavailable:         &intString10,
					MaxParallelPodCreation: datadoghqv1alpha1.NewInt32(20),
				},
			}

			eds := testutils.NewExtendedDaemonset(namespace, name, "registry.k8s.io/pause:3.0", edsOptions)
			Expect(k8sClient.Create(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.ActiveReplicaSet != ""
			}), timeout, interval).Should(BeTrue())

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			ersKey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.ActiveReplicaSet,
			}
			Eventually(withERS(ersKey, ers, func() bool {
				fmt.Fprintf(GinkgoWriter, "ERS status:\n%s\n", spew.Sdump(ers.Status))
				return ers.Status.Status == "active" && int(ers.Status.Available) == len(nodeList.Items)
			}), timeout, interval).Should(BeTrue())
		})

		It("Should do canary deployment", func() {
			updateFunc := func(eds *datadoghqv1alpha1.ExtendedDaemonSet) {
				eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("registry.k8s.io/pause:3.1")
			}

			Eventually(updateEDS(k8sClient, key, updateFunc), timeout, interval).Should(
				BeTrue(),
				func() string { return "Unable to update the EDS" },
			)

			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			fmt.Fprintf(GinkgoWriter, "EDS status:\n%s\n", spew.Sdump(eds.Status))

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.Canary != nil && eds.Status.Canary.ReplicaSet != ""
			}), timeout, interval).Should(BeTrue())
		})

		It("Should add canary labels", func() {
			canaryPods := &corev1.PodList{}
			listOptions := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCanaryLabelKey: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCanaryLabelValue,
				},
			}
			Eventually(withList(listOptions, canaryPods, "canary pods", func() bool {
				fmt.Fprintf(GinkgoWriter, "canary pods nb: %d ", len(canaryPods.Items))
				return len(canaryPods.Items) == 1
			}), timeout, interval).Should(BeTrue())
		})

		It("Should remove canary labels", func() {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			canaryReplicaSet := eds.Status.Canary.ReplicaSet

			updateFunc := func(eds *datadoghqv1alpha1.ExtendedDaemonSet) {
				if eds.Annotations == nil {
					eds.Annotations = make(map[string]string)
				}
				eds.Annotations[datadoghqv1alpha1.ExtendedDaemonSetCanaryValidAnnotationKey] = canaryReplicaSet
				eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("registry.k8s.io/pause:3.1")
			}

			Eventually(updateEDS(k8sClient, key, updateFunc), timeout, interval).Should(
				BeTrue(),
				func() string { return "Unable to update the EDS" },
			)

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.ActiveReplicaSet == canaryReplicaSet
			}), timeout, interval).Should(BeTrue(), "eds status should be updated")

			canaryPods := &corev1.PodList{}
			listOptions := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCanaryLabelKey: datadoghqv1alpha1.ExtendedDaemonSetReplicaSetCanaryLabelValue,
					datadoghqv1alpha1.ExtendedDaemonSetReplicaSetNameLabelKey:   eds.Status.ActiveReplicaSet,
				},
			}
			Eventually(withList(listOptions, canaryPods, "canary pods", func() bool {
				return len(canaryPods.Items) == 0
			}), timeout, interval).Should(BeTrue())
		})

		It("Should delete EDS", func() {
			Eventually(deleteEDS(k8sClient, key), timeout, interval).Should(BeTrue(), "EDS should be deleted")

			pods := &corev1.PodList{}
			listOptions := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: name,
				},
			}
			Eventually(withList(listOptions, pods, "EDS pods", func() bool {
				return len(pods.Items) == 0
			}), timeout*2, interval).Should(BeTrue(), "All EDS pods should be destroyed")
		})
	})
})

// This test may take ~30s to run, check your go test timeout
var _ = Describe("ExtendedDaemonSet Controller", func() {
	const timeout = time.Second * 30
	const longTimeout = time.Second * 120
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

		It("Add label on a node", func() {
			nodeList := &corev1.NodeList{}
			Expect(k8sClient.List(ctx, nodeList)).Should(Succeed())

			Expect(len(nodeList.Items) > 0).Should(BeTrue())
			node := nodeList.Items[0].DeepCopy()
			if node.Labels == nil {
				node.Labels = make(map[string]string)
			}
			node.Labels["role"] = "eds-setting-worker"
			Expect(k8sClient.Update(ctx, node)).Should(Succeed())
		})

		It("Should use DaemonsetSetting", func() {
			resouresRef := corev1.ResourceList{
				"cpu":    resource.MustParse("0.1"),
				"memory": resource.MustParse("20M"),
			}
			edsNodeSetting := testutils.NewExtendedDaemonsetSetting(namespace, "eds-setting-worker", name, &testutils.NewExtendedDaemonsetSettingOptions{
				Selector: map[string]string{"role": "eds-setting-worker"},
				Resources: map[string]corev1.ResourceRequirements{
					"main": {
						Requests: resouresRef,
					},
				},
			})
			Expect(k8sClient.Create(ctx, edsNodeSetting)).Should(Succeed())

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
			eds := testutils.NewExtendedDaemonset(namespace, name, "registry.k8s.io/pause:3.0", edsOptions)
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
			}), longTimeout, interval).Should(BeTrue(), func() string {
				return fmt.Sprintf(
					"ers.Status.Desired should be equal to ers.Status.Current, status: %#v",
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
				// remove terminated pods
				for index, pod := range podList.Items {
					if pod.DeletionTimestamp != nil {
						podList.Items = append(podList.Items[:index], podList.Items[index+1:]...)
					}
				}
				return len(podList.Items) == 3
			}), timeout, interval).Should(BeTrue())

			// TODO: This loop below does not assert on anything in any way
			for _, pod := range podList.Items {
				if pod.Spec.NodeName == "node-worker" {
					for _, container := range pod.Spec.Containers {
						if container.Name != "main" {
							continue
						}
						if diff := cmp.Diff(resouresRef, container.Resources.Requests); diff != "" {
							fmt.Fprintf(GinkgoWriter, "diff pods resources: %s", diff)
						}
					}
				}
			}
		})
	})
})

// These tests may take several minutes to run, check your go test timeout
var _ = Describe("ExtendedDaemonSet e2e validationMode setting", func() {
	Context("Initial deployment", func() {
		name := fmt.Sprintf("eds-validationmode-%d", time.Now().Unix())
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		nodeList := &corev1.NodeList{}

		It("Should deploy EDS", func() {
			Expect(k8sClient.List(ctx, nodeList)).Should(Succeed())

			edsOptions := &testutils.NewExtendedDaemonsetOptions{
				CanaryStrategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					Replicas:       &intString1,
					ValidationMode: datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeManual,
				},
			}

			eds := testutils.NewExtendedDaemonset(namespace, name, "registry.k8s.io/pause:3.0", edsOptions)
			Expect(k8sClient.Create(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.ActiveReplicaSet != ""
			}), timeout, interval).Should(BeTrue())

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			ersKey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.ActiveReplicaSet,
			}
			Eventually(withERS(ersKey, ers, func() bool {
				fmt.Fprintf(GinkgoWriter, "ERS status:\n%s\n", spew.Sdump(ers.Status))
				return ers.Status.Status == "active" && int(ers.Status.Available) == len(nodeList.Items)
			}), timeout, interval).Should(BeTrue())
		})

		It("Should do canary deployment", func() {
			updateFunc := func(eds *datadoghqv1alpha1.ExtendedDaemonSet) {
				eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("registry.k8s.io/pause:3.1")
			}

			Eventually(updateEDS(k8sClient, key, updateFunc), timeout, interval).Should(
				BeTrue(),
				func() string { return "Unable to update the EDS" },
			)

			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			fmt.Fprintf(GinkgoWriter, "EDS status:\n%s\n", spew.Sdump(eds.Status))

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.Canary != nil && eds.Status.Canary.ReplicaSet != ""
			}), timeout, interval).Should(BeTrue())
		})

		It("Should not validate canary", func() {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			info("EDS status:\n%s\n", spew.Sdump(eds.Status))

			canaryReplicaSet := eds.Status.Canary.ReplicaSet
			Consistently(withEDS(key, eds, func() bool {
				return eds.Spec.Strategy.Canary.Duration == nil && eds.Status.ActiveReplicaSet != canaryReplicaSet
			}), timeout, interval).Should(BeTrue())
		})

		It("Should delete EDS", func() {
			Eventually(deleteEDS(k8sClient, key), timeout, interval).Should(BeTrue(), "EDS should be deleted")

			pods := &corev1.PodList{}
			listOptions := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: name,
				},
			}
			Eventually(withList(listOptions, pods, "EDS pods", func() bool {
				return len(pods.Items) == 0
			}), longTimeout, interval).Should(BeTrue(), "All EDS pods should be destroyed")
		})
	})
})

// These tests may take several minutes to run, check your go test timeout
var _ = Describe("ExtendedDaemonSet e2e rollout not blocked due to already failing pods", func() {
	Context("Deployment with failure pods then rollout to healthy pods", func() {
		name := "eds-fail"

		key := types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		nodeList := &corev1.NodeList{}

		It("Should deploy EDS", func() {
			Expect(k8sClient.List(ctx, nodeList)).Should(Succeed())

			edsOptions := &testutils.NewExtendedDaemonsetOptions{
				CanaryStrategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
					Duration: &metav1.Duration{Duration: 1 * time.Minute},
					Replicas: &intString1,
				},
				RollingUpdate: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
					MaxUnavailable:         &intString1,
					MaxParallelPodCreation: datadoghqv1alpha1.NewInt32(20),
				},
			}

			corrupted_image := "corrupted_image:image_corrupted"
			eds := testutils.NewExtendedDaemonset(namespace, name, corrupted_image, edsOptions)
			Expect(k8sClient.Create(ctx, eds)).Should(Succeed())

			eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.ActiveReplicaSet != ""
			}), timeout, interval).Should(BeTrue())

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			ersKey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.ActiveReplicaSet,
			}
			Eventually(withERS(ersKey, ers, func() bool {
				return ers.Status.Status == "active" && int(ers.Status.Current) == len(nodeList.Items) && int(ers.Status.Ready) == 0
			}), timeout, interval).Should(BeTrue())

		})

		It("Should deploy canary ERS", func() {
			updateFunc := func(eds *datadoghqv1alpha1.ExtendedDaemonSet) {
				eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("registry.k8s.io/pause:3.0")
			}

			Eventually(updateEDS(k8sClient, key, updateFunc), timeout, interval).Should(
				BeTrue(),
				func() string { return "Unable to update the EDS" },
			)

			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanary
			}), timeout, interval).Should(BeTrue())
		})

		It("Should rollout successfully after success of canary - canary ERS should become active ERS", func() {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanary
			}), timeout, interval).Should(BeTrue())

			ersCanary := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			ersCanaryKey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.Canary.ReplicaSet,
			}

			Eventually(withERS(ersCanaryKey, ersCanary, func() bool {
				return ersCanary.Status.Status == "canary"
			}), timeout, interval).Should(BeTrue())

			Eventually(withEDS(key, eds, func() bool {
				return eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRunning
			}), longTimeout, interval).Should(BeTrue())

			ers := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
			ersKey := types.NamespacedName{
				Namespace: namespace,
				Name:      eds.Status.ActiveReplicaSet,
			}

			Eventually(withERS(ersKey, ers, func() bool {
				return ers.Status.Status == "active" && int(ers.Status.Available) == len(nodeList.Items)
			}), timeout, interval).Should(BeTrue())
		})

		It("Should delete EDS", func() {
			Eventually(deleteEDS(k8sClient, key), timeout, interval).Should(BeTrue(), "EDS should be deleted")

			pods := &corev1.PodList{}
			listOptions := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: name,
				},
			}
			Eventually(withList(listOptions, pods, "EDS pods", func() bool {
				return len(pods.Items) == 0
			}), longTimeout, interval).Should(BeTrue(), "All EDS pods should be destroyed")
		})
	})
})

var _ = Describe("ExtendedDaemonSet e2e Pod within MaxSlowStartDuration", func() {
	var (
		name string
		key  types.NamespacedName
	)

	BeforeEach(func() {
		name = fmt.Sprintf("eds-foo-max-slow-%d", time.Now().Unix())
		key = types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}

		info("BeforeEach: Creating EDS %s\n", name)

		edsOptions := &testutils.NewExtendedDaemonsetOptions{
			CanaryStrategy: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
				Duration: &metav1.Duration{Duration: 7 * time.Minute},
				Replicas: &intString2,
				AutoPause: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{
					Enabled:              datadoghqv1alpha1.NewBool(true),
					MaxSlowStartDuration: &metav1.Duration{Duration: 30 * time.Minute},
				},
			},
			RollingUpdate: &datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
				MaxUnavailable:         &intString10,
				MaxParallelPodCreation: datadoghqv1alpha1.NewInt32(20),
			},
		}

		eds := testutils.NewExtendedDaemonset(namespace, name, "registry.k8s.io/pause:3.0", edsOptions)
		Expect(k8sClient.Create(ctx, eds)).Should(Succeed())

		eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
		Eventually(withEDS(key, eds, func() bool {
			return eds.Status.ActiveReplicaSet != ""
		}), timeout, interval).Should(BeTrue())

		info("BeforeEach: Done creating EDS %s - active replicaset: %s\n", name, eds.Status.ActiveReplicaSet)
	})

	AfterEach(func() {
		info("AfterEach: Destroying EDS %s\n", name)
		eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
		Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
		info("AfterEach: Destroying EDS %s - canary replicaset: %s\n", name, eds.Status.Canary.ReplicaSet)
		info("AfterEach: Destroying EDS %s - active replicaset: %s\n", name, eds.Status.ActiveReplicaSet)

		Eventually(deleteEDS(k8sClient, key), timeout, interval).Should(BeTrue(), "EDS should be deleted")

		pods := &corev1.PodList{}
		listOptions := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: name,
			},
		}
		Eventually(withList(listOptions, pods, "EDS pods", func() bool {
			return len(pods.Items) == 0
		}), longTimeout, interval).Should(BeTrue(), "All EDS pods should be destroyed")

		erslist := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSetList{}
		Eventually(withList(listOptions, erslist, "ERS instances", func() bool {
			return len(erslist.Items) == 0
		}), timeout, interval).Should(BeTrue(), "All ERS instances should be destroyed")

		info("AfterEach: Done destroying EDS %s\n", name)
	})

	JustAfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
			Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
			warn("%s - FAILED: EDS status:\n%s\n\n", CurrentGinkgoTestDescription().TestText, spew.Sdump(eds.Status))
		}
	})

	restartOnCannotStartWithinMaxSlowStart := func(configureEDS func(eds *datadoghqv1alpha1.ExtendedDaemonSet), expectedReason string) {
		Eventually(updateEDS(k8sClient, key, configureEDS), timeout, interval).Should(
			BeTrue(),
			func() string { return "Unable to update the EDS" },
		)

		eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
		Expect(k8sClient.Get(ctx, key, eds)).Should(Succeed())
		info("%s: %s - active replicaset: %s\n",
			CurrentGinkgoTestDescription().TestText,
			name, eds.Status.ActiveReplicaSet,
		)

		info("EDS %s - waiting for canary to not be paused\n", name)
		eds = &datadoghqv1alpha1.ExtendedDaemonSet{}
		Eventually(withEDS(key, eds, func() bool {
			return eds.Status.Canary != nil
		}), longTimeout, interval).Should(BeTrue())

		pods := &corev1.PodList{}
		listOptions := []client.ListOption{
			client.InNamespace(namespace),
			client.MatchingLabels{
				datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: name,
			},
		}
		Eventually(withList(listOptions, pods, "EDS pods", func() bool {
			if len(pods.Items) == 0 {
				return false
			}
			for _, item := range pods.Items {
				if len(item.Status.ContainerStatuses) == 0 {
					continue
				}
				for _, status := range item.Status.ContainerStatuses {
					if status.State.Waiting == nil {
						continue
					}
					info("EDS %s - pod %s is in state %s\n", name, item.Name, status.State.Waiting.Reason)
					if (pod.IsCannotStartReason(status.State.Waiting.Reason)) && status.State.Waiting.Reason == expectedReason {
						return true
					}
				}
			}
			return false
		}), longTimeout, interval).Should(BeTrue(), "EDS pod did not hit cannot start state")

		Expect(eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateCanaryPaused).Should(BeFalse())

		info("EDS status:\n%s\n", spew.Sdump(eds.Status))
	}

	Context("When pod has container config error", func() {
		It("Should not promptly auto-pause canary", func() {
			restartOnCannotStartWithinMaxSlowStart(func(eds *datadoghqv1alpha1.ExtendedDaemonSet) {
				eds.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("gcr.io/google-containers/alpine-with-bash:1.0")
				eds.Spec.Template.Spec.Containers[0].Command = []string{
					"tail", "-f", "/dev/null",
				}
				eds.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
					{
						Name: "missing",
						ValueFrom: &corev1.EnvVarSource{
							ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "missing",
								},
								Key: "missing",
							},
						},
					},
				}
			}, "CreateContainerConfigError")
		})
	})
})

func withUpdate(obj client.Object, desc string) condFn {
	return func() bool {
		err := k8sClient.Update(context.Background(), obj)
		if err != nil {
			warn("Failed to update %s: %v\n", desc, err)
			return false
		}
		return true
	}
}

func clearCanaryAnnotations(eds *datadoghqv1alpha1.ExtendedDaemonSet) bool {
	keysToDelete := []string{
		datadoghqv1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey,
		datadoghqv1alpha1.ExtendedDaemonSetCanaryPausedReasonAnnotationKey,
		datadoghqv1alpha1.ExtendedDaemonSetCanaryUnpausedAnnotationKey,
	}
	var updated bool
	for _, key := range keysToDelete {
		if _, ok := eds.Annotations[key]; ok {
			delete(eds.Annotations, key)
			updated = true
		}
	}
	return updated
}

func deleteEDS(k8sclient client.Client, key types.NamespacedName) condFn {
	return func() bool {
		eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
		if err := k8sClient.Get(ctx, key, eds); err != nil {
			if apierrors.IsNotFound(err) {
				return true
			}
			return false
		}
		if err := k8sClient.Delete(ctx, eds); err != nil {
			return false
		}

		if err := k8sClient.Get(ctx, key, eds); apierrors.IsNotFound(err) {
			return true
		}
		warn("Failed to delete eds %v\n", key)
		return false
	}
}

func updateEDS(k8sclient client.Client, key types.NamespacedName, updateFunc func(eds *datadoghqv1alpha1.ExtendedDaemonSet)) condFn {
	return func() bool {
		eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
		if err := k8sClient.Get(ctx, key, eds); err != nil {
			return false
		}

		updateFunc(eds)
		if err := k8sClient.Update(ctx, eds); err != nil {
			return false
		}
		return true
	}
}
