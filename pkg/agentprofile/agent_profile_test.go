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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
)

const testNamespace = "default"

func TestProfileToApply(t *testing.T) {
	tests := []struct {
		name                           string
		profile                        v1alpha1.DatadogAgentProfile
		nodes                          []v1.Node
		profileAppliedByNode           map[string]types.NamespacedName
		expectedProfilesAppliedPerNode map[string]types.NamespacedName
		expectedErr                    error
	}{
		{
			name:    "empty profile, empty profileAppliedByNode",
			profile: v1alpha1.DatadogAgentProfile{},
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
			profileAppliedByNode:           map[string]types.NamespacedName{},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{},
			expectedErr:                    fmt.Errorf("Profile name cannot be empty"),
		},
		{
			name:    "empty profile, non-empty profileAppliedByNode",
			profile: v1alpha1.DatadogAgentProfile{},
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
			profileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			},
			expectedErr: fmt.Errorf("Profile name cannot be empty"),
		},
		{
			name:    "empty profile, , non-empty profileAppliedByNode, no nodes",
			profile: v1alpha1.DatadogAgentProfile{},
			nodes:   []v1.Node{},
			profileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			},
			expectedErr: fmt.Errorf("Profile name cannot be empty"),
		},
		{
			name:    "non-conflicting profile, empty profileAppliedByNode",
			profile: exampleProfileForLinux(),
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
			profileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			},
			expectedErr: nil,
		},
		{
			name:    "non-conflicting profile, non-empty profileAppliedByNode",
			profile: exampleProfileForLinux(),
			nodes: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"os": "linux",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"b": "1",
						},
					},
				},
			},
			profileAppliedByNode: map[string]types.NamespacedName{
				"node2": {
					Namespace: testNamespace,
					Name:      "windows",
				},
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
				"node2": {
					Namespace: testNamespace,
					Name:      "windows",
				},
			},
			expectedErr: nil,
		},
		{
			name:    "non-conflicting profile, non-empty profileAppliedByNode, no nodes",
			profile: exampleProfileForLinux(),
			nodes:   []v1.Node{},
			profileAppliedByNode: map[string]types.NamespacedName{
				"node2": {
					Namespace: testNamespace,
					Name:      "windows",
				},
			},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node2": {
					Namespace: testNamespace,
					Name:      "windows",
				},
			},
			expectedErr: nil,
		},
		{
			name:    "conflicting profile",
			profile: exampleProfileForLinux(),
			nodes: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"os": "linux",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"os": "windows",
						},
					},
				},
			},
			profileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			}, expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			},
			expectedErr: fmt.Errorf("conflict with existing profile"),
		},
		{
			name:    "invalid profile",
			profile: exampleInvalidProfile(),
			nodes: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"os": "linux",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"os": "windows",
						},
					},
				},
			},
			profileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			}, expectedProfilesAppliedPerNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "linux",
				},
			},
			expectedErr: fmt.Errorf("profileAffinity must be defined"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testLogger := zap.New(zap.UseDevMode(true))
			now := metav1.NewTime(time.Now())
			profileAppliedByNode, err := ProfileToApply(testLogger, &test.profile, test.nodes, test.profileAppliedByNode, now)
			assert.Equal(t, test.expectedErr, err)
			assert.Equal(t, test.expectedProfilesAppliedPerNode, profileAppliedByNode)
		})
	}
}

func TestOverrideFromProfile(t *testing.T) {
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
					"agent.datadoghq.com/datadogagentprofile": "example",
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
					"agent.datadoghq.com/datadogagentprofile": "linux",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedOverride, OverrideFromProfile(&test.profile))
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

func TestPriorityClassNameOverride(t *testing.T) {
	tests := []struct {
		name                  string
		profile               v1alpha1.DatadogAgentProfile
		expectedpriorityClass *string
	}{
		{
			name:                  "empty profile",
			profile:               v1alpha1.DatadogAgentProfile{},
			expectedpriorityClass: nil,
		},
		{
			name: "profile with no priority class set",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "foo",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {
								Containers: map[common.AgentContainerName]*v1alpha1.Container{},
							},
						},
					},
				},
			},
			expectedpriorityClass: nil,
		},
		{
			name: "profile with empty priority class set",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "foo",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {
								Containers:        map[common.AgentContainerName]*v1alpha1.Container{},
								PriorityClassName: apiutils.NewStringPointer(""),
							},
						},
					},
				},
			},
			expectedpriorityClass: apiutils.NewStringPointer(""),
		},
		{
			name: "profile with priority class set",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "foo",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {
								Containers:        map[common.AgentContainerName]*v1alpha1.Container{},
								PriorityClassName: apiutils.NewStringPointer("bar"),
							},
						},
					},
				},
			},
			expectedpriorityClass: apiutils.NewStringPointer("bar"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedpriorityClass, priorityClassNameOverride(&test.profile))
		})
	}
}
func Test_labelsOverride(t *testing.T) {
	tests := []struct {
		name           string
		profile        v1alpha1.DatadogAgentProfile
		expectedLabels map[string]string
	}{
		{
			name: "default profile",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: defaultProfileName,
				},
			},
			expectedLabels: nil,
		},
		{
			name: "profile with no label overrides",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "foo",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {},
						},
					},
				},
			},
			expectedLabels: map[string]string{
				ProfileLabelKey: "foo",
			},
		},
		{
			name: "profile with label overrides",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "foo",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {
								Labels: map[string]string{
									"foo": "bar",
								},
							},
						},
					},
				},
			},
			expectedLabels: map[string]string{
				ProfileLabelKey: "foo",
				"foo":           "bar",
			},
		},
		{
			// ProfileLabelKey should not be overriden by a user-created profile
			name: "profile with label overriding ProfileLabelKey",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "foo",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v1alpha1.Config{
						Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
							v1alpha1.NodeAgentComponentName: {
								Labels: map[string]string{
									ProfileLabelKey: "bar",
								},
							},
						},
					},
				},
			},
			expectedLabels: map[string]string{
				ProfileLabelKey: "foo",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedLabels, labelsOverride(&test.profile))
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

func exampleInvalidProfile() v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      "windows",
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			// missing ProfileAffinity
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

func Test_validateProfileName(t *testing.T) {
	tests := []struct {
		name          string
		profileName   string
		expectedError error
	}{
		{
			name:          "empty profile name",
			profileName:   "",
			expectedError: fmt.Errorf("Profile name cannot be empty"),
		},
		{
			name:          "valid profile name",
			profileName:   "foo",
			expectedError: nil,
		},
		{
			name:          "profile name too long",
			profileName:   "foo123456789012345678901234567890123456789012345678901234567890bar",
			expectedError: fmt.Errorf("Profile name must be no more than 63 characters"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedError, validateProfileName(test.profileName))
		})
	}
}
