package datadogagent

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
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
							Enabled: ptr.To(true),
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
						constants.MD5DDAIDeploymentAnnotationKey: "10394c6b4f1e5029544f602ecb5a557b",
					},
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: ptr.To(true),
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
							Enabled: ptr.To(true),
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
						constants.MD5DDAIDeploymentAnnotationKey: "a9033f6ffba89ddf862136d39a5db466",
					},
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: ptr.To(true),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Name: ptr.To("foo-profile-agent"),
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
								constants.ProfileLabelKey: "foo-profile",
							},
						},
						v2alpha1.ClusterAgentComponentName: {
							Disabled: ptr.To(true),
						},
						v2alpha1.ClusterChecksRunnerComponentName: {
							Disabled: ptr.To(true),
						},
						v2alpha1.OtelAgentGatewayComponentName: {
							Disabled: ptr.To(true),
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
							Name: ptr.To("foo-profile-agent"),
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
							Disabled: ptr.To(true),
						},
						v2alpha1.ClusterChecksRunnerComponentName: {
							Disabled: ptr.To(true),
						},
						v2alpha1.OtelAgentGatewayComponentName: {
							Disabled: ptr.To(true),
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
			expectedNodeAgentComponentNameOverride: ptr.To("my-profile-agent"),
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
								Enabled: ptr.To(true),
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
				"node2": {
					Name:      "default",
					Namespace: "",
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
								Enabled: ptr.To(true),
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
								Enabled: ptr.To(true),
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
			wantProfilesByNode: map[string]types.NamespacedName{
				"node1": {
					Name:      "default",
					Namespace: "",
				},
			},
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
				options:  ReconcilerOptions{},
			}

			// Pre-populate with default profile for nodes without a profile (matching production code)
			for _, node := range tt.nodes {
				if _, exists := tt.profilesByNode[node.Name]; !exists {
					tt.profilesByNode[node.Name] = types.NamespacedName{Namespace: "", Name: "default"}
				}
			}

			profileCopy := tt.profile.DeepCopy()
			csInfo := make(map[types.NamespacedName]*agentprofile.CreateStrategyInfo)
			defaultSpec := &v2alpha1.DatadogAgentSpec{}
			baseDefaultSpec := defaultSpec.DeepCopy()
			err := r.reconcileProfile(ctx, profileCopy, tt.nodes, tt.profilesByNode, csInfo, defaultSpec, baseDefaultSpec, now)

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantStatus, profileCopy.Status)
			assert.Equal(t, tt.wantProfilesByNode, tt.profilesByNode)
		})
	}
}

func Test_reconcileProfile_SharedOverlayConflictDoesNotCommitNodeAssignment(t *testing.T) {
	sch := agenttestutils.TestScheme()
	ctx := context.Background()
	now := metav1.Now()
	fakeClient := fake.NewClientBuilder().WithScheme(sch).Build()
	logger := logf.Log.WithName("Test_reconcileProfile_SharedOverlayConflictDoesNotCommitNodeAssignment")
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(sch, corev1.EventSource{Component: "Test_reconcileProfile_SharedOverlayConflictDoesNotCommitNodeAssignment"})

	r := &Reconciler{
		client:   fakeClient,
		log:      logger,
		scheme:   sch,
		recorder: recorder,
		options:  ReconcilerOptions{},
	}

	profile := &v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu",
			Namespace: "default",
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
					{
						Key:      "profile",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"gpu"},
					},
				},
			},
			Config: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(true),
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:     ptr.To(true),
							LibVersions: map[string]string{"java": "1.44.0"},
						},
					},
				},
			},
		},
	}
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node1",
				Labels: map[string]string{"profile": "gpu"},
			},
		},
	}
	profilesByNode := map[string]types.NamespacedName{
		"node1": {Name: "default"},
	}
	csInfo := make(map[types.NamespacedName]*agentprofile.CreateStrategyInfo)
	defaultSpec := &v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{Enabled: ptr.To(true)},
			APM: &v2alpha1.APMFeatureConfig{
				SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
					Enabled:           ptr.To(true),
					LibVersions:       map[string]string{"java": "1.43.0"},
					LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
				},
			},
		},
	}
	baseDefaultSpec := defaultSpec.DeepCopy()

	nextProfilesByNode := maps.Clone(profilesByNode)
	nextCSInfo := agentprofile.CloneCreateStrategyInfoMap(csInfo)
	nextDefaultSpec := defaultSpec.DeepCopy()
	err := r.reconcileProfile(ctx, profile, nodes, nextProfilesByNode, nextCSInfo, nextDefaultSpec, baseDefaultSpec, now)

	assert.Error(t, err)
	assert.Equal(t, map[string]types.NamespacedName{"node1": {Name: "default"}}, profilesByNode)
	appliedCondition := metav1.Condition{}
	for _, condition := range profile.Status.Conditions {
		if condition.Type == agentprofile.AppliedConditionType {
			appliedCondition = condition
			break
		}
	}
	assert.Equal(t, metav1.ConditionFalse, appliedCondition.Status)
	assert.Equal(t, "1.43.0", defaultSpec.Features.APM.SingleStepInstrumentation.LibVersions["java"])
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
									Enabled: ptr.To(true),
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
									Enabled: ptr.To(true),
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
									Enabled: ptr.To(true),
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
									Enabled: ptr.To(true),
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
				options:  ReconcilerOptions{},
			}

			dsNSName := types.NamespacedName{
				Namespace: "default",
				Name:      "datadog-agent",
			}
			maxUnavailable := intstr.FromInt(1)
			defaultDDAI := &v1alpha1.DatadogAgentInternal{}
			appliedProfiles, err := r.reconcileProfiles(ctx, dsNSName, maxUnavailable, defaultDDAI)

			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantAppliedProfiles, len(appliedProfiles))
		})
	}
}

