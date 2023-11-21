// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration_v2
// +build integration_v2

package controllers

import (
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/testutils"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// This file contains integration tests for Datadog Agent Profiles. They define
// some nodes, an agent, some profiles, and verify that the expected DaemonSets
// are created.
//
// Considerations:
// - The nodes defined in suite_v2_test.go are used in these tests, but they are
// not relevant, because they don't have any labels defined.
// - All the profile and agent names are random to avoid conflicts between tests.

// TODO: these tests are only for DaemonSets. We should add similar tests for
// EDS.

type daemonSetExpectations struct {
	affinity           *v1.Affinity
	containerResources map[common.AgentContainerName]v1.ResourceRequirements
}

var _ = Describe("V2 Controller - DatadogAgentProfile", func() {
	namespace := "default"

	Context("without profiles", func() {
		nodes := []*v1.Node{
			testutils.NewNode("test-profiles-node-1", nil),
			testutils.NewNode("test-profiles-node-2", nil),
		}

		agent := testutils.NewDatadogAgentWithoutFeatures(namespace, randomKubernetesObjectName())

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: nil,
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName:    {},
					common.ProcessAgentContainerName: {},
				},
			},
		}

		testProfilesFunc(nil, &agent, nodes, expectedDaemonSets)()
	})

	Context("with a profile that does not apply to any node", func() {
		nodes := []*v1.Node{
			testutils.NewNode("test-profiles-node-1", map[string]string{"some-label": "1"}),
			testutils.NewNode("test-profiles-node-2", map[string]string{"some-label": "2"}),
		}

		profile := &v1alpha1.DatadogAgentProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomKubernetesObjectName(),
				Namespace: namespace,
			},
			Spec: v1alpha1.DatadogAgentProfileSpec{
				ProfileAffinity: &v1alpha1.ProfileAffinity{
					ProfileNodeAffinity: []v1.NodeSelectorRequirement{
						{
							Key:      "some-label",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"3"}, // This does not apply to any node
						},
					},
				},
				Config: &v1alpha1.Config{
					Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
						v1alpha1.NodeAgentComponentName: {
							Containers: map[common.AgentContainerName]*v1alpha1.Container{
								common.CoreAgentContainerName: {
									Resources: &v1.ResourceRequirements{
										Limits: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("2"),
										},
										Requests: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("1"),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		agent := testutils.NewDatadogAgentWithoutFeatures(namespace, randomKubernetesObjectName())

		profileDaemonSetName := types.NamespacedName{
			Namespace: namespace,
			Name: agentprofile.DaemonSetName(types.NamespacedName{
				Namespace: profile.Namespace,
				Name:      profile.Name,
			}),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpNotIn,
											Values:   []string{"3"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName:    {},
					common.ProcessAgentContainerName: {},
				},
			},
			profileDaemonSetName: {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"3"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					common.ProcessAgentContainerName: {},
				},
			},
		}

		testProfilesFunc([]*v1alpha1.DatadogAgentProfile{profile}, &agent, nodes, expectedDaemonSets)()
	})

	Context("with a profile that applies to all nodes", func() {
		nodes := []*v1.Node{
			testutils.NewNode("test-profiles-node-1", map[string]string{"some-label": "1"}),
			testutils.NewNode("test-profiles-node-2", map[string]string{"some-label": "2"}),
		}

		profile := &v1alpha1.DatadogAgentProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomKubernetesObjectName(),
				Namespace: namespace,
			},
			Spec: v1alpha1.DatadogAgentProfileSpec{
				ProfileAffinity: &v1alpha1.ProfileAffinity{
					ProfileNodeAffinity: []v1.NodeSelectorRequirement{
						{
							Key:      "some-label",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"1", "2"}, // Applies to all nodes
						},
					},
				},
				Config: &v1alpha1.Config{
					Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
						v1alpha1.NodeAgentComponentName: {
							Containers: map[common.AgentContainerName]*v1alpha1.Container{
								common.CoreAgentContainerName: {
									Resources: &v1.ResourceRequirements{
										Limits: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("2"),
										},
										Requests: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("1"),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		agent := testutils.NewDatadogAgentWithoutFeatures(namespace, randomKubernetesObjectName())

		profileDaemonSetName := types.NamespacedName{
			Namespace: namespace,
			Name: agentprofile.DaemonSetName(types.NamespacedName{
				Namespace: profile.Namespace,
				Name:      profile.Name,
			}),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpNotIn,
											Values:   []string{"1", "2"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName:    {},
					common.ProcessAgentContainerName: {},
				},
			},
			profileDaemonSetName: {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"1", "2"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					common.ProcessAgentContainerName: {},
				},
			},
		}

		testProfilesFunc([]*v1alpha1.DatadogAgentProfile{profile}, &agent, nodes, expectedDaemonSets)()
	})

	Context("with a profile that applies to some nodes", func() {
		nodes := []*v1.Node{
			testutils.NewNode("test-profiles-node-1", map[string]string{"some-label": "1"}),
			testutils.NewNode("test-profiles-node-2", map[string]string{"some-label": "2"}),
		}

		profile := &v1alpha1.DatadogAgentProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomKubernetesObjectName(),
				Namespace: namespace,
			},
			Spec: v1alpha1.DatadogAgentProfileSpec{
				ProfileAffinity: &v1alpha1.ProfileAffinity{
					ProfileNodeAffinity: []v1.NodeSelectorRequirement{
						{
							Key:      "some-label",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"1"}, // Applies to some nodes
						},
					},
				},
				Config: &v1alpha1.Config{
					Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
						v1alpha1.NodeAgentComponentName: {
							Containers: map[common.AgentContainerName]*v1alpha1.Container{
								common.CoreAgentContainerName: {
									Resources: &v1.ResourceRequirements{
										Limits: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("2"),
										},
										Requests: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("1"),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		agent := testutils.NewDatadogAgentWithoutFeatures(namespace, randomKubernetesObjectName())

		profileDaemonSetName := types.NamespacedName{
			Namespace: namespace,
			Name: agentprofile.DaemonSetName(types.NamespacedName{
				Namespace: profile.Namespace,
				Name:      profile.Name,
			}),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpNotIn,
											Values:   []string{"1"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName:    {},
					common.ProcessAgentContainerName: {},
				},
			},
			profileDaemonSetName: {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"1"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					common.ProcessAgentContainerName: {},
				},
			},
		}

		testProfilesFunc([]*v1alpha1.DatadogAgentProfile{profile}, &agent, nodes, expectedDaemonSets)()
	})

	Context("with several profiles that don't conflict between them", func() {
		nodes := []*v1.Node{
			testutils.NewNode("test-profiles-node-1", map[string]string{"some-label": "1"}),
			testutils.NewNode("test-profiles-node-2", map[string]string{"some-label": "2"}),
			testutils.NewNode("test-profiles-node-3", map[string]string{"some-label": "3"}),
		}

		profiles := []*v1alpha1.DatadogAgentProfile{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      randomKubernetesObjectName(),
					Namespace: namespace,
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []v1.NodeSelectorRequirement{
							{
								Key:      "some-label",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"1"}, // Applies to one node
							},
						},
					},
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {
								Containers: map[common.AgentContainerName]*v1alpha1.Container{
									common.CoreAgentContainerName: {
										Resources: &v1.ResourceRequirements{
											Limits: map[v1.ResourceName]resource.Quantity{
												v1.ResourceCPU: resource.MustParse("2"),
											},
											Requests: map[v1.ResourceName]resource.Quantity{
												v1.ResourceCPU: resource.MustParse("1"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      randomKubernetesObjectName(),
					Namespace: namespace,
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []v1.NodeSelectorRequirement{
							{
								Key:      "some-label",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"2"}, // Applies to one node
							},
						},
					},
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {
								Containers: map[common.AgentContainerName]*v1alpha1.Container{
									common.CoreAgentContainerName: {
										Resources: &v1.ResourceRequirements{
											Limits: map[v1.ResourceName]resource.Quantity{
												v1.ResourceCPU: resource.MustParse("4"),
											},
											Requests: map[v1.ResourceName]resource.Quantity{
												v1.ResourceCPU: resource.MustParse("3"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		agent := testutils.NewDatadogAgentWithoutFeatures(namespace, randomKubernetesObjectName())

		profile1DaemonSetName := types.NamespacedName{
			Namespace: namespace,
			Name: agentprofile.DaemonSetName(types.NamespacedName{
				Namespace: profiles[0].Namespace,
				Name:      profiles[0].Name,
			}),
		}

		profile2DaemonSetName := types.NamespacedName{
			Namespace: namespace,
			Name: agentprofile.DaemonSetName(types.NamespacedName{
				Namespace: profiles[1].Namespace,
				Name:      profiles[1].Name,
			}),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpNotIn,
											Values:   []string{"1"},
										},
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpNotIn,
											Values:   []string{"2"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName:    {},
					common.ProcessAgentContainerName: {},
				},
			},
			profile1DaemonSetName: {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"1"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					common.ProcessAgentContainerName: {},
				},
			},
			profile2DaemonSetName: {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"2"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("4"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("3"),
						},
					},
					common.ProcessAgentContainerName: {},
				},
			},
		}

		testProfilesFunc(profiles, &agent, nodes, expectedDaemonSets)()
	})

	// TODO: marked this one as "pending" because we haven't implemented support for conflicting profiles yet.
	PContext("with several profiles that conflict between them", func() {
		nodes := []*v1.Node{
			testutils.NewNode("test-profiles-node-1", map[string]string{"some-label": "1"}),
			testutils.NewNode("test-profiles-node-2", map[string]string{"some-label": "2"}),
		}

		creationTimeStampOlderProfile := metav1.NewTime(metav1.Now().Add(-1 * time.Hour))
		creationTimeStampNewerProfile := metav1.NewTime(creationTimeStampOlderProfile.Add(30 * time.Minute))

		profiles := []*v1alpha1.DatadogAgentProfile{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              randomKubernetesObjectName(),
					Namespace:         namespace,
					CreationTimestamp: creationTimeStampOlderProfile,
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []v1.NodeSelectorRequirement{
							{
								Key:      "some-label",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"1"}, // Applies to one node
							},
						},
					},
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {
								Containers: map[common.AgentContainerName]*v1alpha1.Container{
									common.CoreAgentContainerName: {
										Resources: &v1.ResourceRequirements{
											Limits: map[v1.ResourceName]resource.Quantity{
												v1.ResourceCPU: resource.MustParse("2"),
											},
											Requests: map[v1.ResourceName]resource.Quantity{
												v1.ResourceCPU: resource.MustParse("1"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              randomKubernetesObjectName(),
					Namespace:         namespace,
					CreationTimestamp: creationTimeStampNewerProfile, // Newer, so won't apply when conflict
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []v1.NodeSelectorRequirement{
							{
								Key:      "some-label",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"1", "2"}, // Conflicts with the above one
							},
						},
					},
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {
								Containers: map[common.AgentContainerName]*v1alpha1.Container{
									common.CoreAgentContainerName: {
										Resources: &v1.ResourceRequirements{
											Limits: map[v1.ResourceName]resource.Quantity{
												v1.ResourceCPU: resource.MustParse("4"),
											},
											Requests: map[v1.ResourceName]resource.Quantity{
												v1.ResourceCPU: resource.MustParse("3"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		agent := testutils.NewDatadogAgentWithoutFeatures(namespace, randomKubernetesObjectName())

		profile1DaemonSetName := types.NamespacedName{
			Namespace: namespace,
			Name: agentprofile.DaemonSetName(types.NamespacedName{
				Namespace: profiles[0].Namespace,
				Name:      profiles[0].Name,
			}),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpNotIn,
											Values:   []string{"1"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName:    {},
					common.ProcessAgentContainerName: {},
				},
			},
			profile1DaemonSetName: {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"1"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					common.ProcessAgentContainerName: {},
				},
			},
			// Don't expect a DaemonSet for the conflicting profile
		}

		testProfilesFunc(profiles, &agent, nodes, expectedDaemonSets)()
	})

	Context("with a profile that applies and an agent with some resource overrides", func() {
		nodes := []*v1.Node{
			testutils.NewNode("test-profiles-node-1", map[string]string{"some-label": "1"}),
			testutils.NewNode("test-profiles-node-2", map[string]string{"some-label": "2"}),
		}

		profile := &v1alpha1.DatadogAgentProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomKubernetesObjectName(),
				Namespace: namespace,
			},
			Spec: v1alpha1.DatadogAgentProfileSpec{
				ProfileAffinity: &v1alpha1.ProfileAffinity{
					ProfileNodeAffinity: []v1.NodeSelectorRequirement{
						{
							Key:      "some-label",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"1"}, // Applies to some nodes
						},
					},
				},
				Config: &v1alpha1.Config{
					Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
						v1alpha1.NodeAgentComponentName: {
							Containers: map[common.AgentContainerName]*v1alpha1.Container{
								common.CoreAgentContainerName: {
									Resources: &v1.ResourceRequirements{
										Limits: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("2"),
										},
										Requests: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("1"),
										},
									},
								},
							},
						},
					},
				},
			},
		}

		agent := testutils.NewDatadogAgentWithoutFeatures(namespace, randomKubernetesObjectName())
		agent.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {
				Containers: map[common.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					common.CoreAgentContainerName: {
						Resources: &v1.ResourceRequirements{
							Limits: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    resource.MustParse("4"),     // defined also in profile
								v1.ResourceMemory: resource.MustParse("256Mi"), // not defined in profile
							},
							Requests: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    resource.MustParse("3"),     // defined also in profile
								v1.ResourceMemory: resource.MustParse("128Mi"), // not defined in profile
							},
						},
					},
				},
			},
		}

		profileDaemonSetName := types.NamespacedName{
			Namespace: namespace,
			Name: agentprofile.DaemonSetName(types.NamespacedName{
				Namespace: profile.Namespace,
				Name:      profile.Name,
			}),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpNotIn,
											Values:   []string{"1"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("4"),
							v1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("3"),
							v1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					common.ProcessAgentContainerName: {},
				},
			},
			profileDaemonSetName: {
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "some-label",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"1"},
										},
									},
								},
							},
						},
					},
				},
				containerResources: map[common.AgentContainerName]v1.ResourceRequirements{
					common.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("2"),
							v1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("1"),
							v1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					common.ProcessAgentContainerName: {},
				},
			},
		}

		testProfilesFunc([]*v1alpha1.DatadogAgentProfile{profile}, &agent, nodes, expectedDaemonSets)()
	})

	// TODO: marked this one as pending because the current approach of
	// generating the "opposite" affinity for the default profile does not work
	// in this case.
	PContext("with a profile that has more than one node selector requirement", func() {
	})
})

