package datadogagent

import (
	"context"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/constants"
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

func Test_getDeploymentNameFromDCA(t *testing.T) {
	testCases := []struct {
		name               string
		dda                *datadoghqv2alpha1.DatadogAgent
		wantDeploymentName string
	}{
		{
			name: "dca no override",
			dda: &datadoghqv2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantDeploymentName: "foo-cluster-agent",
		},
		{
			name: "dca override with no name override",
			dda: &datadoghqv2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.ClusterAgentComponentName: {
							Replicas: apiutils.NewPointer[int32](10),
						},
					},
				},
			},
			wantDeploymentName: "foo-cluster-agent",
		},
		{
			name: "dca override with name override",
			dda: &datadoghqv2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.ClusterAgentComponentName: {
							Name:     apiutils.NewPointer("bar"),
							Replicas: apiutils.NewPointer[int32](10),
						},
					},
				},
			},
			wantDeploymentName: "bar",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			deploymentName := getDeploymentNameFromDCA(tt.dda)
			assert.Equal(t, tt.wantDeploymentName, deploymentName)
		})
	}
}

func Test_cleanupOldDCADeployments(t *testing.T) {
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
			name:        "no unused DCA deployments",
			description: "DCA deployment `dda-foo-cluster-agent` should not be deleted",
			existingAgents: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dda-foo-cluster-agent",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
						},
					},
				},
			},
			wantDeployment: &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-cluster-agent",
							ResourceVersion: "999",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							},
						},
					},
				},
			},
		},
		{
			name:        "multiple unused DCA deployments",
			description: "all deployments except `dda-foo-cluster-agent` should be deleted",
			existingAgents: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dda-foo-cluster-agent",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-dca",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "bar-dca",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
						},
					},
				},
			},
			wantDeployment: &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-cluster-agent",
							ResourceVersion: "999",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
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
						Name:      "dda-foo-cluster-agent",
						Namespace: "ns-1",
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-test-one-cluster-agent",
						Namespace: "ns-1",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-test-two-cluster-agent",
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
							Name:      "datadog-test-one-cluster-agent",
							Namespace: "ns-1",
							Labels: map[string]string{
								"foo": "bar",
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-test-two-cluster-agent",
							Namespace: "ns-1",
							Labels: map[string]string{
								"bar": "foo",
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-cluster-agent",
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
			logger := logf.Log.WithName("Test_cleanupOldDCADeployments")
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "Test_cleanupOldDCADeployments"})

			r := &Reconciler{
				client:   fakeClient,
				log:      logger,
				recorder: recorder,
			}
			storeOptions := &store.StoreOptions{
				SupportCilium: r.options.SupportCilium,
				PlatformInfo:  r.platformInfo,
				Logger:        logger,
				Scheme:        r.scheme,
			}
			instance := &datadoghqv2alpha1.DatadogAgent{}
			defaults.DefaultDatadogAgent(instance.DeepCopy())
			depsStore := store.NewStore(instance, storeOptions)
			resourcesManager := feature.NewResourceManagers(depsStore)

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

			err := r.cleanupOldDCADeployments(ctx, logger, &dda, resourcesManager, &ddaStatus)
			assert.NoError(t, err)

			deploymentList := &appsv1.DeploymentList{}

			err = fakeClient.List(ctx, deploymentList)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantDeployment, deploymentList)
		})
	}
}