func Test_reconcileProfiles_APMSharedOverlayMatrix(t *testing.T) {
	sch := agenttestutils.TestScheme()
	ctx := context.Background()
	namespace := "default"
	baseTime := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	workerNode := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"profile": "worker"},
		},
	}

	tests := []struct {
		name                string
		description         string
		baseSpec            func() *v2alpha1.DatadogAgentSpec
		profiles            []*v1alpha1.DatadogAgentProfile
		nodes               []corev1.Node
		wantProfileApplied  map[string]metav1.ConditionStatus
		wantProfileMessages map[string]string
		assertAPM           func(*testing.T, *v2alpha1.APMFeatureConfig)
		assertSSI           func(*testing.T, *v2alpha1.SingleStepInstrumentation)
	}{
		{
			name:        "base APM off admission on DAP SSI on",
			description: "Expect the DAP to apply and enable SSI on the default DDAI even though base APM is disabled.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-happy", baseTime, noMatchProfileRequirement("dap-case-happy"), &v2alpha1.APMFeatureConfig{
					SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
						Enabled:           ptr.To(true),
						EnabledNamespaces: []string{"payments"},
						LibVersions:       map[string]string{"java": "1.43.0"},
						LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
					},
				}),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{"dap-case-happy": metav1.ConditionTrue},
			assertAPM: func(t *testing.T, apm *v2alpha1.APMFeatureConfig) {
				require.NotNil(t, apm)
				assert.False(t, ptr.Deref(apm.Enabled, true))
			},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.True(t, ptr.Deref(ssi.Enabled, false))
				assert.Equal(t, []string{"payments"}, ssi.EnabledNamespaces)
				assert.Equal(t, "1.43.0", ssi.LibVersions["java"])
				assert.True(t, ptr.Deref(ssi.LanguageDetection.Enabled, false))
			},
		},
		{
			name:        "base APM off DAP APM on applies without enabling default DDAI APM",
			description: "Expect the DAP to apply while leaving the default DDAI node APM disabled.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-apm-only", baseTime, noMatchProfileRequirement("dap-case-apm-only"), &v2alpha1.APMFeatureConfig{
					Enabled: ptr.To(true),
				}),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{"dap-case-apm-only": metav1.ConditionTrue},
			assertAPM: func(t *testing.T, apm *v2alpha1.APMFeatureConfig) {
				require.NotNil(t, apm)
				assert.False(t, ptr.Deref(apm.Enabled, true))
			},
			assertSSI: assertSSIEnabled(false),
		},
		{
			name:        "DAP apm enabled false with SSI enabled",
			description: "Expect the DAP to be rejected because it explicitly disables APM while enabling SSI.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-apm-disabled", baseTime, noMatchProfileRequirement("dap-case-apm-disabled"), &v2alpha1.APMFeatureConfig{
					Enabled: ptr.To(false),
					SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
						Enabled: ptr.To(true),
					},
				}),
			},
			wantProfileApplied:  map[string]metav1.ConditionStatus{"dap-case-apm-disabled": metav1.ConditionFalse},
			wantProfileMessages: map[string]string{"dap-case-apm-disabled": "features.apm.enabled must be true or unset"},
			assertSSI:           assertSSIEnabled(false),
		},
		{
			name:        "base admission controller disabled with profile SSI enabled",
			description: "Expect the DAP to be rejected because base admission controller is required for SSI.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				spec := testAPMMatrixBaseSpec()
				spec.Features.AdmissionController.Enabled = ptr.To(false)
				return spec
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-no-admission", baseTime, noMatchProfileRequirement("dap-case-no-admission"), testAPMMatrixHappyConfig()),
			},
			wantProfileApplied:  map[string]metav1.ConditionStatus{"dap-case-no-admission": metav1.ConditionFalse},
			wantProfileMessages: map[string]string{"dap-case-no-admission": "features.admissionController.enabled must be true"},
			assertSSI:           assertSSIEnabled(false),
		},
		{
			name:        "base admission controller disabled with profile APM enabled only",
			description: "Expect the DAP to apply because plain profile APM config stays profile-local and does not need shared SSI admission config.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				spec := testAPMMatrixBaseSpec()
				spec.Features.AdmissionController.Enabled = ptr.To(false)
				return spec
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-no-admission-apm-only", baseTime, noMatchProfileRequirement("dap-case-no-admission-apm-only"), &v2alpha1.APMFeatureConfig{
					Enabled: ptr.To(true),
				}),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{"dap-case-no-admission-apm-only": metav1.ConditionTrue},
			assertSSI:          assertSSIEnabled(false),
		},
		{
			name:        "base clusterAgent disabled with profile SSI enabled",
			description: "Expect the DAP to be rejected because shared SSI config requires the base Cluster Agent.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				spec := testAPMMatrixBaseSpec()
				spec.Override[v2alpha1.ClusterAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{Disabled: ptr.To(true)}
				return spec
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-no-dca", baseTime, noMatchProfileRequirement("dap-case-no-dca"), testAPMMatrixHappyConfig()),
			},
			wantProfileApplied:  map[string]metav1.ConditionStatus{"dap-case-no-dca": metav1.ConditionFalse},
			wantProfileMessages: map[string]string{"dap-case-no-dca": "clusterAgent cannot be disabled"},
			assertSSI:           assertSSIEnabled(false),
		},
		{
			name:        "base clusterAgent disabled with profile APM enabled only",
			description: "Expect the DAP to apply because plain profile APM config stays profile-local and does not need shared SSI Cluster Agent config.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				spec := testAPMMatrixBaseSpec()
				spec.Override[v2alpha1.ClusterAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{Disabled: ptr.To(true)}
				return spec
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-no-dca-apm-only", baseTime, noMatchProfileRequirement("dap-case-no-dca-apm-only"), &v2alpha1.APMFeatureConfig{
					Enabled: ptr.To(true),
				}),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{"dap-case-no-dca-apm-only": metav1.ConditionTrue},
			assertSSI:          assertSSIEnabled(false),
		},
		{
			name:        "enabledNamespaces on base conflicts with disabledNamespaces on DAP",
			description: "Expect the DAP to be rejected and the base enabledNamespaces config to remain unchanged.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				spec := testAPMMatrixBaseSpec()
				spec.Features.APM.SingleStepInstrumentation.Enabled = ptr.To(true)
				spec.Features.APM.SingleStepInstrumentation.EnabledNamespaces = []string{"default"}
				return spec
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-ns-conflict", baseTime, noMatchProfileRequirement("dap-case-ns-conflict"), &v2alpha1.APMFeatureConfig{
					SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
						Enabled:            ptr.To(true),
						DisabledNamespaces: []string{"kube-system"},
					},
				}),
			},
			wantProfileApplied:  map[string]metav1.ConditionStatus{"dap-case-ns-conflict": metav1.ConditionFalse},
			wantProfileMessages: map[string]string{"dap-case-ns-conflict": "enabledNamespaces and features.apm.instrumentation.disabledNamespaces cannot both be set"},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.True(t, ptr.Deref(ssi.Enabled, false))
				assert.Equal(t, []string{"default"}, ssi.EnabledNamespaces)
				assert.Empty(t, ssi.DisabledNamespaces)
			},
		},
		{
			name:        "multiple DAPs merge distinct libVersions",
			description: "Expect both DAPs to apply and merge distinct tracer library versions into default DDAI SSI.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-lib-java", baseTime, noMatchProfileRequirement("dap-case-lib-java"), testAPMMatrixLibVersionConfig("java", "1.43.0")),
				testAPMMatrixProfile(namespace, "dap-case-lib-python", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-lib-python"), testAPMMatrixLibVersionConfig("python", "2.14.0")),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-lib-java":   metav1.ConditionTrue,
				"dap-case-lib-python": metav1.ConditionTrue,
			},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.True(t, ptr.Deref(ssi.Enabled, false))
				assert.Equal(t, map[string]string{"java": "1.43.0", "python": "2.14.0"}, ssi.LibVersions)
			},
		},
		{
			name:        "libVersions conflict rejects second profile",
			description: "Expect the first DAP to apply, the conflicting second DAP to be rejected, and the first libVersion to remain.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-lib-conflict-a", baseTime, noMatchProfileRequirement("dap-case-lib-conflict-a"), testAPMMatrixLibVersionConfig("java", "1.43.0")),
				testAPMMatrixProfile(namespace, "dap-case-lib-conflict-b", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-lib-conflict-b"), testAPMMatrixLibVersionConfig("java", "1.44.0")),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-lib-conflict-a": metav1.ConditionTrue,
				"dap-case-lib-conflict-b": metav1.ConditionFalse,
			},
			wantProfileMessages: map[string]string{"dap-case-lib-conflict-b": `libVersions["java"] has conflicting values`},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.Equal(t, map[string]string{"java": "1.43.0"}, ssi.LibVersions)
			},
		},
		{
			name:        "injector tag conflict rejects second profile",
			description: "Expect the first injector tag to apply and the second DAP to be rejected for changing that singleton value.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-injector-a", baseTime, noMatchProfileRequirement("dap-case-injector-a"), testAPMMatrixInjectorConfig("7.66.0")),
				testAPMMatrixProfile(namespace, "dap-case-injector-b", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-injector-b"), testAPMMatrixInjectorConfig("7.67.0")),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-injector-a": metav1.ConditionTrue,
				"dap-case-injector-b": metav1.ConditionFalse,
			},
			wantProfileMessages: map[string]string{"dap-case-injector-b": "injector.imageTag has conflicting values"},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				require.NotNil(t, ssi.Injector)
				assert.Equal(t, "7.66.0", ssi.Injector.ImageTag)
			},
		},
		{
			name:        "injectionMode conflict rejects second profile",
			description: "Expect the first injection mode to apply and the second DAP to be rejected for changing that singleton value.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-mode-a", baseTime, noMatchProfileRequirement("dap-case-mode-a"), testAPMMatrixInjectionModeConfig(v2alpha1.InjectionModeInitContainer)),
				testAPMMatrixProfile(namespace, "dap-case-mode-b", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-mode-b"), testAPMMatrixInjectionModeConfig(v2alpha1.InjectionModeCSI)),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-mode-a": metav1.ConditionTrue,
				"dap-case-mode-b": metav1.ConditionFalse,
			},
			wantProfileMessages: map[string]string{"dap-case-mode-b": "injectionMode has conflicting values"},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.Equal(t, v2alpha1.InjectionModeInitContainer, ssi.InjectionMode)
			},
		},
		{
			name:        "target-based SSI on supported Cluster Agent",
			description: "Expect target-based SSI to apply when the base Cluster Agent image is at or above the required version.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				return testAPMMatrixBaseSpecWithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.64.0")
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-target-supported", baseTime, noMatchProfileRequirement("dap-case-target-supported"), testAPMMatrixTargetConfig("api", "api", map[string]string{"java": "1.43.0"}, []corev1.EnvVar{{Name: "DD_TRACE_DEBUG", Value: "true"}})),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{"dap-case-target-supported": metav1.ConditionTrue},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				require.Len(t, ssi.Targets, 1)
				assert.Equal(t, "api", ssi.Targets[0].Name)
				assert.Equal(t, "1.43.0", ssi.Targets[0].TracerVersions["java"])
			},
		},
		{
			name:        "target-based SSI on unsupported Cluster Agent",
			description: "Expect target-based SSI to be rejected when the base Cluster Agent image is below the required version.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				return testAPMMatrixBaseSpecWithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.63.0")
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-target-unsupported", baseTime, noMatchProfileRequirement("dap-case-target-unsupported"), testAPMMatrixTargetConfig("api", "api", nil, nil)),
			},
			wantProfileApplied:  map[string]metav1.ConditionStatus{"dap-case-target-unsupported": metav1.ConditionFalse},
			wantProfileMessages: map[string]string{"dap-case-target-unsupported": "targets requires Cluster Agent version"},
			assertSSI:           assertSSIEnabled(false),
		},
		{
			name:        "target overlays append in deterministic profile order",
			description: "Expect all target overlays to apply and append in sorted profile order; targets remain an ordered first-match-wins list, even when names match.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				return testAPMMatrixBaseSpecWithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.64.0")
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-target-a", baseTime, noMatchProfileRequirement("dap-case-target-a"), testAPMMatrixTargetConfig("api", "api", map[string]string{"java": "1.43.0"}, nil)),
				testAPMMatrixProfile(namespace, "dap-case-target-b", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-target-b"), testAPMMatrixTargetConfig("api", "api", map[string]string{"python": "2.14.0"}, nil)),
				testAPMMatrixProfile(namespace, "dap-case-target-c", metav1.NewTime(baseTime.Add(2*time.Second)), noMatchProfileRequirement("dap-case-target-c"), testAPMMatrixTargetConfig("api", "api-v2", nil, nil)),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-target-a": metav1.ConditionTrue,
				"dap-case-target-b": metav1.ConditionTrue,
				"dap-case-target-c": metav1.ConditionTrue,
			},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				require.Len(t, ssi.Targets, 3)
				assert.Equal(t, map[string]string{"java": "1.43.0"}, ssi.Targets[0].TracerVersions)
				assert.Equal(t, map[string]string{"python": "2.14.0"}, ssi.Targets[1].TracerVersions)
				assert.Equal(t, map[string]string{"app": "api-v2"}, ssi.Targets[2].PodSelector.MatchLabels)
			},
		},
		{
			name:        "unnamed target-only overlay is preserved",
			description: "Expect the DAP to apply and preserve the unnamed target as an ordered target entry because target.name is optional; unnamed targets are appended, not name-merged.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				return testAPMMatrixBaseSpecWithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.64.0")
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-unnamed-target", baseTime, noMatchProfileRequirement("dap-case-unnamed-target"), &v2alpha1.APMFeatureConfig{
					SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
						Enabled: ptr.To(true),
						Targets: []v2alpha1.SSITarget{
							{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}},
						},
					},
				}),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{"dap-case-unnamed-target": metav1.ConditionTrue},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.True(t, ptr.Deref(ssi.Enabled, false))
				require.Len(t, ssi.Targets, 1)
				assert.Empty(t, ssi.Targets[0].Name)
				assert.Equal(t, map[string]string{"app": "api"}, ssi.Targets[0].PodSelector.MatchLabels)
			},
		},
		{
			name:        "instrumentation enabled false with enabledNamespaces is skipped",
			description: "Expect the DAP to apply but skip shared SSI overlay because instrumentation is explicitly disabled.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-enabled-false-ns", baseTime, noMatchProfileRequirement("dap-case-enabled-false-ns"), &v2alpha1.APMFeatureConfig{
					SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
						Enabled:           ptr.To(false),
						EnabledNamespaces: []string{"payments"},
					},
				}),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{"dap-case-enabled-false-ns": metav1.ConditionTrue},
			assertSSI:          assertSSIEnabled(false),
		},
		{
			name:        "two profiles match the same real node and second does not contribute overlay",
			description: "Expect node assignment conflict to reject the second DAP before it can contribute shared SSI config.",
			nodes:       []corev1.Node{workerNode},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-node-conflict-a", baseTime, workerProfileRequirement(), testAPMMatrixLibVersionConfig("java", "1.43.0")),
				testAPMMatrixProfile(namespace, "dap-case-node-conflict-b", metav1.NewTime(baseTime.Add(time.Second)), workerProfileRequirement(), testAPMMatrixLibVersionConfig("python", "2.14.0")),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-node-conflict-a": metav1.ConditionTrue,
				"dap-case-node-conflict-b": metav1.ConditionFalse,
			},
			wantProfileMessages: map[string]string{"dap-case-node-conflict-b": "Conflict with existing profile"},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.Equal(t, map[string]string{"java": "1.43.0"}, ssi.LibVersions)
			},
		},
		{
			name:        "first profile fails overlay and second profile matching same node succeeds",
			description: "Expect a failed overlay to roll back node assignment and shared config so the next same-node DAP can apply.",
			nodes:       []corev1.Node{workerNode},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-rollback-a", baseTime, workerProfileRequirement(), &v2alpha1.APMFeatureConfig{
					Enabled:                   ptr.To(false),
					SingleStepInstrumentation: testAPMMatrixLibVersionConfig("java", "1.43.0").SingleStepInstrumentation,
				}),
				testAPMMatrixProfile(namespace, "dap-case-rollback-b", metav1.NewTime(baseTime.Add(time.Second)), workerProfileRequirement(), testAPMMatrixLibVersionConfig("python", "2.14.0")),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-rollback-a": metav1.ConditionFalse,
				"dap-case-rollback-b": metav1.ConditionTrue,
			},
			wantProfileMessages: map[string]string{"dap-case-rollback-a": "features.apm.enabled must be true or unset"},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.Equal(t, map[string]string{"python": "2.14.0"}, ssi.LibVersions)
			},
		},
		{
			name:        "disjoint profiles with one failed overlay and one successful overlay",
			description: "Expect a failed DAP overlay not to poison later valid shared SSI overlays from other profiles.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-disjoint-fail", baseTime, noMatchProfileRequirement("dap-case-disjoint-fail"), &v2alpha1.APMFeatureConfig{
					Enabled:                   ptr.To(false),
					SingleStepInstrumentation: testAPMMatrixLibVersionConfig("java", "1.43.0").SingleStepInstrumentation,
				}),
				testAPMMatrixProfile(namespace, "dap-case-disjoint-ok", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-disjoint-ok"), testAPMMatrixLibVersionConfig("python", "2.14.0")),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-disjoint-fail": metav1.ConditionFalse,
				"dap-case-disjoint-ok":   metav1.ConditionTrue,
			},
			wantProfileMessages: map[string]string{"dap-case-disjoint-fail": "features.apm.enabled must be true or unset"},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.Equal(t, map[string]string{"python": "2.14.0"}, ssi.LibVersions)
			},
		},
		{
			name:        "languageDetection singleton bool conflict",
			description: "Expect the first language detection value to apply and the conflicting second value to be rejected.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-lang-a", baseTime, noMatchProfileRequirement("dap-case-lang-a"), testAPMMatrixLanguageDetectionConfig(true)),
				testAPMMatrixProfile(namespace, "dap-case-lang-b", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-lang-b"), testAPMMatrixLanguageDetectionConfig(false)),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-lang-a": metav1.ConditionTrue,
				"dap-case-lang-b": metav1.ConditionFalse,
			},
			wantProfileMessages: map[string]string{"dap-case-lang-b": "languageDetection.enabled has conflicting values"},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.True(t, ptr.Deref(ssi.LanguageDetection.Enabled, false))
			},
		},
		{
			name:        "DAP enabledNamespaces conflicts with later DAP disabledNamespaces",
			description: "Expect enabledNamespaces from the first DAP to remain and disabledNamespaces from the later DAP to be rejected.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-ns-a", baseTime, noMatchProfileRequirement("dap-case-ns-a"), &v2alpha1.APMFeatureConfig{
					SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
						Enabled:           ptr.To(true),
						EnabledNamespaces: []string{"payments"},
					},
				}),
				testAPMMatrixProfile(namespace, "dap-case-ns-b", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-ns-b"), &v2alpha1.APMFeatureConfig{
					SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
						Enabled:            ptr.To(true),
						DisabledNamespaces: []string{"kube-system"},
					},
				}),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-ns-a": metav1.ConditionTrue,
				"dap-case-ns-b": metav1.ConditionFalse,
			},
			wantProfileMessages: map[string]string{"dap-case-ns-b": "enabledNamespaces and features.apm.instrumentation.disabledNamespaces cannot both be set"},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.Equal(t, []string{"payments"}, ssi.EnabledNamespaces)
				assert.Empty(t, ssi.DisabledNamespaces)
			},
		},
		{
			name:        "same target and env var name with different ddTraceConfigs appends",
			description: "Expect both target entries to apply in order; if both match the same pod, the Cluster Agent uses the first matching target.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				return testAPMMatrixBaseSpecWithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.64.0")
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-target-env-a", baseTime, noMatchProfileRequirement("dap-case-target-env-a"), testAPMMatrixTargetConfig("api", "api", nil, []corev1.EnvVar{{Name: "DD_TRACE_DEBUG", Value: "true"}})),
				testAPMMatrixProfile(namespace, "dap-case-target-env-b", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-target-env-b"), testAPMMatrixTargetConfig("api", "api", nil, []corev1.EnvVar{{Name: "DD_TRACE_DEBUG", Value: "false"}})),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-target-env-a": metav1.ConditionTrue,
				"dap-case-target-env-b": metav1.ConditionTrue,
			},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				require.Len(t, ssi.Targets, 2)
				require.Len(t, ssi.Targets[0].TracerConfigs, 1)
				require.Len(t, ssi.Targets[1].TracerConfigs, 1)
				assert.Equal(t, "true", ssi.Targets[0].TracerConfigs[0].Value)
				assert.Equal(t, "false", ssi.Targets[1].TracerConfigs[0].Value)
			},
		},
		{
			name:        "same target and identical ddTraceConfigs appends",
			description: "Expect duplicate identical target entries to apply and remain as separate ordered target rules.",
			baseSpec: func() *v2alpha1.DatadogAgentSpec {
				return testAPMMatrixBaseSpecWithClusterAgentImage("gcr.io/datadoghq/cluster-agent:7.64.0")
			},
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-target-env-same-a", baseTime, noMatchProfileRequirement("dap-case-target-env-same-a"), testAPMMatrixTargetConfig("api", "api", nil, []corev1.EnvVar{{Name: "DD_TRACE_DEBUG", Value: "true"}})),
				testAPMMatrixProfile(namespace, "dap-case-target-env-same-b", metav1.NewTime(baseTime.Add(time.Second)), noMatchProfileRequirement("dap-case-target-env-same-b"), testAPMMatrixTargetConfig("api", "api", nil, []corev1.EnvVar{{Name: "DD_TRACE_DEBUG", Value: "true"}})),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{
				"dap-case-target-env-same-a": metav1.ConditionTrue,
				"dap-case-target-env-same-b": metav1.ConditionTrue,
			},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				require.Len(t, ssi.Targets, 2)
				require.Len(t, ssi.Targets[0].TracerConfigs, 1)
				require.Len(t, ssi.Targets[1].TracerConfigs, 1)
				assert.Equal(t, "true", ssi.Targets[0].TracerConfigs[0].Value)
				assert.Equal(t, "true", ssi.Targets[1].TracerConfigs[0].Value)
			},
		},
		{
			name:        "no matching nodes still contributes shared SSI config",
			description: "Expect a DAP with no matching nodes to still contribute shared SSI config to the default DDAI.",
			profiles: []*v1alpha1.DatadogAgentProfile{
				testAPMMatrixProfile(namespace, "dap-case-no-match-contributes", baseTime, noMatchProfileRequirement("dap-case-no-match-contributes"), testAPMMatrixLibVersionConfig("ruby", "2.0.0")),
			},
			wantProfileApplied: map[string]metav1.ConditionStatus{"dap-case-no-match-contributes": metav1.ConditionTrue},
			assertSSI: func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
				require.NotNil(t, ssi)
				assert.Equal(t, map[string]string{"ruby": "2.0.0"}, ssi.LibVersions)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.description)
			objects := make([]k8sruntime.Object, 0, len(tt.profiles)+len(tt.nodes))
			for _, profile := range tt.profiles {
				objects = append(objects, profile.DeepCopy())
			}
			for _, node := range tt.nodes {
				nodeCopy := node.DeepCopy()
				objects = append(objects, nodeCopy)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(sch).
				WithRuntimeObjects(objects...).
				WithStatusSubresource(&v1alpha1.DatadogAgentProfile{}).
				Build()
			logger := logf.Log.WithName("Test_reconcileProfiles_APMSharedOverlayMatrix")
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(sch, corev1.EventSource{Component: "Test_reconcileProfiles_APMSharedOverlayMatrix"})

			r := &Reconciler{
				client:   fakeClient,
				log:      logger,
				scheme:   sch,
				recorder: recorder,
				options:  ReconcilerOptions{},
			}

			baseSpec := testAPMMatrixBaseSpec()
			if tt.baseSpec != nil {
				baseSpec = tt.baseSpec()
			}
			defaultDDAI := &v1alpha1.DatadogAgentInternal{Spec: *baseSpec}
			appliedProfiles, err := r.reconcileProfiles(ctx, types.NamespacedName{Namespace: namespace, Name: "datadog-agent"}, intstr.FromInt(1), defaultDDAI)
			require.NoError(t, err)

			wantAppliedCount := 1
			for _, status := range tt.wantProfileApplied {
				if status == metav1.ConditionTrue {
					wantAppliedCount++
				}
			}
			assert.Len(t, appliedProfiles, wantAppliedCount)

			for profileName, wantApplied := range tt.wantProfileApplied {
				profile := &v1alpha1.DatadogAgentProfile{}
				err := fakeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: profileName}, profile)
				require.NoError(t, err)
				assert.Equal(t, metav1.ConditionTrue, profile.Status.Valid, "profile %s valid status", profileName)
				assert.Equal(t, wantApplied, profile.Status.Applied, "profile %s applied status", profileName)
				if wantMessage := tt.wantProfileMessages[profileName]; wantMessage != "" {
					assert.Contains(t, testAPMMatrixConditionMessage(profile.Status.Conditions, agentprofile.AppliedConditionType), wantMessage)
				}
			}

			require.NotNil(t, defaultDDAI.Spec.Features)
			require.NotNil(t, defaultDDAI.Spec.Features.APM)
			if tt.assertAPM != nil {
				tt.assertAPM(t, defaultDDAI.Spec.Features.APM)
			}
			if tt.assertSSI != nil {
				tt.assertSSI(t, defaultDDAI.Spec.Features.APM.SingleStepInstrumentation)
			}
		})
	}
}

