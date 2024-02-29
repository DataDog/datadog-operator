// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
)

const testNamespace = "default"

func TestProfilesToApply(t *testing.T) {
	t1 := time.Now()
	t2 := t1.Add(time.Minute)
	t3 := t2.Add(time.Minute)

	tests := []struct {
		name                           string
		profiles                       []v1alpha1.DatadogAgentProfile
		nodes                          []v1.Node
		expectedProfiles               []v1alpha1.DatadogAgentProfile
		expectedProfilesAppliedPerNode map[string]types.NamespacedName
	}{
		{
			name:     "no profiles",
			profiles: []v1alpha1.DatadogAgentProfile{},
			nodes: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"os": "linux",
						},
					},
				},
			},
			expectedProfiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: nil, // Applies to all nodes
						Config:          nil, // No overrides
					},
				},
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: "",
					Name:      "default",
				},
			},
		},
		{
			name: "several non-conflicting profiles",
			profiles: []v1alpha1.DatadogAgentProfile{
				exampleProfileForLinux(),
				exampleProfileForWindows(),
			},
			nodes: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"os": "linux",
						},
					},
				},
			},
			expectedProfiles: []v1alpha1.DatadogAgentProfile{
				exampleProfileForLinux(),
				exampleProfileForWindows(),
				defaultProfile(),
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			},
		},
		{
			// This test defines 3 profiles created in this order: profile-2,
			// profile-1, profile-3 (not sorted here to make sure that the code does).
			// - profile-1 and profile-2 conflict, but profile-2 is the oldest,
			// so it wins.
			// - profile-1 and profile-3 conflict, but profile-1 is not applied
			// because of the conflict with profile-2, so profile-3 should be.
			// So in this case, the returned profiles should be profile-2,
			// profile-3 and a default one.
			name: "several conflicting profiles with different creation timestamps",
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              "profile-1",
						CreationTimestamp: metav1.NewTime(t2),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "a",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"1"},
								},
							},
						},
						Config: configWithCPURequestOverrideForCoreAgent("100m"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              "profile-2",
						CreationTimestamp: metav1.NewTime(t1),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "b",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"1"},
								},
							},
						},
						Config: configWithCPURequestOverrideForCoreAgent("200m"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              "profile-3",
						CreationTimestamp: metav1.NewTime(t3),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "c",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"1"},
								},
							},
						},
						Config: configWithCPURequestOverrideForCoreAgent("300m"),
					},
				},
			},
			nodes: []v1.Node{
				// node1 matches profile-1 and profile-3
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"a": "1",
							"c": "1",
						},
					},
				},
				// node2 matches profile-2
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"b": "1",
						},
					},
				},
				// node3 matches profile-1 and profile-2
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							"a": "1",
							"b": "1",
						},
					},
				},
			},
			expectedProfiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              "profile-2",
						CreationTimestamp: metav1.NewTime(t1),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "b",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"1"},
								},
							},
						},
						Config: configWithCPURequestOverrideForCoreAgent("200m"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              "profile-3",
						CreationTimestamp: metav1.NewTime(t3),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "c",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"1"},
								},
							},
						},
						Config: configWithCPURequestOverrideForCoreAgent("300m"),
					},
				},
				defaultProfile(),
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "profile-3",
				},
				"node2": {
					Namespace: testNamespace,
					Name:      "profile-2",
				},
				"node3": {
					Namespace: testNamespace,
					Name:      "profile-2",
				},
			},
		},
		{
			// This test defines 3 profiles with the same creation timestamp:
			// profile-2, profile-1, profile-3 (not sorted alphabetically here
			// to make sure that the code does).
			// The 3 profiles conflict and only profile-1 should apply because
			// it's the first one alphabetically.
			name: "conflicting profiles with the same creation timestamp",
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              "profile-2",
						CreationTimestamp: metav1.NewTime(t1),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "a",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"1"},
								},
							},
						},
						Config: configWithCPURequestOverrideForCoreAgent("100m"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              "profile-1",
						CreationTimestamp: metav1.NewTime(t1),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "b",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"1"},
								},
							},
						},
						Config: configWithCPURequestOverrideForCoreAgent("200m"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              "profile-3",
						CreationTimestamp: metav1.NewTime(t1),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "c",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"1"},
								},
							},
						},
						Config: configWithCPURequestOverrideForCoreAgent("300m"),
					},
				},
			},
			nodes: []v1.Node{
				// matches all profiles
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"a": "1",
							"b": "1",
							"c": "1",
						},
					},
				},
			},
			expectedProfiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         testNamespace,
						Name:              "profile-1",
						CreationTimestamp: metav1.NewTime(t1),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "b",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"1"},
								},
							},
						},
						Config: configWithCPURequestOverrideForCoreAgent("200m"),
					},
				},
				defaultProfile(),
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "profile-1",
				},
			},
		},
		{
			name: "invalid profile",
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      "invalid",
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						Config: configWithCPURequestOverrideForCoreAgent("100m"),
					},
				},
			},
			nodes: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"os": "linux",
						},
					},
				},
			},
			expectedProfiles: []v1alpha1.DatadogAgentProfile{
				defaultProfile(),
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: "",
					Name:      "default",
				},
			},
		},
		{
			name: "invalid profiles + valid profiles",
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      "invalid-no-affinity",
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						Config: configWithCPURequestOverrideForCoreAgent("100m"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      "invalid-no-config",
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{
									Key:      "os",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"linux"},
								},
							},
						},
					},
				},
				exampleProfileForLinux(),
				exampleProfileForWindows(),
			},
			nodes: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"os": "linux",
						},
					},
				},
			},
			expectedProfiles: []v1alpha1.DatadogAgentProfile{
				exampleProfileForLinux(),
				exampleProfileForWindows(),
				defaultProfile(),
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testLogger := zap.New(zap.UseDevMode(true))
			profilesToApply, profileAppliedPerNode, err := ProfilesToApply(test.profiles, test.nodes, testLogger)
			require.NoError(t, err)
			assert.ElementsMatch(t, test.expectedProfiles, profilesToApply)
			assert.Equal(t, test.expectedProfilesAppliedPerNode, profileAppliedPerNode)
		})
	}
}

