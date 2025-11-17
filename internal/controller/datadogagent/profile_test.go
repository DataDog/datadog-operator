package datadogagent

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
)

func Test_computeProfileMerge(t *testing.T) {
	sch := k8sruntime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)
	_ = v2alpha1.AddToScheme(sch)
	_ = corev1.AddToScheme(sch)
	_ = apiextensionsv1.AddToScheme(sch)
	ctx := context.Background()

	testCases := []struct {
		name    string
		ddai    v1alpha1.DatadogAgentInternal
		profile v1alpha1.DatadogAgentProfile
		want    v1alpha1.DatadogAgentInternal
	}{
		{
			name: "default profile",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Env: []corev1.EnvVar{
								{
									Name:  "EXISTING",
									Value: "value",
								},
							},
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: "",
							},
						},
					},
				},
			},
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			want: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Annotations: map[string]string{
						constants.MD5DDAIDeploymentAnnotationKey: "cf36f429dc3cdc72527e13ab7c602dec",
					},
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "agent.datadoghq.com/datadogagentprofile",
														Operator: corev1.NodeSelectorOpDoesNotExist,
													},
												},
											},
										},
									},
								},
								PodAntiAffinity: &corev1.PodAntiAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchExpressions: []metav1.LabelSelectorRequirement{
													{
														Key:      "agent.datadoghq.com/component",
														Operator: metav1.LabelSelectorOpIn,
														Values:   []string{"agent"},
													},
												},
											},
											TopologyKey: "kubernetes.io/hostname",
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "EXISTING",
									Value: "value",
								},
							},
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: "",
							},
						},
					},
				},
			},
		},
		{
			name: "user created profile",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Env: []corev1.EnvVar{
								{
									Name:  "EXISTING",
									Value: "value",
								},
							},
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: "",
							},
						},
					},
				},
			},
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-profile",
					Namespace: "bar",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
							{
								Key:      "test",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"foo"},
							},
						},
					},
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								Env: []corev1.EnvVar{
									{
										Name:  "EXISTING",
										Value: "newvalue",
									},
								},
							},
						},
					},
				},
			},
			want: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-profile",
					Namespace: "bar",
					Annotations: map[string]string{
						constants.MD5DDAIDeploymentAnnotationKey: "7540aac2cb9cbb8adc8666a70fc3e822",
					},
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Name: apiutils.NewStringPointer("foo-profile-agent"),
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "test",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"foo"},
													},
													{
														Key:      "agent.datadoghq.com/datadogagentprofile",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"foo-profile"},
													},
												},
											},
										},
									},
								},
								PodAntiAffinity: &corev1.PodAntiAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchExpressions: []metav1.LabelSelectorRequirement{
													{
														Key:      "agent.datadoghq.com/component",
														Operator: metav1.LabelSelectorOpIn,
														Values:   []string{"agent"},
													},
												},
											},
											TopologyKey: "kubernetes.io/hostname",
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "EXISTING",
									Value: "newvalue",
								},
							},
							Labels: map[string]string{
								constants.ProfileLabelKey:                    "foo-profile",
								constants.MD5AgentDeploymentProviderLabelKey: "",
							},
						},
						v2alpha1.ClusterAgentComponentName: {
							Disabled: apiutils.NewBoolPointer(true),
						},
						v2alpha1.ClusterChecksRunnerComponentName: {
							Disabled: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
		},
	}

	for _, tt := range testCases {
		// Load CRD from config folder
		crd, err := getDDAICRDFromConfig(sch)
		assert.NoError(t, err)

		fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(&tt.ddai, crd).Build()
		logger := logf.Log.WithName("Test_computeProfileMerge")
		eventBroadcaster := record.NewBroadcaster()
		recorder := eventBroadcaster.NewRecorder(sch, corev1.EventSource{Component: "Test_computeProfileMerge"})
		fieldManager, err := newFieldManager(fakeClient, sch, v1alpha1.GroupVersion.WithKind("DatadogAgentInternal"))
		assert.NoError(t, err)

		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:       fakeClient,
				log:          logger,
				scheme:       sch,
				recorder:     recorder,
				fieldManager: fieldManager,
			}

			crd := &apiextensionsv1.CustomResourceDefinition{}
			err := r.client.Get(ctx,
				types.NamespacedName{
					Name: "datadogagentinternals.datadoghq.com",
				},
				crd)
			assert.NoError(t, err)

			ddai, err := r.computeProfileMerge(&tt.ddai, &tt.profile)
			assert.NoError(t, err)
			assert.Equal(t, tt.want.Name, ddai.Name)
			assert.Equal(t, tt.want.Annotations[constants.MD5DDAIDeploymentAnnotationKey], ddai.Annotations[constants.MD5DDAIDeploymentAnnotationKey])
			assert.Equal(t, tt.want.Spec, ddai.Spec)
		})
	}
}

