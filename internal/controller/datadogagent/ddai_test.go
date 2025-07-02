package datadogagent

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
)

func Test_generateObjMetaFromDDA(t *testing.T) {
	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want *v1alpha1.DatadogAgentInternal
	}{
		{
			name: "minimal dda",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			},
			want: &v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"agent.datadoghq.com/datadogagent": "foo",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "datadoghq.com/v2alpha1",
							Kind:               "DatadogAgent",
							Name:               "foo",
							UID:                "",
							BlockOwnerDeletion: apiutils.NewBoolPointer(true),
							Controller:         apiutils.NewBoolPointer(true),
						},
					},
				},
			},
		},
		{
			name: "dda with meta",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
			},
			want: &v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"foo":                              "bar",
						"agent.datadoghq.com/datadogagent": "foo",
					},
					Annotations: map[string]string{
						"foo": "bar",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "datadoghq.com/v2alpha1",
							Kind:               "DatadogAgent",
							Name:               "foo",
							UID:                "",
							BlockOwnerDeletion: apiutils.NewBoolPointer(true),
							Controller:         apiutils.NewBoolPointer(true),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ddai := &v1alpha1.DatadogAgentInternal{}
			generateObjMetaFromDDA(tt.dda, ddai, agenttestutils.TestScheme())
			assert.Equal(t, tt.want, ddai)
		})
	}
}

func Test_generateSpecFromDDA(t *testing.T) {
	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want *v1alpha1.DatadogAgentInternal
	}{
		{
			name: "empty dda override",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						Credentials: &v2alpha1.DatadogCredentials{
							APIKey: apiutils.NewStringPointer("key"),
						},
					},
				},
			},
			want: &v1alpha1.DatadogAgentInternal{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						Credentials: &v2alpha1.DatadogCredentials{
							APISecret: &v2alpha1.SecretConfig{
								SecretName: "foo-secret",
								KeyName:    "api_key",
							},
						},
						ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
							SecretName: "foo-token",
							KeyName:    "token",
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: "",
							},
						},
					},
				},
			},
		},
		{
			name: "dda override configured",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						Credentials: &v2alpha1.DatadogCredentials{
							APIKey: apiutils.NewStringPointer("key"),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Labels: map[string]string{
								"foo": "bar",
							},
							PriorityClassName: apiutils.NewStringPointer("foo-priority-class"),
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "key",
														Operator: corev1.NodeSelectorOpIn,
													},
												},
											},
										},
									},
								},
							},
						},
						v2alpha1.ClusterAgentComponentName: {
							PriorityClassName: apiutils.NewStringPointer("bar-priority-class"),
						},
					},
				},
			},
			want: &v1alpha1.DatadogAgentInternal{
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						Credentials: &v2alpha1.DatadogCredentials{
							APISecret: &v2alpha1.SecretConfig{
								SecretName: "foo-secret",
								KeyName:    "api_key",
							},
						},
						ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
							SecretName: "foo-token",
							KeyName:    "token",
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: "",
								"foo": "bar",
							},
							PriorityClassName: apiutils.NewStringPointer("foo-priority-class"),
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{
													{
														Key:      "key",
														Operator: corev1.NodeSelectorOpIn,
													},
												},
											},
										},
									},
								},
							},
						},
						v2alpha1.ClusterAgentComponentName: {
							PriorityClassName: apiutils.NewStringPointer("bar-priority-class"),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ddai := &v1alpha1.DatadogAgentInternal{}
			generateSpecFromDDA(tt.dda, ddai)
			assert.Equal(t, tt.want, ddai)
		})
	}
}

