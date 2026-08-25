package datadogagentinternal

import (
	"context"
	"testing"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

const defaultProvider = kubernetes.DefaultProvider
const gkeCosProvider = kubernetes.GKECloudProvider + "-" + kubernetes.GKECosType

func TestReconcileV2Agent_EDSSingleContainerStrategySetsDataPlaneFlags(t *testing.T) {
	ctx := context.Background()
	scheme := newMigrationTestScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := NewReconciler(
		ReconcilerOptions{
			ExtendedDaemonsetOptions:     agent.ExtendedDaemonsetOptions{Enabled: true},
			DefaultDataPlaneLinuxEnabled: true,
		},
		client,
		kubernetes.PlatformInfo{},
		scheme,
		record.NewFakeRecorder(10),
		nil,
	)
	ddai := testutils.NewDefaultDatadogAgentInternalBuilder().
		WithCredentials("api-key", "").
		WithSingleContainerStrategy(true).
		Build()
	ddai.Name = "datadog"
	ddai.Namespace = "default"

	configuredFeatures, enabledFeatures, _, _ := feature.BuildFeatures(
		ddai,
		&ddai.Spec,
		ddai.Status.RemoteConfigConfiguration,
		reconciler.reconcilerOptionsToFeatureOptions(ctx, ddai),
	)
	var dataPlaneFeature feature.Feature
	for _, candidate := range append(configuredFeatures, enabledFeatures...) {
		if candidate.ID() == feature.DataPlaneIDType {
			dataPlaneFeature = candidate
			break
		}
	}
	require.NotNil(t, dataPlaneFeature)

	requiredComponents := feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			IsRequired: ptr.To(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.UnprivilegedSingleAgentContainerName,
			},
		},
	}
	_, resourceManagers := reconciler.setupDependencies(ctx, ddai)
	_, err := reconciler.reconcileV2Agent(
		ctx,
		requiredComponents,
		[]feature.Feature{dataPlaneFeature},
		ddai,
		resourceManagers,
		&datadoghqv1alpha1.DatadogAgentInternalStatus{},
		"",
	)
	require.NoError(t, err)

	eds := &edsv1alpha1.ExtendedDaemonSet{}
	require.NoError(t, client.Get(ctx, types.NamespacedName{Namespace: ddai.Namespace, Name: "datadog-agent"}, eds))
	require.Len(t, eds.Spec.Template.Spec.Containers, 1)
	agentContainer := eds.Spec.Template.Spec.Containers[0]
	assert.Equal(t, string(apicommon.UnprivilegedSingleAgentContainerName), agentContainer.Name)

	envVars := make(map[string]string, len(agentContainer.Env))
	for _, envVar := range agentContainer.Env {
		envVars[envVar.Name] = envVar.Value
	}
	assert.Equal(t, "true", envVars[common.DDDataPlaneEnabled])
	assert.Equal(t, "true", envVars[common.DDDataPlaneDogstatsdEnabled])
	assert.Equal(t, "true", envVars[common.DDDataPlaneRemoteAgentEnabled])
	assert.Equal(t, "true", envVars[common.DDDataPlaneUseNewConfigStreamEndpoint])
}

// func Test_getValidDaemonSetNames(t *testing.T) {
// 	testCases := []struct {
// 		name       string
// 		dsName     string
// 		edsEnabled bool
// 		wantDS     map[string]struct{}
// 		wantEDS    map[string]struct{}
// 	}{
// 		{
// 			name:       "introspection disabled, profiles disabled, eds disabled",
// 			dsName:     "foo",
// 			edsEnabled: false,
// 			wantDS:     map[string]struct{}{"foo": {}},
// 			wantEDS:    map[string]struct{}{},
// 		},
// 		{
// 			name:       "introspection disabled, profiles disabled, eds enabled",
// 			dsName:     "foo",
// 			edsEnabled: true,
// 			wantDS:     map[string]struct{}{},
// 			wantEDS:    map[string]struct{}{"foo": {}},
// 		},
// 	}

// 	for _, tt := range testCases {
// 		t.Run(tt.name, func(t *testing.T) {
// 			r := &Reconciler{
// 				options: ReconcilerOptions{
// 					ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
// 						Enabled: tt.edsEnabled,
// 					},
// 				},
// 			}

// 			// Provide empty maps/slices for removed fields
// 			validDSNames, validEDSNames := r.getValidDaemonSetNames(tt.dsName)
// 			assert.Equal(t, tt.wantDS, validDSNames)
// 			assert.Equal(t, tt.wantEDS, validEDSNames)
// 		})
// 	}
// }