func Test_setProfileSpec(t *testing.T) {
	testCases := []struct {
		name    string
		ddai    v1alpha1.DatadogAgentInternal
		profile v1alpha1.DatadogAgentProfile
		want    v1alpha1.DatadogAgentInternal
	}{
		{
			name: "default profile",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: "",
							},
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "key",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"value"},
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
			},
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			want: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "key",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"value"},
													},
													{
														Key:      "agent.datadoghq.com/datadogagentprofile",
														Operator: corev1.NodeSelectorOpDoesNotExist,
													},
												},
											},
										},
									},
								},
								PodAntiAffinity: &corev1.PodAntiAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchExpressions: []metav1.LabelSelectorRequirement{
													{
														Key:      "agent.datadoghq.com/component",
														Operator: metav1.LabelSelectorOpIn,
														Values:   []string{"agent"},
													},
												},
											},
											TopologyKey: "kubernetes.io/hostname",
										},
									},
								},
							},
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: "",
							},
						},
					},
				},
			},
		},
		{
			name: "user created profile",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				// DDAI spec is overridden to create the profile DDAI
				// Therefore, the provider label will not be in the final profile DDAI spec
				// This config will be merged with a copy of the original DDAI
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: "",
							},
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "key",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"value"},
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
			},
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-profile",
					Namespace: "bar",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
							{
								Key:      "test",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"foo"},
							},
						},
					},
					Config: &v2alpha1.DatadogAgentSpec{
						Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
							v2alpha1.NodeAgentComponentName: {
								Env: []corev1.EnvVar{
									{
										Name:  "foo",
										Value: "bar",
									},
								},
							},
						},
					},
				},
			},
			want: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Name: apiutils.NewStringPointer("foo-profile-agent"),
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "key",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"value"},
													},
													{
														Key:      "test",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"foo"},
													},
													{
														Key:      "agent.datadoghq.com/datadogagentprofile",
														Operator: corev1.NodeSelectorOpIn,
														Values:   []string{"foo-profile"},
													},
												},
											},
										},
									},
								},
								PodAntiAffinity: &corev1.PodAntiAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchExpressions: []metav1.LabelSelectorRequirement{
													{
														Key:      "agent.datadoghq.com/component",
														Operator: metav1.LabelSelectorOpIn,
														Values:   []string{"agent"},
													},
												},
											},
											TopologyKey: "kubernetes.io/hostname",
										},
									},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "foo",
									Value: "bar",
								},
							},
							Labels: map[string]string{
								constants.ProfileLabelKey: "foo-profile",
							},
						},
						v2alpha1.ClusterAgentComponentName: {
							Disabled: apiutils.NewBoolPointer(true),
						},
						v2alpha1.ClusterChecksRunnerComponentName: {
							Disabled: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
		},
		{
			name: "nil override map and component",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{},
			},
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			want: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "agent.datadoghq.com/datadogagentprofile",
														Operator: corev1.NodeSelectorOpDoesNotExist,
													},
												},
											},
										},
									},
								},
								PodAntiAffinity: &corev1.PodAntiAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchExpressions: []metav1.LabelSelectorRequirement{
													{
														Key:      "agent.datadoghq.com/component",
														Operator: metav1.LabelSelectorOpIn,
														Values:   []string{"agent"},
													},
												},
											},
											TopologyKey: "kubernetes.io/hostname",
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

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			setProfileSpec(&tt.ddai, &tt.profile)
			assert.Equal(t, tt.want, tt.ddai)
		})
	}
}

