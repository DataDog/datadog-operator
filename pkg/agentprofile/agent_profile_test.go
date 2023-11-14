// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
)

func TestProfilesToApply(t *testing.T) {
	tests := []struct {
		name             string
		profiles         []v1alpha1.DatadogAgentProfile
		expectedProfiles []v1alpha1.DatadogAgentProfile
	}{
		{
			name:     "no profiles",
			profiles: []v1alpha1.DatadogAgentProfile{},
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
		},
		{
			name: "several non-conflicting profiles",
			profiles: []v1alpha1.DatadogAgentProfile{
				exampleProfileForLinux(),
				exampleProfileForWindows(),
			},
			expectedProfiles: []v1alpha1.DatadogAgentProfile{
				exampleProfileForLinux(),
				exampleProfileForWindows(),
				{ // Default that does not apply to linux or windows
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []v1.NodeSelectorRequirement{
								{ // Opposite of example Linux profile
									Key:      "os",
									Operator: v1.NodeSelectorOpNotIn,
									Values:   []string{"linux"},
								},
								{ // Opposite of example Windows profile
									Key:      "os",
									Operator: v1.NodeSelectorOpNotIn,
									Values:   []string{"windows"},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.ElementsMatch(t, test.expectedProfiles, ProfilesToApply(test.profiles))
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
			name: "profile without affinity or config",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "example",
				},
			},
			expectedOverride: v2alpha1.DatadogAgentComponentOverride{
				Name: &overrideNameForExampleProfile,
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
				Namespace: "agent",
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
			Namespace: "default",
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
			Config: &v1alpha1.Config{
				Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
					v1alpha1.NodeAgentComponentName: {
						Containers: map[common.AgentContainerName]*v1alpha1.Container{
							common.CoreAgentContainerName: {
								Resources: &v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU: resource.MustParse("100m"),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func exampleProfileForWindows() v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
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
			Config: &v1alpha1.Config{
				Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
					v1alpha1.NodeAgentComponentName: {
						Containers: map[common.AgentContainerName]*v1alpha1.Container{
							common.CoreAgentContainerName: {
								Resources: &v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU: resource.MustParse("200m"),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
