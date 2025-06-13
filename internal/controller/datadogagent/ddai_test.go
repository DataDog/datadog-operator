package datadogagent

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