func testProfilesFunc(profiles []*v1alpha1.DatadogAgentProfile, agent *v2alpha1.DatadogAgent, nodes []*v1.Node, expectedDaemonSets map[types.NamespacedName]daemonSetExpectations) func() {
	return func() {
		BeforeEach(func() {
			for _, node := range nodes {
				createKubernetesObject(k8sClient, node)
			}

			createKubernetesObject(k8sClient, agent)

			for _, profile := range profiles {
				createKubernetesObject(k8sClient, profile)
			}
		})

		AfterEach(func() {
			for _, profile := range profiles {
				deleteKubernetesObject(k8sClient, profile)
			}

			deleteKubernetesObject(k8sClient, agent)

			for _, node := range nodes {
				deleteKubernetesObject(k8sClient, node)
			}
		})

		It("should create the expected DaemonSets", func() {
			for namespacedName, expectedDaemonSet := range expectedDaemonSets {
				storedDaemonSet := &appsv1.DaemonSet{}

				// Wait until the DaemonSet is created
				getObjectAndCheck(storedDaemonSet, namespacedName, func() bool { return true })

				sortAffinityRequirements(storedDaemonSet.Spec.Template.Spec.Affinity)
				sortAffinityRequirements(expectedDaemonSet.affinity)
				Expect(storedDaemonSet.Spec.Template.Spec.Affinity).Should(Equal(expectedDaemonSet.affinity))

				// Check the limits and requests for each container
				Expect(len(storedDaemonSet.Spec.Template.Spec.Containers)).Should(Equal(len(expectedDaemonSet.containerResources)))
				for expectedContainerName, expectedResources := range expectedDaemonSet.containerResources {
					for _, container := range storedDaemonSet.Spec.Template.Spec.Containers {
						if container.Name == string(expectedContainerName) {
							Expect(len(container.Resources.Requests)).Should(Equal(len(expectedResources.Requests)))
							Expect(len(container.Resources.Limits)).Should(Equal(len(expectedResources.Limits)))

							for resourceName, expectedRequest := range expectedResources.Requests {
								quantityCmp := expectedRequest.Cmp(container.Resources.Requests[resourceName])
								Expect(quantityCmp).Should(BeZero()) // Cmp returns 0 if the quantities are equal
							}

							for resourceName, expectedLimit := range expectedResources.Limits {
								quantityCmp := expectedLimit.Cmp(container.Resources.Limits[resourceName])
								Expect(quantityCmp).Should(BeZero()) // Cmp returns 0 if the quantities are equal
							}
						}
					}
				}
			}
		})
	}
}

