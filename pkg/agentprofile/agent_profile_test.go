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
	"github.com/go-logr/logr"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
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

func TestSortProfiles(t *testing.T) {
	time1 := metav1.Now()
	time2 := metav1.NewTime(time1.Add(time.Hour))
	time3 := metav1.NewTime(time1.Add(2 * time.Hour))

	tests := []struct {
		name     string
		profiles []v1alpha1.DatadogAgentProfile
		expected []v1alpha1.DatadogAgentProfile
	}{
		{
			name:     "nil",
			profiles: nil,
			expected: []v1alpha1.DatadogAgentProfile{},
		},
		{
			name:     "empty slice",
			profiles: []v1alpha1.DatadogAgentProfile{},
			expected: []v1alpha1.DatadogAgentProfile{},
		},
		{
			name: "single profile",
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "foo",
						CreationTimestamp: time1,
					},
				},
			},
			expected: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "foo",
						CreationTimestamp: time1,
					},
				},
			},
		},
		{
			name: "profiles with different creation timestamps",
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "newest",
						CreationTimestamp: time3,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "oldest",
						CreationTimestamp: time1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "middle",
						CreationTimestamp: time2,
					},
				},
			},
			expected: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "oldest",
						CreationTimestamp: time1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "middle",
						CreationTimestamp: time2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "newest",
						CreationTimestamp: time3,
					},
				},
			},
		},
		{
			name: "profiles with same creation timestamp, different names",
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "aaa",
						CreationTimestamp: time1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "ccc",
						CreationTimestamp: time1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "bbb",
						CreationTimestamp: time1,
					},
				},
			},
			expected: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "aaa",
						CreationTimestamp: time1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "bbb",
						CreationTimestamp: time1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "ccc",
						CreationTimestamp: time1,
					},
				},
			},
		},
		{
			name: "mix of same and different timestamps",
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "profile-z",
						CreationTimestamp: time2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "profile-a",
						CreationTimestamp: time2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "oldest",
						CreationTimestamp: time1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "newest",
						CreationTimestamp: time3,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "profile-b",
						CreationTimestamp: time2,
					},
				},
			},
			expected: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "oldest",
						CreationTimestamp: time1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "profile-a",
						CreationTimestamp: time2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "profile-b",
						CreationTimestamp: time2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "profile-z",
						CreationTimestamp: time2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "newest",
						CreationTimestamp: time3,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SortProfiles(tt.profiles)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// Helper function to create a labels.Requirement for testing
func newRequirement(key string, op selection.Operator, values []string) *labels.Requirement {
	req, err := labels.NewRequirement(key, op, values)
	if err != nil {
		panic(fmt.Sprintf("failed to create requirement: %v", err))
	}
	return req
}

