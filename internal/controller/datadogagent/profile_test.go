package datadogagent

import (
	"context"
	"testing"

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
						},
					},
				},
			},
			profile: v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-profile",
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
					Name:      "foo-profile-foo-profile",
					Namespace: "bar",
					Annotations: map[string]string{
						constants.MD5DDAIDeploymentAnnotationKey: "fb25e5160453e69437f7a77848f5c0d9",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "datadoghq.com/v1alpha1",
							Kind:               "DatadogAgentProfile",
							Name:               "foo-profile",
							Controller:         apiutils.NewBoolPointer(true),
							BlockOwnerDeletion: apiutils.NewBoolPointer(true),
						},
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
						},
						v2alpha1.ClusterAgentComponentName: &v2alpha1.DatadogAgentComponentOverride{
							Disabled: apiutils.NewBoolPointer(true),
						},
						v2alpha1.ClusterChecksRunnerComponentName: &v2alpha1.DatadogAgentComponentOverride{
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

		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:   fakeClient,
				log:      logger,
				scheme:   sch,
				recorder: recorder,
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
			assert.Equal(t, tt.want.OwnerReferences, ddai.OwnerReferences)
			assert.Equal(t, tt.want.Annotations[constants.MD5DDAIDeploymentAnnotationKey], ddai.Annotations[constants.MD5DDAIDeploymentAnnotationKey])
			assert.Equal(t, tt.want.Spec, ddai.Spec)
		})
	}
}