func Test_setProfileDDAIMeta(t *testing.T) {
	testCases := []struct {
		name    string
		ddai    v1alpha1.DatadogAgentInternal
		profile v1alpha1.DatadogAgentProfile
		want    v1alpha1.DatadogAgentInternal
	}{
		{
			name: "default profile",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					ManagedFields: []metav1.ManagedFieldsEntry{
						{
							Manager: "datadog-operator",
						},
					},
				},
			},
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			want: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			},
		},
		{
			name: "user created profile",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					ManagedFields: []metav1.ManagedFieldsEntry{
						{
							Manager: "datadog-operator",
						},
					},
				},
			},
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			},
			want: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						constants.ProfileLabelKey: "foo",
					},
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			setProfileDDAIMeta(&tt.ddai, &tt.profile)
			assert.Equal(t, tt.want, tt.ddai)
		})
	}
}

func Test_setProfileNodeAgentOverride(t *testing.T) {
	testCases := []struct {
		name                                   string
		ddai                                   v1alpha1.DatadogAgentInternal
		profile                                v1alpha1.DatadogAgentProfile
		expectedNodeAgentComponentNameOverride *string
		expectedLabels                         map[string]string
	}{
		{
			name: "non-default profile should get DaemonSet name override",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-profile",
					Namespace: "default",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {},
					},
				},
			},
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-profile",
					Namespace: "default",
				},
			},
			expectedNodeAgentComponentNameOverride: apiutils.NewStringPointer("my-profile-agent"),
			expectedLabels: map[string]string{
				constants.ProfileLabelKey: "my-profile",
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			setProfileNodeAgentOverride(&tt.ddai, &tt.profile)

			// Check that the override exists
			override, ok := tt.ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
			assert.True(t, ok, "NodeAgent override should exist")

			// Check Node Agent component override name
			if !agentprofile.IsDefaultProfile(tt.profile.Namespace, tt.profile.Name) {
				assert.NotNil(t, override.Name, "Override Name should not be nil")
				assert.Equal(t, *tt.expectedNodeAgentComponentNameOverride, *override.Name, "Node Agent component override name should match expected")
			}

			// Check labels
			assert.Equal(t, tt.expectedLabels, override.Labels, "Labels should match expected")
		})
	}
}