func TestParseProfileRequirements(t *testing.T) {
	tests := []struct {
		name                 string
		profile              *v1alpha1.DatadogAgentProfile
		expectedRequirements []*labels.Requirement
		expectedError        error
	}{
		{
			name:                 "nil profile",
			profile:              nil,
			expectedRequirements: nil,
			expectedError:        nil,
		},
		{
			name:                 "empty profile",
			profile:              &v1alpha1.DatadogAgentProfile{},
			expectedRequirements: nil,
			expectedError:        nil,
		},
		{
			name: "profile with node affinity",
			profile: &v1alpha1.DatadogAgentProfile{
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
							{
								Key:      "foo",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"bar"},
							},
						},
					},
				},
			},
			expectedRequirements: []*labels.Requirement{
				newRequirement("foo", selection.In, []string{"bar"}),
			},
			expectedError: nil,
		},
		{
			name: "profile with invalid node affinity",
			profile: &v1alpha1.DatadogAgentProfile{
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
							{
								Key:      "foo",
								Operator: corev1.NodeSelectorOpExists,
								Values:   []string{"bar"},
							},
						},
					},
				},
			},
			expectedRequirements: nil,
			expectedError:        fmt.Errorf("values set must be empty for exists and does not exist"),
		},
		{
			name: "profile with multiple invalid node affinity requirements (only first error is returned)",
			profile: &v1alpha1.DatadogAgentProfile{
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
							{
								Key:      "foo",
								Operator: corev1.NodeSelectorOpExists,
								Values:   []string{"bar"}, // values set must be empty for exists and does not exist
							},
							{
								Key:      "", // name part must be non-empty
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"value"},
							},
						},
					},
				},
			},
			expectedRequirements: nil,
			expectedError:        fmt.Errorf("values set must be empty for exists and does not exist"),
		},
		{
			name: "profile with empty node affinity slice",
			profile: &v1alpha1.DatadogAgentProfile{
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{},
					},
				},
			},
			expectedRequirements: []*labels.Requirement{},
			expectedError:        nil,
		},
		{
			name: "profile with multiple node affinity",
			profile: &v1alpha1.DatadogAgentProfile{
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
							{
								Key:      "foo",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"bar"},
							},
							{
								Key:      "foo2",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"bar2"},
							},
						},
					},
				},
			},
			expectedRequirements: []*labels.Requirement{
				newRequirement("foo", selection.In, []string{"bar"}),
				newRequirement("foo2", selection.In, []string{"bar2"}),
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requirements, err := parseProfileRequirements(tt.profile)
			assert.Equal(t, tt.expectedRequirements, requirements)
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestApplyProfileToNodes(t *testing.T) {
	tests := []struct {
		name                    string
		createStrategyEnabled   bool
		profileMeta             metav1.ObjectMeta
		profileRequirementsSpec []struct {
			key      string
			operator selection.Operator
			values   []string
		}
		nodes                        []v1.Node
		existingProfileAppliedByNode map[string]types.NamespacedName
		expectedProfileAppliedByNode map[string]types.NamespacedName
		expectedNodesNeedingLabel    []string
		expectedNodesAlreadyLabeled  int32
		expectedError                error
	}{
		{
			name: "profile applied to single node",
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
			},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{"os": "linux"}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile"},
			},
			expectedError: nil,
		},
		{
			name: "profile applied to multiple nodes",
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
			},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{"os": "linux"}),
				createNodeWithLabels("node2", map[string]string{"os": "windows"}),
				createNodeWithLabels("node3", map[string]string{"os": "linux"}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile"},
				"node2": {Namespace: "", Name: "default"}, // doesn't match, keeps default
				"node3": {Namespace: testNamespace, Name: "profile"},
			},
			expectedError: nil,
		},
		{
			name: "conflicting profile",
			profileMeta: metav1.ObjectMeta{
				Name:      "new-profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
			},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{"os": "linux"}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "existing-profile"},
			},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "existing-profile"},
			},
			expectedError: fmt.Errorf("profile new-profile conflicts with existing profile: default/existing-profile"),
		},
		{
			name: "no matching nodes",
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
			},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{"os": "windows"}),
				createNodeWithLabels("node2", map[string]string{"os": "darwin"}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: "", Name: "default"}, // no match, keeps default
				"node2": {Namespace: "", Name: "default"}, // no match, keeps default
			},
			expectedError: nil,
		},
		{
			name: "empty nodes list",
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
			},
			nodes:                        []v1.Node{},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedError:                nil,
		},
		{
			name: "nil profile requirements (matches all nodes)",
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: nil,
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{"os": "linux"}),
				createNodeWithLabels("node2", map[string]string{"os": "windows"}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile"},
				"node2": {Namespace: testNamespace, Name: "profile"},
			},
			expectedError: nil,
		},
		{
			name: "empty profile requirements (matches all nodes)",
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{"os": "linux"}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile"},
			},
			expectedError: nil,
		},
		{
			name: "multiple nodes, some conflicting profiles",
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
			},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{"os": "linux"}),
				createNodeWithLabels("node2", map[string]string{"os": "linux"}),
				createNodeWithLabels("node3", map[string]string{"os": "windows"}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "existing-profile"},
			},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "existing-profile"}, // conflict, keeps existing
				"node2": {Namespace: "", Name: "default"},                     // gets default (pre-populated)
				"node3": {Namespace: "", Name: "default"},                     // gets default (pre-populated)
			},
			expectedError: fmt.Errorf("profile profile conflicts with existing profile: default/existing-profile"),
		},
		{
			name: "multiple requirements",
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
				{key: "tier", operator: selection.In, values: []string{"production"}},
				{key: "zone", operator: selection.Exists, values: nil},
			},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{"os": "linux", "tier": "production", "zone": "us-east-1"}),
				createNodeWithLabels("node2", map[string]string{"os": "linux", "tier": "staging"}),
				createNodeWithLabels("node3", map[string]string{"os": "windows", "tier": "production", "zone": "us-east-1"}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile"},
				"node2": {Namespace: "", Name: "default"}, // doesn't match, keeps default
				"node3": {Namespace: "", Name: "default"}, // doesn't match, keeps default
			},
			expectedError: nil,
		},
		// Create strategy test cases
		{
			name:                  "create strategy enabled, nodes need labels",
			createStrategyEnabled: true,
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
			},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{"os": "linux"}),
				createNodeWithLabels("node2", map[string]string{"os": "linux"}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile"},
				"node2": {Namespace: testNamespace, Name: "profile"},
			},
			expectedNodesNeedingLabel:   []string{"node1", "node2"},
			expectedNodesAlreadyLabeled: 0,
			expectedError:               nil,
		},
		{
			name:                  "create strategy enabled, nodes already labeled",
			createStrategyEnabled: true,
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
			},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{
					"os":                      "linux",
					constants.ProfileLabelKey: "profile",
				}),
				createNodeWithLabels("node2", map[string]string{
					"os":                      "linux",
					constants.ProfileLabelKey: "profile",
				}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile"},
				"node2": {Namespace: testNamespace, Name: "profile"},
			},
			expectedNodesNeedingLabel:   []string{},
			expectedNodesAlreadyLabeled: 2,
			expectedError:               nil,
		},
		{
			name:                  "create strategyenabled, some nodes need labels, some already labeled",
			createStrategyEnabled: true,
			profileMeta: metav1.ObjectMeta{
				Name:      "profile",
				Namespace: testNamespace,
			},
			profileRequirementsSpec: []struct {
				key      string
				operator selection.Operator
				values   []string
			}{
				{key: "os", operator: selection.In, values: []string{"linux"}},
			},
			nodes: []v1.Node{
				createNodeWithLabels("node1", map[string]string{
					"os":                      "linux",
					constants.ProfileLabelKey: "profile",
				}),
				createNodeWithLabels("node2", map[string]string{"os": "linux"}),
				createNodeWithLabels("node3", map[string]string{
					"os":                      "linux",
					constants.ProfileLabelKey: "wrong-profile",
				}),
			},
			existingProfileAppliedByNode: map[string]types.NamespacedName{},
			expectedProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile"},
				"node2": {Namespace: testNamespace, Name: "profile"},
				"node3": {Namespace: testNamespace, Name: "profile"},
			},
			expectedNodesNeedingLabel:   []string{"node2", "node3"},
			expectedNodesAlreadyLabeled: 1,
			expectedError:               nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.createStrategyEnabled {
				t.Setenv(apicommon.CreateStrategyEnabled, "true")
			}

			profileRequirements, err := createRequirements(tt.profileRequirementsSpec)
			assert.NoError(t, err)

			// Set existingProfileAppliedByNode to default profile (done in reconcileProfiles)
			if tt.existingProfileAppliedByNode == nil {
				tt.existingProfileAppliedByNode = make(map[string]types.NamespacedName)
			}
			for _, node := range tt.nodes {
				if _, exists := tt.existingProfileAppliedByNode[node.Name]; !exists {
					tt.existingProfileAppliedByNode[node.Name] = types.NamespacedName{Namespace: "", Name: "default"}
				}
			}

			csInfo := make(map[types.NamespacedName]*CreateStrategyInfo)
			e := ApplyProfileToNodes(tt.profileMeta, profileRequirements, tt.nodes, tt.existingProfileAppliedByNode, csInfo)
			assert.Equal(t, tt.expectedError, e)
			assert.Equal(t, tt.expectedProfileAppliedByNode, tt.existingProfileAppliedByNode)

			if tt.createStrategyEnabled {
				profileNSName := types.NamespacedName{Namespace: tt.profileMeta.Namespace, Name: tt.profileMeta.Name}
				if len(tt.expectedNodesNeedingLabel) > 0 || tt.expectedNodesAlreadyLabeled > 0 {
					assert.NotNil(t, csInfo[profileNSName])
					assert.ElementsMatch(t, tt.expectedNodesNeedingLabel, csInfo[profileNSName].nodesNeedingLabel)
					assert.Equal(t, tt.expectedNodesAlreadyLabeled, csInfo[profileNSName].nodesAlreadyLabeled)
				}
			}
		})
	}
}