func testAPMMatrixBaseSpec() *v2alpha1.DatadogAgentSpec {
	return &v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{Enabled: ptr.To(true)},
			APM: &v2alpha1.APMFeatureConfig{
				Enabled: ptr.To(false),
				SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
					Enabled:           ptr.To(false),
					LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
					Injector:          &v2alpha1.InjectorConfig{},
				},
			},
		},
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{},
	}
}

func testAPMMatrixBaseSpecWithClusterAgentImage(imageName string) *v2alpha1.DatadogAgentSpec {
	spec := testAPMMatrixBaseSpec()
	spec.Override[v2alpha1.ClusterAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{
		Image: &v2alpha1.AgentImageConfig{Name: imageName},
	}
	return spec
}

func testAPMMatrixProfile(namespace, name string, creationTimestamp metav1.Time, requirement corev1.NodeSelectorRequirement, apm *v2alpha1.APMFeatureConfig) *v1alpha1.DatadogAgentProfile {
	return &v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: creationTimestamp,
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []corev1.NodeSelectorRequirement{requirement},
			},
			Config: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{APM: apm},
			},
		},
	}
}

func noMatchProfileRequirement(value string) corev1.NodeSelectorRequirement {
	return corev1.NodeSelectorRequirement{
		Key:      "profile",
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{value},
	}
}

