// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"fmt"
	"testing"
	"time"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/api/utils"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/constants"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const testNamespace = "default"

func TestApplyProfile(t *testing.T) {
	tests := []struct {
		name                           string
		profile                        v1alpha1.DatadogAgentProfile
		nodes                          []v1.Node
		datadogAgentInternalEnabled    bool
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
			datadogAgentInternalEnabled:    true,
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
			datadogAgentInternalEnabled: true,
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
			datadogAgentInternalEnabled: true,
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
			datadogAgentInternalEnabled: true,
			profileAppliedByNode:        map[string]types.NamespacedName{},
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
			datadogAgentInternalEnabled: true,
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
			datadogAgentInternalEnabled: true,
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
			datadogAgentInternalEnabled: true,
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
			datadogAgentInternalEnabled: true,
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
		{
			name:    "feature override when ddai disabled",
			profile: exampleFeatureOverrideProfile(),
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
			datadogAgentInternalEnabled:    false,
			profileAppliedByNode:           map[string]types.NamespacedName{},
			expectedProfilesAppliedPerNode: map[string]types.NamespacedName{},
			expectedErr:                    fmt.Errorf("the 'features' field is only supported when DatadogAgentInternal is enabled"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testLogger := zap.New(zap.UseDevMode(true))
			now := metav1.NewTime(time.Now())
			profileAppliedByNode, err := ApplyProfile(testLogger, &test.profile, test.nodes, test.profileAppliedByNode, now, 1, test.datadogAgentInternalEnabled)
			assert.Equal(t, test.expectedErr, err)
			assert.Equal(t, test.expectedProfilesAppliedPerNode, profileAppliedByNode)
		})
	}
}

func TestOverrideFromProfile(t *testing.T) {
	overrideNameForLinuxProfile := "datadog-agent-with-profile-default-linux"
	overrideNameForExampleProfile := "datadog-agent-with-profile-default-example"
	overrideNameForAllContainersProfile := "datadog-agent-with-profile-default-all-containers"

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
					PodAntiAffinity: profilePodAntiAffinity(),
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
										{
											Key:      constants.ProfileLabelKey,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"linux"},
										},
									},
								},
							},
						},
					},
					PodAntiAffinity: profilePodAntiAffinity(),
				},
				PriorityClassName: apiutils.NewStringPointer("foo"),
				RuntimeClassName:  apiutils.NewStringPointer("bar"),
				UpdateStrategy: &apicommon.UpdateStrategy{
					Type: "RollingUpdate",
					RollingUpdate: &apicommon.RollingUpdate{
						MaxUnavailable: &intstr.IntOrString{
							IntVal: 10,
						},
					},
				},
				Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					apicommon.CoreAgentContainerName: {
						Resources: cpuRequestResource("100m"),
						Env: []corev1.EnvVar{
							{
								Name:  "foo",
								Value: "bar",
							},
						},
					},
				},
				Labels: map[string]string{
					"agent.datadoghq.com/datadogagentprofile": "linux",
					"foo": "bar",
				},
			},
		},
		{
			name:    "default profile, no overrides applied",
			profile: exampleDefaultProfile(),
			expectedOverride: v2alpha1.DatadogAgentComponentOverride{
				Name: apiutils.NewStringPointer(""),
				Affinity: &v1.Affinity{
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
					PodAntiAffinity: profilePodAntiAffinity(),
				},
			},
		},
		{
			name:    "all containers, cpu request override",
			profile: fixedCpuRequestAllContainersProfile(),
			expectedOverride: v2alpha1.DatadogAgentComponentOverride{
				Name: &overrideNameForAllContainersProfile,
				Labels: map[string]string{
					"agent.datadoghq.com/datadogagentprofile": "all-containers",
				},
				Affinity: &v1.Affinity{
					PodAntiAffinity: profilePodAntiAffinity(),
				},
				Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					apicommon.CoreAgentContainerName: {
						Resources: cpuRequestResource("100m"),
					},
					apicommon.ProcessAgentContainerName: {
						Resources: cpuRequestResource("100m"),
					},
					apicommon.TraceAgentContainerName: {
						Resources: cpuRequestResource("100m"),
					},
					apicommon.SecurityAgentContainerName: {
						Resources: cpuRequestResource("100m"),
					},
					apicommon.SystemProbeContainerName: {
						Resources: cpuRequestResource("100m"),
					},
					apicommon.OtelAgent: {
						Resources: cpuRequestResource("100m"),
					},
					apicommon.AgentDataPlaneContainerName: {
						Resources: cpuRequestResource("100m"),
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedOverride, OverrideFromProfile(&test.profile, false))
		})
	}
}