func TestApplyCreateStrategy(t *testing.T) {
	t.Setenv(apicommon.CreateStrategyEnabled, "true")

	logger := logr.Discard()

	tests := []struct {
		name                         string
		profilesByNode               map[string]types.NamespacedName
		csInfo                       map[types.NamespacedName]*CreateStrategyInfo
		appliedProfiles              []*v1alpha1.DatadogAgentProfile
		ddaEDSMaxUnavailable         intstr.IntOrString
		numNodes                     int
		numberReady                  int32
		expectedProfilesByNode       map[string]types.NamespacedName
		expectedNodesLabeled         map[types.NamespacedName]int32
		expectedCreateStrategyStatus map[types.NamespacedName]v1alpha1.CreateStrategyStatus
	}{
		{
			name: "nodes under limit - all kept",
			profilesByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile1"},
				"node2": {Namespace: testNamespace, Name: "profile1"},
			},
			csInfo: map[types.NamespacedName]*CreateStrategyInfo{
				{Namespace: testNamespace, Name: "profile1"}: {
					nodesNeedingLabel:   []string{"node1", "node2"},
					nodesAlreadyLabeled: 0,
				},
			},
			appliedProfiles: []*v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "profile1", Namespace: testNamespace},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						Config: &v2alpha1.DatadogAgentSpec{},
					},
					Status: v1alpha1.DatadogAgentProfileStatus{},
				},
			},
			ddaEDSMaxUnavailable: intstr.FromInt(3),
			numNodes:             5,
			numberReady:          0,
			expectedProfilesByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile1"},
				"node2": {Namespace: testNamespace, Name: "profile1"},
			},
			expectedNodesLabeled: map[types.NamespacedName]int32{
				{Namespace: testNamespace, Name: "profile1"}: 2,
			},
			expectedCreateStrategyStatus: map[types.NamespacedName]v1alpha1.CreateStrategyStatus{
				{Namespace: testNamespace, Name: "profile1"}: v1alpha1.InProgressStatus,
			},
		},
		{
			name: "nodes over limit - excess deleted",
			profilesByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile1"},
				"node2": {Namespace: testNamespace, Name: "profile1"},
				"node3": {Namespace: testNamespace, Name: "profile1"},
			},
			csInfo: map[types.NamespacedName]*CreateStrategyInfo{
				{Namespace: testNamespace, Name: "profile1"}: {
					nodesNeedingLabel:   []string{"node1", "node2", "node3"},
					nodesAlreadyLabeled: 0,
				},
			},
			appliedProfiles: []*v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "profile1", Namespace: testNamespace},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						Config: &v2alpha1.DatadogAgentSpec{},
					},
					Status: v1alpha1.DatadogAgentProfileStatus{},
				},
			},
			ddaEDSMaxUnavailable: intstr.FromInt(2),
			numNodes:             5,
			numberReady:          0,
			expectedProfilesByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile1"},
				"node2": {Namespace: testNamespace, Name: "profile1"},
				// node3 should be deleted
			},
			expectedNodesLabeled: map[types.NamespacedName]int32{
				{Namespace: testNamespace, Name: "profile1"}: 2,
			},
			expectedCreateStrategyStatus: map[types.NamespacedName]v1alpha1.CreateStrategyStatus{
				{Namespace: testNamespace, Name: "profile1"}: v1alpha1.InProgressStatus,
			},
		},
		{
			name: "already labeled nodes don't count against limit",
			profilesByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile1"},
				"node2": {Namespace: testNamespace, Name: "profile1"},
				"node3": {Namespace: testNamespace, Name: "profile1"},
			},
			csInfo: map[types.NamespacedName]*CreateStrategyInfo{
				{Namespace: testNamespace, Name: "profile1"}: {
					nodesNeedingLabel:   []string{"node3"},
					nodesAlreadyLabeled: 2, // node1 and node2 already have correct label
				},
			},
			appliedProfiles: []*v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "profile1", Namespace: testNamespace},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						Config: &v2alpha1.DatadogAgentSpec{},
					},
					Status: v1alpha1.DatadogAgentProfileStatus{},
				},
			},
			ddaEDSMaxUnavailable: intstr.FromInt(1),
			numNodes:             5,
			numberReady:          2, // node1 and node2 pods are ready
			expectedProfilesByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile1"},
				"node2": {Namespace: testNamespace, Name: "profile1"},
				"node3": {Namespace: testNamespace, Name: "profile1"},
			},
			expectedNodesLabeled: map[types.NamespacedName]int32{
				{Namespace: testNamespace, Name: "profile1"}: 3,
			},
			expectedCreateStrategyStatus: map[types.NamespacedName]v1alpha1.CreateStrategyStatus{
				{Namespace: testNamespace, Name: "profile1"}: v1alpha1.InProgressStatus,
			},
		},
		{
			name: "multiple profiles with different limits",
			profilesByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile1"},
				"node2": {Namespace: testNamespace, Name: "profile1"},
				"node3": {Namespace: testNamespace, Name: "profile2"},
				"node4": {Namespace: testNamespace, Name: "profile2"},
				"node5": {Namespace: testNamespace, Name: "profile2"},
			},
			csInfo: map[types.NamespacedName]*CreateStrategyInfo{
				{Namespace: testNamespace, Name: "profile1"}: {
					nodesNeedingLabel:   []string{"node1", "node2"},
					nodesAlreadyLabeled: 0,
				},
				{Namespace: testNamespace, Name: "profile2"}: {
					nodesNeedingLabel:   []string{"node3", "node4", "node5"},
					nodesAlreadyLabeled: 0,
				},
			},
			appliedProfiles: []*v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "profile1", Namespace: testNamespace},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						Config: &v2alpha1.DatadogAgentSpec{},
					},
					Status: v1alpha1.DatadogAgentProfileStatus{},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "profile2", Namespace: testNamespace},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						Config: &v2alpha1.DatadogAgentSpec{
							Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
								v2alpha1.NodeAgentComponentName: {
									UpdateStrategy: &apicommon.UpdateStrategy{
										RollingUpdate: &apicommon.RollingUpdate{
											MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
										},
									},
								},
							},
						},
					},
					Status: v1alpha1.DatadogAgentProfileStatus{},
				},
			},
			ddaEDSMaxUnavailable: intstr.FromInt(2),
			numNodes:             5,
			numberReady:          0,
			expectedProfilesByNode: map[string]types.NamespacedName{
				"node1": {Namespace: testNamespace, Name: "profile1"},
				"node2": {Namespace: testNamespace, Name: "profile1"},
				"node3": {Namespace: testNamespace, Name: "profile2"},
				// node4 and node5 deleted due to profile2's limit of 1
			},
			expectedNodesLabeled: map[types.NamespacedName]int32{
				{Namespace: testNamespace, Name: "profile1"}: 2,
				{Namespace: testNamespace, Name: "profile2"}: 1,
			},
			expectedCreateStrategyStatus: map[types.NamespacedName]v1alpha1.CreateStrategyStatus{
				{Namespace: testNamespace, Name: "profile1"}: v1alpha1.InProgressStatus,
				{Namespace: testNamespace, Name: "profile2"}: v1alpha1.InProgressStatus,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, profile := range tt.appliedProfiles {
				dsStatus := &appsv1.DaemonSetStatus{
					NumberReady: tt.numberReady,
				}
				profileNSName := types.NamespacedName{Namespace: profile.Namespace, Name: profile.Name}
				ApplyCreateStrategy(logger, tt.profilesByNode, tt.csInfo[profileNSName], profile, tt.ddaEDSMaxUnavailable, tt.numNodes, dsStatus)
			}

			assert.Equal(t, tt.expectedProfilesByNode, tt.profilesByNode)

			// status
			for _, profile := range tt.appliedProfiles {
				profileNSName := types.NamespacedName{Namespace: profile.Namespace, Name: profile.Name}
				if expectedCount, ok := tt.expectedNodesLabeled[profileNSName]; ok {
					assert.NotNil(t, profile.Status.CreateStrategy)
					assert.Equal(t, expectedCount, profile.Status.CreateStrategy.NodesLabeled)
					assert.Equal(t, tt.expectedCreateStrategyStatus[profileNSName], profile.Status.CreateStrategy.Status)
				}
			}
		})
	}
}

// createRequirements creates multiple label requirements with the given parameters
func createRequirements(requirements []struct {
	key      string
	operator selection.Operator
	values   []string
}) ([]*labels.Requirement, error) {
	result := make([]*labels.Requirement, len(requirements))
	for i, req := range requirements {
		labelReq, err := labels.NewRequirement(req.key, req.operator, req.values)
		if err != nil {
			return nil, fmt.Errorf("Failed to create requirement for key %s: %v", req.key, err)
		}
		result[i] = labelReq
	}
	return result, nil
}

// createNodeWithLabels creates a node with the given name and labels
func createNodeWithLabels(name string, nodeLabels map[string]string) v1.Node {
	return v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: nodeLabels,
		},
	}
}
