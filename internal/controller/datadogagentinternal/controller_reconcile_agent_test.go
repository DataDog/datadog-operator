package datadogagentinternal

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	pkgtestutils "github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const defaultProvider = kubernetes.DefaultProvider
const gkeCosProvider = kubernetes.GKECloudProvider + "-" + kubernetes.GKECosType

func TestReconcileV2AgentCreatesPreparedSurgeDaemonSet(t *testing.T) {
	r, ddai := newPreparedRolloutReconciler(t, false)
	status := &datadoghqv1alpha1.DatadogAgentInternalStatus{}

	result, err := r.reconcileV2Agent(
		context.Background(),
		preparedRolloutRequiredComponents(),
		nil,
		ddai,
		feature.NewResourceManagers(store.NewStore(ddai, nil)),
		status,
		defaultProvider,
	)

	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
	daemonSets := &appsv1.DaemonSetList{}
	require.NoError(t, r.client.List(context.Background(), daemonSets))
	require.Len(t, daemonSets.Items, 1)
	ds := &daemonSets.Items[0]
	assert.Equal(t, preparedRolloutModeV1, ds.Spec.Template.Annotations[preparedRolloutModeAnnotation])
	require.NotNil(t, ds.Spec.UpdateStrategy.RollingUpdate)
	assert.Equal(t, intstr.FromInt(1), *ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge)
	assert.Equal(t, intstr.FromInt(0), *ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
}

func newPreparedRolloutReconciler(t *testing.T, hostNetwork bool) (*Reconciler, *datadoghqv1alpha1.DatadogAgentInternal) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&appsv1.DaemonSet{}).Build()
	one := intstr.FromInt(1)
	ddai := pkgtestutils.NewDatadogAgentInternal("datadog-agent", "agent", nil)
	ddai.UID = "ddai-uid"
	ddai.Annotations = map[string]string{preparedRolloutModeAnnotation: preparedRolloutModeV1}
	ddai.Spec.Features = &datadoghqv2alpha1.DatadogFeatures{}
	ddai.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
		datadoghqv2alpha1.NodeAgentComponentName: {
			HostNetwork: ptr.To(hostNetwork),
			UpdateStrategy: &apicommon.UpdateStrategy{
				Type: string(appsv1.RollingUpdateDaemonSetStrategyType),
				RollingUpdate: &apicommon.RollingUpdate{
					MaxSurge:       ptr.To(one),
					MaxUnavailable: ptr.To(one),
				},
			},
		},
	}
	defaults.DefaultDatadogAgentSpec(&ddai.Spec)
	return &Reconciler{
		client:    c,
		apiReader: c,
		scheme:    scheme,
		recorder:  record.NewFakeRecorder(10),
	}, ddai
}

func preparedRolloutRequiredComponents() feature.RequiredComponents {
	return feature.RequiredComponents{Agent: feature.RequiredComponent{
		IsRequired: ptr.To(true),
		Containers: []apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
			apicommon.TraceAgentContainerName,
		},
	}}
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