func TestDaemonSetName(t *testing.T) {
	tests := []struct {
		name                  string
		profileNamespacedName types.NamespacedName
		useV3Metadata         bool
		expectedDaemonSetName string
	}{
		{
			name: "default profile name",
			profileNamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "default",
			},
			useV3Metadata:         false,
			expectedDaemonSetName: "",
		},
		{
			name: "non-default profile name",
			profileNamespacedName: types.NamespacedName{
				Namespace: "agent",
				Name:      "linux",
			},
			useV3Metadata:         false,
			expectedDaemonSetName: "datadog-agent-with-profile-agent-linux",
		},
		{
			name: "non-default profile name, v3 metadata",
			profileNamespacedName: types.NamespacedName{
				Namespace: "agent",
				Name:      "linux",
			},
			useV3Metadata:         true,
			expectedDaemonSetName: "linux-agent",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedDaemonSetName, DaemonSetName(test.profileNamespacedName, test.useV3Metadata))
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
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {},
						},
					},
				},
			},
			expectedLabels: map[string]string{
				constants.ProfileLabelKey: "foo",
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
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								Labels: map[string]string{
									"foo": "bar",
								},
							},
						},
					},
				},
			},
			expectedLabels: map[string]string{
				constants.ProfileLabelKey: "foo",
				"foo":                     "bar",
			},
		},
		{
			// constants.ProfileLabelKey should not be overriden by a user-created profile
			name: "profile with label overriding constants.ProfileLabelKey",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "foo",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								Labels: map[string]string{
									constants.ProfileLabelKey: "bar",
								},
							},
						},
					},
				},
			},
			expectedLabels: map[string]string{
				constants.ProfileLabelKey: "foo",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedLabels, labelsOverride(&test.profile))
		})
	}
}

func Test_canLabel(t *testing.T) {
	tests := []struct {
		name           string
		createStrategy *v1alpha1.CreateStrategy
		expected       bool
	}{
		{
			name:           "nil create strategy",
			createStrategy: nil,
			expected:       false,
		},
		{
			name:           "empty create strategy",
			createStrategy: &v1alpha1.CreateStrategy{},
			expected:       false,
		},
		{
			name: "completed create strategy status",
			createStrategy: &v1alpha1.CreateStrategy{
				Status: v1alpha1.CompletedStatus,
			},
			expected: true,
		},
		{
			name: "in progress create strategy status",
			createStrategy: &v1alpha1.CreateStrategy{
				Status: v1alpha1.InProgressStatus,
			},
			expected: true,
		},
		{
			name: "waiting create strategy status",
			createStrategy: &v1alpha1.CreateStrategy{
				Status: v1alpha1.WaitingStatus,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testLogger := zap.New(zap.UseDevMode(true))
			actual := canLabel(testLogger, tt.createStrategy)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
func Test_getCreateStrategyStatus(t *testing.T) {
	now := metav1.NewTime(time.Now())
	oneSecBefore := metav1.NewTime(now.Add(time.Duration(-1) * time.Second))
	tests := []struct {
		name              string
		status            *v1alpha1.CreateStrategy
		nodesNeedingLabel int
		expectedStatus    v1alpha1.CreateStrategyStatus
	}{
		{
			name:              "nil status",
			status:            nil,
			nodesNeedingLabel: 0,
			expectedStatus:    v1alpha1.WaitingStatus,
		},
		{
			name:              "empty status",
			status:            &v1alpha1.CreateStrategy{},
			nodesNeedingLabel: 1,
			expectedStatus:    "",
		},
		{
			name: "non-empty status, no nodes need labeling",
			status: &v1alpha1.CreateStrategy{
				Status: v1alpha1.InProgressStatus,
			},
			nodesNeedingLabel: 0,
			expectedStatus:    v1alpha1.CompletedStatus,
		},
		{
			name: "non-empty status, waiting",
			status: &v1alpha1.CreateStrategy{
				Status:         v1alpha1.WaitingStatus,
				LastTransition: &oneSecBefore,
			},
			nodesNeedingLabel: 1,
			expectedStatus:    v1alpha1.WaitingStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := getCreateStrategyStatus(tt.status, tt.nodesNeedingLabel)
			assert.Equal(t, tt.expectedStatus, status)
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
			Config: configWithAllOverrides("100m"),
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
			Config: configWithAllOverrides("200m"),
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
			Config: configWithAllOverrides("200m"),
		},
	}
}

func exampleFeatureOverrideProfile() v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      "feature",
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
			Config: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: utils.NewBoolPointer(true),
					},
				},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
		},
	}
}