func randomKubernetesObjectName() string {
	return strings.ToLower(utils.GenerateRandomString(10))
}

func defaultDaemonSetNamespacedName(namespace string, agent *v2alpha1.DatadogAgent) types.NamespacedName {
	return types.NamespacedName{
		Namespace: namespace,
		Name:      component.GetAgentName(agent),
	}
}

// sortAffinityRequirements sorts the affinity requirements in the given
// affinity, but only takes into account the attributes relevant to agent
// profiles.
func sortAffinityRequirements(affinity *v1.Affinity) {
	if affinity == nil ||
		affinity.NodeAffinity == nil ||
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return
	}

	nodeSelectorTerms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms

	for _, nodeSelectorTerm := range nodeSelectorTerms {
		for _, matchExpression := range nodeSelectorTerm.MatchExpressions {
			sort.Strings(matchExpression.Values)
		}

		sort.Slice(nodeSelectorTerm.MatchExpressions, func(i, j int) bool {
			if nodeSelectorTerm.MatchExpressions[i].Key != nodeSelectorTerm.MatchExpressions[j].Key {
				return nodeSelectorTerm.MatchExpressions[i].Key < nodeSelectorTerm.MatchExpressions[j].Key
			}

			if nodeSelectorTerm.MatchExpressions[i].Operator != nodeSelectorTerm.MatchExpressions[j].Operator {
				return nodeSelectorTerm.MatchExpressions[i].Operator < nodeSelectorTerm.MatchExpressions[j].Operator
			}

			return strings.Join(nodeSelectorTerm.MatchExpressions[i].Values, ",") <
				strings.Join(nodeSelectorTerm.MatchExpressions[j].Values, ",")
		})
	}

	sort.Slice(nodeSelectorTerms, func(i, j int) bool {
		return nodeSelectorTerms[i].String() < nodeSelectorTerms[j].String()
	})
}
