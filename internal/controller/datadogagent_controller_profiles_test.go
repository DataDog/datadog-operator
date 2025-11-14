// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controller

import (
	"sort"
	"strings"
	"time"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/testutils"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"

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
// - All the profile, agent, and node names are random to avoid conflicts
// between tests.

// TODO: these tests are only for DaemonSets. We should add similar tests for
// EDS.

type daemonSetExpectations struct {
	affinity           *v1.Affinity
	containerResources map[apicommon.AgentContainerName]v1.ResourceRequirements
	envVars            []v1.EnvVar
}

type profilesTestScenario struct {
	nodes                []*v1.Node
	agent                *v2alpha1.DatadogAgent
	profiles             []*v1alpha1.DatadogAgentProfile
	expectedDaemonSets   map[types.NamespacedName]daemonSetExpectations
	expectedLabeledNodes map[string]bool

	// When there are conflicts between profiles, their creation timestamp
	// matters because the oldest has precedence. We cannot set the Creation
	// Timestamp, it's set by Kubernetes. Set this to true in tests where it's
	// important that profiles are created with different timestamps.
	waitBetweenProfilesCreation bool
}

var _ = Describe("V2 Controller - DatadogAgentProfile", func() {
	namespace := "default"

	Context("without profiles", func() {
		nodes := []*v1.Node{
			testutils.NewNode(randomKubernetesObjectName(), nil),
			testutils.NewNode(randomKubernetesObjectName(), nil),
		}

		agent := testutils.NewDatadogAgentWithoutFeatures(namespace, randomKubernetesObjectName())

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: affinityForDefaultProfile(),
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName:  {},
					apicommon.TraceAgentContainerName: {},
				},
			},
		}

		testScenario := profilesTestScenario{
			nodes:                nodes,
			agent:                &agent,
			profiles:             nil,
			expectedDaemonSets:   expectedDaemonSets,
			expectedLabeledNodes: nil,
		}

		testProfilesFunc(testScenario)()
	})

	Context("with a profile that does not apply to any node", func() {
		nodes := []*v1.Node{
			testutils.NewNode(randomKubernetesObjectName(), map[string]string{"some-label": "1"}),
			testutils.NewNode(randomKubernetesObjectName(), map[string]string{"some-label": "2"}),
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
				Config: &v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								apicommon.CoreAgentContainerName: {
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
			}, true),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: affinityForDefaultProfile(),
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName:  {},
					apicommon.TraceAgentContainerName: {},
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
										{
											Key:      constants.ProfileLabelKey,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{profile.Name},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: podAntiAffinityForAgents(),
				},
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					apicommon.TraceAgentContainerName: {},
				},
			},
		}

		testScenario := profilesTestScenario{
			nodes:                nodes,
			agent:                &agent,
			profiles:             []*v1alpha1.DatadogAgentProfile{profile},
			expectedDaemonSets:   expectedDaemonSets,
			expectedLabeledNodes: nil,
		}

		testProfilesFunc(testScenario)()
	})

	Context("with a profile that applies to all nodes", func() {
		nodeName1 := randomKubernetesObjectName()
		nodeName2 := randomKubernetesObjectName()
		nodes := []*v1.Node{
			testutils.NewNode(nodeName1, map[string]string{"some-label": "1"}),
			testutils.NewNode(nodeName2, map[string]string{"some-label": "2"}),
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
				Config: &v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								apicommon.CoreAgentContainerName: {
									Resources: &v1.ResourceRequirements{
										Limits: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("2"),
										},
										Requests: map[v1.ResourceName]resource.Quantity{
											v1.ResourceCPU: resource.MustParse("1"),
										},
									},
									Env: []v1.EnvVar{
										{
											Name:  "one",
											Value: "foo",
										},
										{
											Name: "two",
											ValueFrom: &v1.EnvVarSource{
												FieldRef: &v1.ObjectFieldSelector{
													FieldPath: common.FieldPathStatusPodIP,
												},
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

		profileDaemonSetName := types.NamespacedName{
			Namespace: namespace,
			Name: agentprofile.DaemonSetName(types.NamespacedName{
				Namespace: profile.Namespace,
				Name:      profile.Name,
			}, true),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: affinityForDefaultProfile(),
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName:  {},
					apicommon.TraceAgentContainerName: {},
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
										{
											Key:      constants.ProfileLabelKey,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{profile.Name},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: podAntiAffinityForAgents(),
				},
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					apicommon.TraceAgentContainerName: {},
				},
				envVars: []v1.EnvVar{
					{
						Name:  "one",
						Value: "foo",
					},
					{
						Name: "two",
						ValueFrom: &v1.EnvVarSource{
							FieldRef: &v1.ObjectFieldSelector{
								FieldPath: common.FieldPathStatusPodIP,
							},
						},
					},
				},
			},
		}

		expectedLabeledNodes := map[string]bool{
			nodeName1: true,
			nodeName2: true,
		}

		testScenario := profilesTestScenario{
			nodes:                nodes,
			agent:                &agent,
			profiles:             []*v1alpha1.DatadogAgentProfile{profile},
			expectedDaemonSets:   expectedDaemonSets,
			expectedLabeledNodes: expectedLabeledNodes,
		}

		testProfilesFunc(testScenario)()
	})

	Context("with a profile that applies to some nodes", func() {
		nodeName1 := randomKubernetesObjectName()
		nodeName2 := randomKubernetesObjectName()
		nodes := []*v1.Node{
			testutils.NewNode(nodeName1, map[string]string{"some-label": "1"}),
			testutils.NewNode(nodeName2, map[string]string{"some-label": "2"}),
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
				Config: &v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								apicommon.CoreAgentContainerName: {
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
			}, true),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: affinityForDefaultProfile(),
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName:  {},
					apicommon.TraceAgentContainerName: {},
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
										{
											Key:      constants.ProfileLabelKey,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{profile.Name},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: podAntiAffinityForAgents(),
				},
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					apicommon.TraceAgentContainerName: {},
				},
			},
		}

		expectedLabeledNodes := map[string]bool{
			nodeName1: true,
		}

		testScenario := profilesTestScenario{
			nodes:                nodes,
			agent:                &agent,
			profiles:             []*v1alpha1.DatadogAgentProfile{profile},
			expectedDaemonSets:   expectedDaemonSets,
			expectedLabeledNodes: expectedLabeledNodes,
		}

		testProfilesFunc(testScenario)()
	})

	Context("with several profiles that don't conflict between them", func() {
		nodeName1 := randomKubernetesObjectName()
		nodeName2 := randomKubernetesObjectName()
		nodeName3 := randomKubernetesObjectName()
		nodes := []*v1.Node{
			testutils.NewNode(nodeName1, map[string]string{"some-label": "1"}),
			testutils.NewNode(nodeName2, map[string]string{"some-label": "2"}),
			testutils.NewNode(nodeName3, map[string]string{"some-label": "3"}),
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
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
									apicommon.CoreAgentContainerName: {
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
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
									apicommon.CoreAgentContainerName: {
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
			}, true),
		}

		profile2DaemonSetName := types.NamespacedName{
			Namespace: namespace,
			Name: agentprofile.DaemonSetName(types.NamespacedName{
				Namespace: profiles[1].Namespace,
				Name:      profiles[1].Name,
			}, true),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: affinityForDefaultProfile(),
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName:  {},
					apicommon.TraceAgentContainerName: {},
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
										{
											Key:      constants.ProfileLabelKey,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{profiles[0].Name},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: podAntiAffinityForAgents(),
				},
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					apicommon.TraceAgentContainerName: {},
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
										{
											Key:      constants.ProfileLabelKey,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{profiles[1].Name},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: podAntiAffinityForAgents(),
				},
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("4"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("3"),
						},
					},
					apicommon.TraceAgentContainerName: {},
				},
			},
		}

		expectedLabeledNodes := map[string]bool{
			nodeName1: true,
			nodeName2: true,
		}

		testScenario := profilesTestScenario{
			nodes:                nodes,
			agent:                &agent,
			profiles:             profiles,
			expectedDaemonSets:   expectedDaemonSets,
			expectedLabeledNodes: expectedLabeledNodes,
		}

		testProfilesFunc(testScenario)()
	})

	Context("with several profiles that conflict between them", func() {
		nodeName1 := randomKubernetesObjectName()
		nodeName2 := randomKubernetesObjectName()
		nodes := []*v1.Node{
			testutils.NewNode(nodeName1, map[string]string{"some-label": "1"}),
			testutils.NewNode(nodeName2, map[string]string{"some-label": "2"}),
		}

		// The order is important in this test. We cannot set the Creation
		// Timestamp here, it will be set by Kubernetes, and the objects will be
		// created in the order specified here.
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
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
									apicommon.CoreAgentContainerName: {
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
								Values:   []string{"1", "2"}, // Conflicts with the above one
							},
						},
					},
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
									apicommon.CoreAgentContainerName: {
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
			}, true),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: affinityForDefaultProfile(),
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName:  {},
					apicommon.TraceAgentContainerName: {},
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
										{
											Key:      constants.ProfileLabelKey,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{profiles[0].Name},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: podAntiAffinityForAgents(),
				},
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					apicommon.TraceAgentContainerName: {},
				},
			},
			// Don't expect a DaemonSet for the conflicting profile
		}

		expectedLabeledNodes := map[string]bool{
			nodeName1: true,
		}

		testScenario := profilesTestScenario{
			nodes:                       nodes,
			agent:                       &agent,
			profiles:                    profiles,
			expectedDaemonSets:          expectedDaemonSets,
			expectedLabeledNodes:        expectedLabeledNodes,
			waitBetweenProfilesCreation: true,
		}

		testProfilesFunc(testScenario)()
	})

	Context("with a profile that applies and an agent with some resource overrides", func() {
		nodeName1 := randomKubernetesObjectName()
		nodeName2 := randomKubernetesObjectName()
		nodes := []*v1.Node{
			testutils.NewNode(nodeName1, map[string]string{"some-label": "1"}),
			testutils.NewNode(nodeName2, map[string]string{"some-label": "2"}),
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
				Config: &v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								apicommon.CoreAgentContainerName: {
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
				Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					apicommon.CoreAgentContainerName: {
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
			}, true),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: affinityForDefaultProfile(),
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("4"),
							v1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("3"),
							v1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					apicommon.TraceAgentContainerName: {},
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
										{
											Key:      constants.ProfileLabelKey,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{profile.Name},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: podAntiAffinityForAgents(),
				},
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("2"),
							v1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("1"),
							v1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					apicommon.TraceAgentContainerName: {},
				},
			},
		}

		expectedLabeledNodes := map[string]bool{
			nodeName1: true,
		}

		testScenario := profilesTestScenario{
			nodes:                nodes,
			agent:                &agent,
			profiles:             []*v1alpha1.DatadogAgentProfile{profile},
			expectedDaemonSets:   expectedDaemonSets,
			expectedLabeledNodes: expectedLabeledNodes,
		}

		testProfilesFunc(testScenario)()
	})

	Context("with a profile that has more than one node selector requirement", func() {
		nodeName1 := randomKubernetesObjectName()
		nodeName2 := randomKubernetesObjectName()
		nodes := []*v1.Node{
			testutils.NewNode(nodeName1, map[string]string{"a": "1", "b": "2"}),
			testutils.NewNode(nodeName2, map[string]string{"a": "1"}),
		}

		profile := &v1alpha1.DatadogAgentProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomKubernetesObjectName(),
				Namespace: namespace,
			},
			Spec: v1alpha1.DatadogAgentProfileSpec{
				ProfileAffinity: &v1alpha1.ProfileAffinity{
					ProfileNodeAffinity: []v1.NodeSelectorRequirement{ // Applies only to the first node
						{
							Key:      "a",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"1"},
						},
						{
							Key:      "b",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"2"},
						},
					},
				},
				Config: &v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								apicommon.CoreAgentContainerName: {
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
			}, true),
		}

		expectedDaemonSets := map[types.NamespacedName]daemonSetExpectations{
			defaultDaemonSetNamespacedName(namespace, &agent): {
				affinity: affinityForDefaultProfile(),
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName:  {},
					apicommon.TraceAgentContainerName: {},
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
											Key:      "a",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"1"},
										},
										{
											Key:      "b",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"2"},
										},
										{
											Key:      constants.ProfileLabelKey,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{profile.Name},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: podAntiAffinityForAgents(),
				},
				containerResources: map[apicommon.AgentContainerName]v1.ResourceRequirements{
					apicommon.CoreAgentContainerName: {
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("2"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU: resource.MustParse("1"),
						},
					},
					apicommon.TraceAgentContainerName: {},
				},
			},
		}

		expectedLabeledNodes := map[string]bool{
			nodeName1: true,
		}

		testScenario := profilesTestScenario{
			nodes:                nodes,
			agent:                &agent,
			profiles:             []*v1alpha1.DatadogAgentProfile{profile},
			expectedDaemonSets:   expectedDaemonSets,
			expectedLabeledNodes: expectedLabeledNodes,
		}

		testProfilesFunc(testScenario)()
	})
})