func exampleDefaultProfile() v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "",
			Name:      "default",
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			Config: configWithAllOverrides("200m"),
		},
	}
}

func fixedCpuRequestAllContainersProfile() v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      "all-containers",
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			Config: &v2alpha1.DatadogAgentSpec{
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
							apicommon.CoreAgentContainerName: {
								Resources: cpuRequestResource("100m"),
							},
							apicommon.ProcessAgentContainerName: {
								Resources: cpuRequestResource("100m"),
							},
							apicommon.TraceAgentContainerName: {
								Resources: cpuRequestResource("100m"),
							},
							apicommon.SecurityAgentContainerName: {
								Resources: cpuRequestResource("100m"),
							},
							apicommon.SystemProbeContainerName: {
								Resources: cpuRequestResource("100m"),
							},
							apicommon.OtelAgent: {
								Resources: cpuRequestResource("100m"),
							},
							apicommon.AgentDataPlaneContainerName: {
								Resources: cpuRequestResource("100m"),
							},
						},
					},
				},
			},
		},
	}
}

// configWithAllOverrides returns a config with all available overrides for the
// core agent container.
func configWithAllOverrides(cpuRequest string) *v2alpha1.DatadogAgentSpec {
	return &v2alpha1.DatadogAgentSpec{
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {
				PriorityClassName: apiutils.NewStringPointer("foo"),
				RuntimeClassName:  apiutils.NewStringPointer("bar"),
				UpdateStrategy: &apicommon.UpdateStrategy{
					Type: "RollingUpdate",
					RollingUpdate: &apicommon.RollingUpdate{
						MaxUnavailable: &intstr.IntOrString{
							IntVal: 10,
						},
					},
				},
				Labels: map[string]string{
					"foo": "bar",
				},
				Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					apicommon.CoreAgentContainerName: {
						Env: []corev1.EnvVar{
							{
								Name:  "foo",
								Value: "bar",
							},
						},
						Resources: cpuRequestResource(cpuRequest),
					},
				},
			},
		},
	}
}

func cpuRequestResource(cpuRequest string) *v1.ResourceRequirements {
	return &v1.ResourceRequirements{
		Requests: v1.ResourceList{
			v1.ResourceCPU: resource.MustParse(cpuRequest),
		},
	}
}