func Test_getDaemonSetNameFromDatadogAgent(t *testing.T) {
	testCases := []struct {
		name       string
		ddai       *datadoghqv1alpha1.DatadogAgentInternal
		wantDSName string
	}{
		{
			name: "no node override",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantDSName: "foo-agent",
		},
		{
			name: "node override with no name override",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.NodeAgentComponentName: {
							Replicas: ptr.To[int32](10),
						},
					},
				},
			},
			wantDSName: "foo-agent",
		},
		{
			name: "node override with name override",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.NodeAgentComponentName: {
							Name:     ptr.To("bar"),
							Replicas: ptr.To[int32](10),
						},
					},
				},
			},
			wantDSName: "bar",
		},
		{
			name: "dca override with name override",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.ClusterAgentComponentName: {
							Name:     ptr.To("bar"),
							Replicas: ptr.To[int32](10),
						},
					},
				},
			},
			wantDSName: "foo-agent",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			dsName := component.GetDaemonSetNameFromDatadogAgent(tt.ddai.GetObjectMeta(), &tt.ddai.Spec)
			assert.Equal(t, tt.wantDSName, dsName)
		})
	}
}

func Test_isDDAILabeledWithProfile(t *testing.T) {
	testCases := []struct {
		name string
		ddai *datadoghqv1alpha1.DatadogAgentInternal
		want bool
	}{
		{
			name: "no profile label",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			want: false,
		},
		{
			name: "profile label",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						constants.ProfileLabelKey: "foo",
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got := isDDAILabeledWithProfile(tt.ddai)
			assert.Equal(t, tt.want, got)
		})
	}
}

// func Test_cleanupExtraneousDaemonSets(t *testing.T) {
// 	sch := runtime.NewScheme()
// 	_ = scheme.AddToScheme(sch)
// 	_ = edsdatadoghqv1alpha1.AddToScheme(sch)
// 	ctx := context.Background()

// 	testCases := []struct {
// 		name           string
// 		description    string
// 		existingAgents []client.Object
// 		edsEnabled     bool
// 		wantDS         *appsv1.DaemonSetList
// 		wantEDS        *edsdatadoghqv1alpha1.ExtendedDaemonSetList
// 	}{
// 		{
// 			name:        "no unused ds, introspection disabled, profiles disabled",
// 			description: "DS `dda-foo-agent` should not be deleted",
// 			existingAgents: []client.Object{
// 				&appsv1.DaemonSet{
// 					ObjectMeta: metav1.ObjectMeta{
// 						Name: "dda-foo-agent",
// 						Labels: map[string]string{
// 							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
// 							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
// 							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
// 						},
// 					},
// 				},
// 			},
// 			edsEnabled: false,
// 			wantDS: &appsv1.DaemonSetList{
// 				Items: []appsv1.DaemonSet{
// 					{
// 						ObjectMeta: metav1.ObjectMeta{
// 							Name:            "dda-foo-agent",
// 							ResourceVersion: "999",
// 							Labels: map[string]string{
// 								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
// 								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
// 								kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
// 							},
// 						},
// 					},
// 				},
// 			},
// 			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
// 				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{},
// 			},
// 		},
// 		{
// 			name:        "no unused eds, introspection disabled, profiles disabled",
// 			description: "EDS `dda-foo-agent` should not be deleted",
// 			existingAgents: []client.Object{
// 				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
// 					ObjectMeta: metav1.ObjectMeta{
// 						Name: "dda-foo-agent",
// 						Labels: map[string]string{
// 							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
// 							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
// 							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
// 						},
// 					},
// 				},
// 			},
// 			edsEnabled: true,
// 			wantDS: &appsv1.DaemonSetList{
// 				Items: []appsv1.DaemonSet{},
// 			},
// 			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
// 				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
// 					{
// 						ObjectMeta: metav1.ObjectMeta{
// 							Name:            "dda-foo-agent",
// 							ResourceVersion: "999",
// 							Labels: map[string]string{
// 								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
// 								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
// 								kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
// 							},
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}

// 	for _, tt := range testCases {
// 		t.Run(tt.name, func(t *testing.T) {
// 			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.existingAgents...).Build()
// 			logger := logf.Log.WithName("test_cleanupExtraneousDaemonSets")
// 			eventBroadcaster := record.NewBroadcaster()
// 			recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "test_cleanupExtraneousDaemonSets"})

// 			r := &Reconciler{
// 				client:   fakeClient,
// 				log:      logger,
// 				recorder: recorder,
// 				options: ReconcilerOptions{
// 					ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
// 						Enabled: tt.edsEnabled,
// 					},
// 				},
// 			}

// 			dda := datadoghqv1alpha1.DatadogAgentInternal{
// 				TypeMeta: metav1.TypeMeta{
// 					Kind:       "DatadogAgent",
// 					APIVersion: "datadoghq.com/v2alpha1",
// 				},
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "dda-foo",
// 					Namespace: "ns-1",
// 				},
// 			}
// 			ddaStatus := datadoghqv1alpha1.DatadogAgentInternalStatus{}

// 			err := r.cleanupExtraneousDaemonSets(ctx, logger, &dda, &ddaStatus)
// 			assert.NoError(t, err)

// 			dsList := &appsv1.DaemonSetList{}
// 			tedsList := &edsdatadoghqv1alpha1.ExtendedDaemonSetList{}

// 			err = fakeClient.List(ctx, dsList)
// 			assert.NoError(t, err)
// 			err = fakeClient.List(ctx, tedsList)
// 			assert.NoError(t, err)

// 			assert.Equal(t, tt.wantDS, dsList)
// 			assert.Equal(t, tt.wantEDS, tedsList)
// 		})
// 	}
// }