func workerProfileRequirement() corev1.NodeSelectorRequirement {
	return noMatchProfileRequirement("worker")
}

func testAPMMatrixHappyConfig() *v2alpha1.APMFeatureConfig {
	return &v2alpha1.APMFeatureConfig{
		SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
			Enabled:           ptr.To(true),
			EnabledNamespaces: []string{"payments"},
			LibVersions:       map[string]string{"java": "1.43.0"},
			LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(true)},
		},
	}
}

func testAPMMatrixLibVersionConfig(language, version string) *v2alpha1.APMFeatureConfig {
	return &v2alpha1.APMFeatureConfig{
		SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
			Enabled:     ptr.To(true),
			LibVersions: map[string]string{language: version},
		},
	}
}

func testAPMMatrixInjectorConfig(imageTag string) *v2alpha1.APMFeatureConfig {
	return &v2alpha1.APMFeatureConfig{
		SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
			Enabled:  ptr.To(true),
			Injector: &v2alpha1.InjectorConfig{ImageTag: imageTag},
		},
	}
}

func testAPMMatrixInjectionModeConfig(mode v2alpha1.InjectionModeType) *v2alpha1.APMFeatureConfig {
	return &v2alpha1.APMFeatureConfig{
		SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
			Enabled:       ptr.To(true),
			InjectionMode: mode,
		},
	}
}

func testAPMMatrixLanguageDetectionConfig(enabled bool) *v2alpha1.APMFeatureConfig {
	return &v2alpha1.APMFeatureConfig{
		SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
			Enabled:           ptr.To(true),
			LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(enabled)},
		},
	}
}

func testAPMMatrixTargetConfig(name, app string, tracerVersions map[string]string, tracerConfigs []corev1.EnvVar) *v2alpha1.APMFeatureConfig {
	return &v2alpha1.APMFeatureConfig{
		SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
			Enabled: ptr.To(true),
			Targets: []v2alpha1.SSITarget{
				{
					Name: name,
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": app},
					},
					TracerVersions: tracerVersions,
					TracerConfigs:  tracerConfigs,
				},
			},
		},
	}
}

func assertSSIEnabled(want bool) func(*testing.T, *v2alpha1.SingleStepInstrumentation) {
	return func(t *testing.T, ssi *v2alpha1.SingleStepInstrumentation) {
		require.NotNil(t, ssi)
		assert.Equal(t, want, ptr.Deref(ssi.Enabled, false))
	}
}

func testAPMMatrixConditionMessage(conditions []metav1.Condition, conditionType string) string {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Message
		}
	}
	return ""
}