func profilePodAntiAffinity() *v1.PodAntiAffinity {
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

func TestGetMaxUnavailable(t *testing.T) {
	tests := []struct {
		name                   string
		dda                    *v2alpha1.DatadogAgent
		profile                *v1alpha1.DatadogAgentProfile
		edsOptions             *agent.ExtendedDaemonsetOptions
		expectedMaxUnavailable int
	}{
		{
			name:                   "empty dda, empty profile",
			dda:                    &v2alpha1.DatadogAgent{},
			profile:                &v1alpha1.DatadogAgentProfile{},
			expectedMaxUnavailable: 1,
		},
		{
			name: "non-empty dda, empty profile, int value",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							UpdateStrategy: &apicommon.UpdateStrategy{
								Type: "RollingUpdate",
								RollingUpdate: &apicommon.RollingUpdate{
									MaxUnavailable: &intstr.IntOrString{
										IntVal: 5,
									},
								},
							},
						},
					},
				},
			},
			profile:                &v1alpha1.DatadogAgentProfile{},
			expectedMaxUnavailable: 5,
		},
		{
			name: "non-empty dda, empty profile, empty max unavailable",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							UpdateStrategy: &apicommon.UpdateStrategy{
								Type: "RollingUpdate",
								RollingUpdate: &apicommon.RollingUpdate{
									MaxUnavailable: &intstr.IntOrString{},
								},
							},
						},
					},
				},
			},
			profile:                &v1alpha1.DatadogAgentProfile{},
			expectedMaxUnavailable: 0,
		},
		{
			name: "non-empty dda, empty profile, malformed string value",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							UpdateStrategy: &apicommon.UpdateStrategy{
								Type: "RollingUpdate",
								RollingUpdate: &apicommon.RollingUpdate{
									MaxUnavailable: &intstr.IntOrString{
										Type:   intstr.String,
										StrVal: "10",
									},
								},
							},
						},
					},
				},
			},
			profile:                &v1alpha1.DatadogAgentProfile{},
			expectedMaxUnavailable: 1,
		},
		{
			name: "non-empty dda, non-empty profile, string value",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							UpdateStrategy: &apicommon.UpdateStrategy{
								Type: "RollingUpdate",
								RollingUpdate: &apicommon.RollingUpdate{
									MaxUnavailable: &intstr.IntOrString{
										IntVal: 5,
									},
								},
							},
						},
					},
				},
			},
			profile: &v1alpha1.DatadogAgentProfile{
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								UpdateStrategy: &apicommon.UpdateStrategy{
									Type: "RollingUpdate",
									RollingUpdate: &apicommon.RollingUpdate{
										MaxUnavailable: &intstr.IntOrString{
											Type:   intstr.String,
											StrVal: "15%",
										},
									},
								},
							},
						},
					},
				},
			},
			edsOptions: &agent.ExtendedDaemonsetOptions{
				MaxPodSchedulerFailure: "5",
			},
			expectedMaxUnavailable: 15,
		},
		{
			name: "empty dda, empty profile, non-empty edsOptions, string value",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Name: apiutils.NewStringPointer("test"),
						},
					},
				},
			},
			profile: &v1alpha1.DatadogAgentProfile{
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								PriorityClassName: apiutils.NewStringPointer("test"),
							},
						},
					},
				},
			},
			edsOptions: &agent.ExtendedDaemonsetOptions{
				MaxPodUnavailable: "50%",
			},
			expectedMaxUnavailable: 50,
		},
		{
			name: "non-empty dda, non-empty profile with empty updatestrategy, string value",
			dda: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							UpdateStrategy: &apicommon.UpdateStrategy{
								Type: "RollingUpdate",
								RollingUpdate: &apicommon.RollingUpdate{
									MaxUnavailable: &intstr.IntOrString{
										Type:   intstr.String,
										StrVal: "20%",
									},
								},
							},
						},
					},
				},
			},
			profile: &v1alpha1.DatadogAgentProfile{
				Spec: v1alpha1.DatadogAgentProfileSpec{
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								PriorityClassName: apiutils.NewStringPointer("test"),
							},
						},
					},
				},
			},
			expectedMaxUnavailable: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testLogger := zap.New(zap.UseDevMode(true))
			actualMaxUnavailable := GetMaxUnavailable(testLogger, &tt.dda.Spec, tt.profile, 100, tt.edsOptions)
			assert.Equal(t, tt.expectedMaxUnavailable, actualMaxUnavailable)
		})
	}
}
