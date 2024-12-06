package datadogagent

import (
	"context"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/crds/datadoghq/common"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_getDeploymentNameFromCCR(t *testing.T) {
	testCases := []struct {
		name               string
		dda                *datadoghqv2alpha1.DatadogAgent
		wantDeploymentName string
	}{
		{
			name: "ccr no override",
			dda: &datadoghqv2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantDeploymentName: "foo-cluster-checks-runner",
		},
		{
			name: "ccr override with no name override",
			dda: &datadoghqv2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.ClusterAgentComponentName: {
							Replicas: apiutils.NewInt32Pointer(10),
						},
					},
				},
			},
			wantDeploymentName: "foo-cluster-checks-runner",
		},
		{
			name: "ccr override with name override",
			dda: &datadoghqv2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.ClusterChecksRunnerComponentName: {
							Name:     apiutils.NewStringPointer("bar"),
							Replicas: apiutils.NewInt32Pointer(10),
						},
					},
				},
			},
			wantDeploymentName: "bar",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			deploymentName := getDeploymentNameFromCCR(tt.dda)
			assert.Equal(t, tt.wantDeploymentName, deploymentName)
		})
	}
}

func Test_cleanupOldCCRDeployments(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	ctx := context.Background()

	testCases := []struct {
		name           string
		description    string
		existingAgents []client.Object
		wantDeployment *appsv1.DeploymentList
	}{
		{
			name:        "no unused CCR deployments",
			description: "DCA deployment `dda-foo-cluster-checks-runner` should not be deleted",
			existingAgents: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dda-foo-cluster-checks-runner",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: datadoghqv2alpha1.DefaultClusterChecksRunnerResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
						},
					},
				},
			},
			wantDeployment: &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-cluster-checks-runner",
							ResourceVersion: "999",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: datadoghqv2alpha1.DefaultClusterChecksRunnerResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							},
						},
					},
				},
			},
		},
		{
			name:        "multiple unused CCR deployments",
			description: "all deployments except `dda-foo-cluster-checks-runner` should be deleted",
			existingAgents: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dda-foo-cluster-checks-runner",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: datadoghqv2alpha1.DefaultClusterChecksRunnerResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-ccr",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: datadoghqv2alpha1.DefaultClusterChecksRunnerResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bar-ccr",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: datadoghqv2alpha1.DefaultClusterChecksRunnerResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
						},
					},
				},
			},
			wantDeployment: &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-cluster-checks-runner",
							ResourceVersion: "999",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: datadoghqv2alpha1.DefaultClusterChecksRunnerResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							},
						},
					},
				},
			},
		},
		{
			name:        "deployments are not created by the operator (do not have the expected labels) and should not be removed",
			description: "No deployments should be deleted",
			existingAgents: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-cluster-checks-runner",
						Namespace: "ns-1",
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-test-one-cluster-checks-runner",
						Namespace: "ns-1",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-test-two-cluster-checks-runner",
						Namespace: "ns-1",
						Labels: map[string]string{
							"bar": "foo",
						},
					},
				},
			},
			wantDeployment: &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-test-one-cluster-checks-runner",
							Namespace: "ns-1",
							Labels: map[string]string{
								"foo": "bar",
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-test-two-cluster-checks-runner",
							Namespace: "ns-1",
							Labels: map[string]string{
								"bar": "foo",
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-cluster-checks-runner",
							Namespace:       "ns-1",
							ResourceVersion: "999",
						},
					},
				},
			},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.existingAgents...).Build()
			logger := logf.Log.WithName("Test_cleanupOldCCRDeployments")
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "Test_cleanupOldCCRDeployments"})

			r := &Reconciler{
				client:   fakeClient,
				log:      logger,
				recorder: recorder,
			}

			dda := datadoghqv2alpha1.DatadogAgent{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DatadogAgent",
					APIVersion: "datadoghq.com/v2alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dda-foo",
					Namespace: "ns-1",
				},
			}
			ddaStatus := datadoghqv2alpha1.DatadogAgentStatus{}

			err := r.cleanupOldCCRDeployments(ctx, logger, &dda, &ddaStatus)
			assert.NoError(t, err)

			deploymentList := &appsv1.DeploymentList{}

			err = fakeClient.List(ctx, deploymentList)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantDeployment, deploymentList)
		})
	}
}