func testProfilesFunc(testScenario profilesTestScenario) func() {
	return func() {
		BeforeEach(func() {
			for _, node := range testScenario.nodes {
				createKubernetesObject(k8sClient, node)
			}

			for _, profile := range testScenario.profiles {
				createKubernetesObject(k8sClient, profile)

				if testScenario.waitBetweenProfilesCreation {
					time.Sleep(2 * time.Second)
				}
			}

			createKubernetesObject(k8sClient, testScenario.agent)
		})

		AfterEach(func() {
			for _, profile := range testScenario.profiles {
				deleteKubernetesObject(k8sClient, profile)
			}

			deleteKubernetesObject(k8sClient, testScenario.agent)

			for _, node := range testScenario.nodes {
				deleteKubernetesObject(k8sClient, node)
			}
		})

		It("should create the expected DaemonSets and label nodes with profiles", func() {
			for namespacedName, expectedDaemonSet := range testScenario.expectedDaemonSets {
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

			// TODO: commented for now. We need to find a reliable way to test
			// this. At the moment the test is flaky. I believe it's because the
			// finalizer of agents of other tests interferes with the reconciler
			// of the current test because both update the node labels.

			// Check that only the nodes with profiles are labeled
			// In some runs, this takes a bit of time, not sure why.
			/*
				Eventually(func() bool {
					nodeList := v1.NodeList{}
					err := k8sClient.List(context.TODO(), &nodeList)
					Expect(err).ToNot(HaveOccurred())

					for _, node := range nodeList.Items {
						_, expectLabel := testScenario.expectedLabeledNodes[node.Name]

						if expectLabel && node.Labels[constants.ProfileLabelKey] != "true" ||
							!expectLabel && node.Labels[constants.ProfileLabelKey] != "" {
							return false
						}
					}

					return true
				}, 1*time.Minute, 1*time.Second).Should(BeTrue())
			*/
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

func affinityForDefaultProfile() *v1.Affinity {
	return &v1.Affinity{
		NodeAffinity: &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      constants.ProfileLabelKey,
								Operator: v1.NodeSelectorOpDoesNotExist,
							},
						},
					},
				},
			},
		},
		PodAntiAffinity: podAntiAffinityForAgents(),
	}
}

func podAntiAffinityForAgents() *v1.PodAntiAffinity {
	return &v1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      apicommon.AgentDeploymentComponentLabelKey,
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"agent"},
						},
					},
				},
				TopologyKey: v1.LabelHostname,
			},
		},
	}
}