func Test_reconcileProfile(t *testing.T) {
	sch := agenttestutils.TestScheme()
	ctx := context.Background()
	now := metav1.Now()

	testCases := []struct {
		name               string
		profile            v1alpha1.DatadogAgentProfile
		nodes              []corev1.Node
		profilesByNode     map[string]types.NamespacedName
		wantErr            error
		wantStatus         v1alpha1.DatadogAgentProfileStatus
		wantProfilesByNode map[string]types.NamespacedName
	}{
		{
			name: "valid profile with matching node",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "profile",
					Namespace: "default",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
							{
								Key:      "profile",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"enabled"},
							},
						},
					},
					Config: &v2alpha1.DatadogAgentSpec{
						Features: &v2alpha1.DatadogFeatures{
							GPU: &v2alpha1.GPUFeatureConfig{
								Enabled: apiutils.NewBoolPointer(true),
							},
						},
					},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"profile": "enabled",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"profile": "disabled",
						},
					},
				},
			},
			profilesByNode: make(map[string]types.NamespacedName),
			wantErr:        nil,
			wantStatus: v1alpha1.DatadogAgentProfileStatus{
				Conditions: []metav1.Condition{
					{
						Type:               agentprofile.ValidConditionType,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             agentprofile.ValidConditionReason,
						Message:            "Valid manifest",
					},
					{
						Type:               agentprofile.AppliedConditionType,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             agentprofile.AppliedConditionReason,
						Message:            "Profile applied",
					},
				},
			},
			wantProfilesByNode: map[string]types.NamespacedName{
				"node1": {
					Name:      "profile",
					Namespace: "default",
				},
			},
		},
		{
			name: "invalid profile name",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-profile-name-that-is-way-too-long-and-exceeds-kubernetes-limits-for-resource-names",
					Namespace: "default",
				},
			},
			nodes:          []corev1.Node{},
			profilesByNode: make(map[string]types.NamespacedName),
			wantErr:        fmt.Errorf("profile name is invalid: %w", fmt.Errorf("Profile name must be no more than 63 characters")),
			wantStatus: v1alpha1.DatadogAgentProfileStatus{
				Conditions: []metav1.Condition{
					{
						Type:               agentprofile.ValidConditionType,
						Status:             metav1.ConditionFalse,
						LastTransitionTime: now,
						Reason:             agentprofile.InvalidConditionReason,
						Message:            "profile name is invalid: Profile name must be no more than 63 characters",
					},
					{
						Type:               agentprofile.AppliedConditionType,
						Status:             metav1.ConditionUnknown,
						LastTransitionTime: now,
						Reason:             "",
						Message:            "",
					},
				},
			},
			wantProfilesByNode: make(map[string]types.NamespacedName),
		},
		{
			name: "profile conflict with existing profile",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "profile",
					Namespace: "default",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
							{
								Key:      "profile",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"enabled"},
							},
						},
					},
					Config: &v2alpha1.DatadogAgentSpec{
						Features: &v2alpha1.DatadogFeatures{
							GPU: &v2alpha1.GPUFeatureConfig{
								Enabled: apiutils.NewBoolPointer(true),
							},
						},
					},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"profile": "enabled",
						},
					},
				},
			},
			profilesByNode: map[string]types.NamespacedName{
				"node1": {
					Name:      "existing-profile",
					Namespace: "default",
				},
			},
			wantErr: fmt.Errorf("profile profile conflicts with existing profile: default/existing-profile"),
			wantStatus: v1alpha1.DatadogAgentProfileStatus{
				Conditions: []metav1.Condition{
					{
						Type:               agentprofile.ValidConditionType,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             agentprofile.ValidConditionReason,
						Message:            "Valid manifest",
					},
					{
						Type:               agentprofile.AppliedConditionType,
						Status:             metav1.ConditionFalse,
						LastTransitionTime: now,
						Reason:             agentprofile.ConflictConditionReason,
						Message:            "Conflict with existing profile",
					},
				},
			},
			wantProfilesByNode: map[string]types.NamespacedName{
				"node1": {
					Name:      "existing-profile",
					Namespace: "default",
				},
			},
		},
		{
			name: "no matching nodes",
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "profile",
					Namespace: "default",
				},
				Spec: v1alpha1.DatadogAgentProfileSpec{
					ProfileAffinity: &v1alpha1.ProfileAffinity{
						ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
							{
								Key:      "profile",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"enabled"},
							},
						},
					},
					Config: &v2alpha1.DatadogAgentSpec{
						Features: &v2alpha1.DatadogFeatures{
							GPU: &v2alpha1.GPUFeatureConfig{
								Enabled: apiutils.NewBoolPointer(true),
							},
						},
					},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"profile": "disabled",
						},
					},
				},
			},
			profilesByNode: make(map[string]types.NamespacedName),
			wantErr:        nil,
			wantStatus: v1alpha1.DatadogAgentProfileStatus{
				Conditions: []metav1.Condition{
					{
						Type:               agentprofile.ValidConditionType,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             agentprofile.ValidConditionReason,
						Message:            "Valid manifest",
					},
					{
						Type:               agentprofile.AppliedConditionType,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             agentprofile.AppliedConditionReason,
						Message:            "Profile applied",
					},
				},
			},
			wantProfilesByNode: make(map[string]types.NamespacedName),
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).Build()
			logger := logf.Log.WithName("Test_reconcileProfile")
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(sch, corev1.EventSource{Component: "Test_reconcileProfile"})

			r := &Reconciler{
				client:   fakeClient,
				log:      logger,
				scheme:   sch,
				recorder: recorder,
				options: ReconcilerOptions{
					DatadogAgentInternalEnabled: true,
				},
			}

			profileCopy := tt.profile.DeepCopy()
			err := r.reconcileProfile(ctx, profileCopy, tt.nodes, tt.profilesByNode, now)

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantStatus, profileCopy.Status)
			assert.Equal(t, tt.wantProfilesByNode, tt.profilesByNode)
		})
	}
}