func TestComponentOverrideFromProfile(t *testing.T) {
	overrideNameForLinuxProfile := "datadog-agent-with-profile-default-linux"
	overrideNameForExampleProfile := "datadog-agent-with-profile-default-example"

	tests := []struct {
		name             string
		profile          v1alpha1.DatadogAgentProfile
		expectedOverride v2alpha1.DatadogAgentComponentOverride
	}{
		{
			name:             "empty profile",
			profile:          v1alpha1.DatadogAgentProfile{},
			expectedOverride: v2alpha1.DatadogAgentComponentOverride{},
		},
		{
			name: "profile without affinity or config",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "example",
				},
			},
			expectedOverride: v2alpha1.DatadogAgentComponentOverride{
				Name: &overrideNameForExampleProfile,
				Labels: map[string]string{
					"agent.datadoghq.com/profile": fmt.Sprintf("%s-%s", "default", "example"),
				},
				Affinity: &v1.Affinity{
					PodAntiAffinity: &v1.PodAntiAffinity{
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
					},
				},
			},
		},
		{
			name:    "profile with affinity and config",
			profile: exampleProfileForLinux(),
			expectedOverride: v2alpha1.DatadogAgentComponentOverride{
				Name: &overrideNameForLinuxProfile,
				Affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "os",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"linux"},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: &v1.PodAntiAffinity{
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
					},
				},
				Containers: map[common.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					common.CoreAgentContainerName: {
						Resources: &v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceCPU: resource.MustParse("100m"),
							},
						},
					},
				},
				Labels: map[string]string{
					"agent.datadoghq.com/profile": fmt.Sprintf("%s-%s", "default", "linux"),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedOverride, ComponentOverrideFromProfile(&test.profile))
		})
	}
}

func TestDaemonSetName(t *testing.T) {
	tests := []struct {
		name                  string
		profileNamespacedName types.NamespacedName
		expectedDaemonSetName string
	}{
		{
			name: "default profile name",
			profileNamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "default",
			},
			expectedDaemonSetName: "",
		},
		{
			name: "non-default profile name",
			profileNamespacedName: types.NamespacedName{
				Namespace: "agent",
				Name:      "linux",
			},
			expectedDaemonSetName: "datadog-agent-with-profile-agent-linux",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedDaemonSetName, DaemonSetName(test.profileNamespacedName))
		})
	}
}

func exampleProfileForLinux() v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      "linux",
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []v1.NodeSelectorRequirement{
					{
						Key:      "os",
						Operator: v1.NodeSelectorOpIn,
						Values:   []string{"linux"},
					},
				},
			},
			Config: configWithCPURequestOverrideForCoreAgent("100m"),
		},
	}
}

func exampleProfileForWindows() v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      "windows",
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []v1.NodeSelectorRequirement{
					{
						Key:      "os",
						Operator: v1.NodeSelectorOpIn,
						Values:   []string{"windows"},
					},
				},
			},
			Config: configWithCPURequestOverrideForCoreAgent("200m"),
		},
	}
}

// configWithCPURequestOverrideForCoreAgent returns a config with a CPU request
// for the core agent container.
func configWithCPURequestOverrideForCoreAgent(cpuRequest string) *v1alpha1.Config {
	return &v1alpha1.Config{
		Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
			v1alpha1.NodeAgentComponentName: {
				Containers: map[common.AgentContainerName]*v1alpha1.Container{
					common.CoreAgentContainerName: {
						Resources: &v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceCPU: resource.MustParse(cpuRequest),
							},
						},
					},
				},
			},
		},
	}
}