func Test_addRemoteConfigStatusToDDAIStatus(t *testing.T) {
	sch := agenttestutils.TestScheme()
	tests := []struct {
		name      string
		ddaStatus v2alpha1.DatadogAgentStatus
		ddai      *v1alpha1.DatadogAgentInternal
		want      *v1alpha1.DatadogAgentInternal
	}{
		{
			name: "nil remote config configuration",
			ddaStatus: v2alpha1.DatadogAgentStatus{
				RemoteConfigConfiguration: nil,
			},
			ddai: &v1alpha1.DatadogAgentInternal{
				Status: v1alpha1.DatadogAgentInternalStatus{},
			},
			want: &v1alpha1.DatadogAgentInternal{
				Status: v1alpha1.DatadogAgentInternalStatus{},
			},
		},
		{
			name: "empty remote config configuration",
			ddaStatus: v2alpha1.DatadogAgentStatus{
				RemoteConfigConfiguration: &v2alpha1.RemoteConfigConfiguration{},
			},
			ddai: &v1alpha1.DatadogAgentInternal{
				Status: v1alpha1.DatadogAgentInternalStatus{},
			},
			want: &v1alpha1.DatadogAgentInternal{
				Status: v1alpha1.DatadogAgentInternalStatus{
					RemoteConfigConfiguration: &v2alpha1.RemoteConfigConfiguration{},
				},
			},
		},
		{
			name: "remote config configuration with CSPM enabled",
			ddaStatus: v2alpha1.DatadogAgentStatus{
				RemoteConfigConfiguration: &v2alpha1.RemoteConfigConfiguration{
					Features: &v2alpha1.DatadogFeatures{
						CSPM: &v2alpha1.CSPMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
			ddai: &v1alpha1.DatadogAgentInternal{
				Status: v1alpha1.DatadogAgentInternalStatus{},
			},
			want: &v1alpha1.DatadogAgentInternal{
				Status: v1alpha1.DatadogAgentInternalStatus{
					RemoteConfigConfiguration: &v2alpha1.RemoteConfigConfiguration{
						Features: &v2alpha1.DatadogFeatures{
							CSPM: &v2alpha1.CSPMFeatureConfig{
								Enabled: apiutils.NewBoolPointer(true),
							},
						},
					},
				},
			},
		},
		{
			name: "remote config configuration with CSPM enabled, existing status",
			ddaStatus: v2alpha1.DatadogAgentStatus{
				ClusterAgent: &v2alpha1.DeploymentStatus{
					Status: "running",
				},
				RemoteConfigConfiguration: &v2alpha1.RemoteConfigConfiguration{
					Features: &v2alpha1.DatadogFeatures{
						CSPM: &v2alpha1.CSPMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
			ddai: &v1alpha1.DatadogAgentInternal{
				Status: v1alpha1.DatadogAgentInternalStatus{
					Agent: &v2alpha1.DaemonSetStatus{
						Status: "running",
					},
				},
			},
			want: &v1alpha1.DatadogAgentInternal{
				Status: v1alpha1.DatadogAgentInternalStatus{
					Agent: &v2alpha1.DaemonSetStatus{
						Status: "running",
					},
					RemoteConfigConfiguration: &v2alpha1.RemoteConfigConfiguration{
						Features: &v2alpha1.DatadogFeatures{
							CSPM: &v2alpha1.CSPMFeatureConfig{
								Enabled: apiutils.NewBoolPointer(true),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client: fake.NewClientBuilder().WithScheme(sch).Build(),
			}
			r.addRemoteConfigStatusToDDAIStatus(&tt.ddaStatus, tt.ddai)
			assert.Equal(t, tt.want, tt.ddai)
		})
	}
}

func Test_cleanUpUnusedDDAIs(t *testing.T) {
	sch := agenttestutils.TestScheme()
	ctx := context.Background()
	logger := logf.Log.WithName("test_cleanUpUnusedDDAIs")

	testCases := []struct {
		name          string
		existingDDAIs []client.Object
		validDDAIs    []*v1alpha1.DatadogAgentInternal
		wantDDAIs     *v1alpha1.DatadogAgentInternalList
	}{
		{
			name:          "empty lists",
			existingDDAIs: []client.Object{},
			validDDAIs:    []*v1alpha1.DatadogAgentInternal{},
			wantDDAIs: &v1alpha1.DatadogAgentInternalList{
				Items: []v1alpha1.DatadogAgentInternal{},
			},
		},
		{
			name: "no ddais to delete",
			existingDDAIs: []client.Object{
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "default",
					},
				},
			},
			validDDAIs: []*v1alpha1.DatadogAgentInternal{
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "default",
					},
				},
			},
			wantDDAIs: &v1alpha1.DatadogAgentInternalList{
				Items: []v1alpha1.DatadogAgentInternal{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-agent",
							Namespace:       "default",
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name: "multiple ddais to delete",
			existingDDAIs: []client.Object{
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "default",
					},
				},
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-bar-agent",
						Namespace: "default",
					},
				},
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-baz-agent",
						Namespace: "default",
					},
				},
			},
			validDDAIs: []*v1alpha1.DatadogAgentInternal{
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "default",
					},
				},
			},
			wantDDAIs: &v1alpha1.DatadogAgentInternalList{
				Items: []v1alpha1.DatadogAgentInternal{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-agent",
							Namespace:       "default",
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name: "multiple ddais to delete with different namespace",
			existingDDAIs: []client.Object{
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "default",
					},
				},
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "foo",
					},
				},
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "bar",
					},
				},
			},
			validDDAIs: []*v1alpha1.DatadogAgentInternal{
				&v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "foo",
					},
				},
			},
			wantDDAIs: &v1alpha1.DatadogAgentInternalList{
				Items: []v1alpha1.DatadogAgentInternal{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-agent",
							Namespace:       "foo",
							ResourceVersion: "999",
						},
					},
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.existingDDAIs...).Build()

			r := &Reconciler{
				client: fakeClient,
				log:    logger,
			}

			err := r.cleanUpUnusedDDAIs(ctx, tt.validDDAIs)
			assert.NoError(t, err)

			ddaiList := &v1alpha1.DatadogAgentInternalList{}
			err = fakeClient.List(ctx, ddaiList)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantDDAIs, ddaiList)
		})
	}
}