func Test_reconcileProfiles(t *testing.T) {
	sch := agenttestutils.TestScheme()
	ctx := context.Background()

	testCases := []struct {
		name                string
		existingProfiles    []k8sruntime.Object
		existingNodes       []k8sruntime.Object
		wantErr             error
		wantAppliedProfiles int
	}{
		{
			name:                "no existing profiles",
			existingProfiles:    []k8sruntime.Object{},
			existingNodes:       []k8sruntime.Object{},
			wantErr:             nil,
			wantAppliedProfiles: 1, // default profile
		},
		{
			name: "one applied profile",
			existingProfiles: []k8sruntime.Object{
				&v1alpha1.DatadogAgentProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "profile",
						Namespace:         "default",
						CreationTimestamp: metav1.Now(),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
								{
									Key:      "profile",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"enabled"},
								},
							},
						},
						Config: &v2alpha1.DatadogAgentSpec{
							Features: &v2alpha1.DatadogFeatures{
								GPU: &v2alpha1.GPUFeatureConfig{
									Enabled: apiutils.NewBoolPointer(true),
								},
							},
						},
					},
				},
			},
			existingNodes: []k8sruntime.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"profile": "enabled",
						},
					},
				},
			},
			wantErr:             nil,
			wantAppliedProfiles: 2, // default + user profile
		},
		{
			name: "multiple profiles with conflicts",
			existingProfiles: []k8sruntime.Object{
				&v1alpha1.DatadogAgentProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "profile1",
						Namespace:         "default",
						CreationTimestamp: metav1.Time{Time: metav1.Now().Time.Add(-1 * time.Minute)},
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
								{
									Key:      "profile",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"enabled"},
								},
							},
						},
						Config: &v2alpha1.DatadogAgentSpec{
							Features: &v2alpha1.DatadogFeatures{
								GPU: &v2alpha1.GPUFeatureConfig{
									Enabled: apiutils.NewBoolPointer(true),
								},
							},
						},
					},
				},
				&v1alpha1.DatadogAgentProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "profile2",
						Namespace:         "default",
						CreationTimestamp: metav1.Now(),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
								{
									Key:      "profile",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"enabled"},
								},
							},
						},
						Config: &v2alpha1.DatadogAgentSpec{
							Features: &v2alpha1.DatadogFeatures{
								GPU: &v2alpha1.GPUFeatureConfig{
									Enabled: apiutils.NewBoolPointer(true),
								},
							},
						},
					},
				},
			},
			existingNodes: []k8sruntime.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"profile": "enabled",
						},
					},
				},
			},
			wantErr:             nil,
			wantAppliedProfiles: 2, // default + user profile
		},
		{
			name: "invalid profile name",
			existingProfiles: []k8sruntime.Object{
				&v1alpha1.DatadogAgentProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "invalid-profile-name-that-is-way-too-long-and-exceeds-kubernetes-limits",
						Namespace:         "default",
						CreationTimestamp: metav1.Now(),
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
								{
									Key:      "invalid",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"test"},
								},
							},
						},
						Config: &v2alpha1.DatadogAgentSpec{
							Features: &v2alpha1.DatadogFeatures{
								GPU: &v2alpha1.GPUFeatureConfig{
									Enabled: apiutils.NewBoolPointer(true),
								},
							},
						},
					},
				},
			},
			existingNodes:       []k8sruntime.Object{},
			wantErr:             nil,
			wantAppliedProfiles: 1, // default profile
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			objects := append(tt.existingProfiles, tt.existingNodes...)
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithRuntimeObjects(objects...).Build()
			logger := logf.Log.WithName("Test_reconcileProfiles")
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(sch, corev1.EventSource{Component: "Test_reconcileProfiles"})

			r := &Reconciler{
				client:   fakeClient,
				log:      logger,
				scheme:   sch,
				recorder: recorder,
				options: ReconcilerOptions{
					DatadogAgentInternalEnabled: true,
				},
			}

			appliedProfiles, err := r.reconcileProfiles(ctx)

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantAppliedProfiles, len(appliedProfiles))
		})
	}
}
