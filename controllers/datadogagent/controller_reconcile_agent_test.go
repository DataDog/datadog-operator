package datadogagent

import (
	"context"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const defaultProvider = kubernetes.DefaultProvider
const gkeCosProvider = kubernetes.GKECloudProvider + "-" + kubernetes.GKECosType

func Test_cleanupDaemonSetsForProvidersThatNoLongerApply(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = edsdatadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	testCases := []struct {
		name              string
		agents            []client.Object
		edsEnabled        bool
		existingProviders map[string]struct{}
		wantDS            *appsv1.DaemonSetList
		wantEDS           *edsdatadoghqv1alpha1.ExtendedDaemonSetList
	}{
		{
			name: "no unused ds",
			agents: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gke-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
						},
					},
				},
			},
			edsEnabled: false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			wantDS: &appsv1.DaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSetList",
					APIVersion: "apps/v1",
				},
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gke-cos-node",
							Labels: map[string]string{
								apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							},
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name: "no unused eds",
			agents: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gke-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
						},
					},
				},
			},
			edsEnabled: true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExtendedDaemonSetList",
					APIVersion: "datadoghq.com/v1alpha1",
				},
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gke-cos-node",
							Labels: map[string]string{
								apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							},
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name: "unused ds",
			agents: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gke-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
						},
					},
				},
			},
			edsEnabled: false,
			existingProviders: map[string]struct{}{
				defaultProvider: {},
			},
			wantDS: &appsv1.DaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSetList",
					APIVersion: "apps/v1",
				},
				Items: []appsv1.DaemonSet{},
			},
		},
		{
			name: "unused eds",
			agents: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gke-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
						},
					},
				},
			},
			edsEnabled: true,
			existingProviders: map[string]struct{}{
				defaultProvider: {},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExtendedDaemonSetList",
					APIVersion: "datadoghq.com/v1alpha1",
				},
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{},
			},
		},
		{
			name:       "no ds to delete",
			agents:     []client.Object{},
			edsEnabled: false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			wantDS: &appsv1.DaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSetList",
					APIVersion: "apps/v1",
				},
				Items: nil,
			},
		},
		{
			name:       "no eds to delete",
			agents:     []client.Object{},
			edsEnabled: true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExtendedDaemonSetList",
					APIVersion: "datadoghq.com/v1alpha1",
				},
				Items: nil,
			},
		},
		{
			name: "no providers for ds",
			agents: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gke-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
						},
					},
				},
			},
			edsEnabled:        false,
			existingProviders: map[string]struct{}{},
			wantDS: &appsv1.DaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSetList",
					APIVersion: "apps/v1",
				},
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gke-cos-node",
							Labels: map[string]string{
								apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							},
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name: "no providers for eds",
			agents: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gke-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
						},
					},
				},
			},
			edsEnabled:        true,
			existingProviders: map[string]struct{}{},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExtendedDaemonSetList",
					APIVersion: "datadoghq.com/v1alpha1",
				},
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gke-cos-node",
							Labels: map[string]string{
								apicommon.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							},
							ResourceVersion: "999",
						},
					},
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.agents...).Build()
			logger := logf.Log.WithName("test_cleanupDaemonSetsForProvidersThatNoLongerApply")
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_createNewExtendedDaemonSet"})

			r := &Reconciler{
				client:   fakeClient,
				log:      logger,
				recorder: recorder,
				options: ReconcilerOptions{
					ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
						Enabled: tt.edsEnabled,
					},
				},
			}

			dda := datadoghqv2alpha1.DatadogAgent{}
			ddaStatus := datadoghqv2alpha1.DatadogAgentStatus{}

			err := r.cleanupDaemonSetsForProvidersThatNoLongerApply(ctx, &dda, &ddaStatus, tt.existingProviders)
			assert.NoError(t, err)

			kind := "daemonsets"
			if tt.edsEnabled {
				kind = "extendeddaemonsets"
			}
			objList := getObjectListFromKind(kind)
			err = fakeClient.List(ctx, objList)
			assert.NoError(t, err)

			if tt.edsEnabled {
				assert.Equal(t, tt.wantEDS, objList)
			} else {
				assert.Equal(t, tt.wantDS, objList)
			}
		})
	}
}

func getObjectListFromKind(kind string) client.ObjectList {
	switch kind {
	case "daemonsets":
		return &appsv1.DaemonSetList{}
	case "extendeddaemonsets":
		return &edsdatadoghqv1alpha1.ExtendedDaemonSetList{}
	}
	return nil
}

func Test_removeStaleStatus(t *testing.T) {
	testCases := []struct {
		name       string
		ddaStatus  *datadoghqv2alpha1.DatadogAgentStatus
		dsName     string
		wantStatus *datadoghqv2alpha1.DatadogAgentStatus
	}{
		{
			name: "no status to delete",
			ddaStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*common.DaemonSetStatus{
					{
						Desired:       1,
						Current:       1,
						Ready:         1,
						Available:     1,
						DaemonsetName: "foo",
					},
				},
			},
			dsName: "bar",
			wantStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*common.DaemonSetStatus{
					{
						Desired:       1,
						Current:       1,
						Ready:         1,
						Available:     1,
						DaemonsetName: "foo",
					},
				},
			},
		},
		{
			name: "delete status",
			ddaStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*common.DaemonSetStatus{
					{
						Desired:       1,
						Current:       1,
						Ready:         1,
						Available:     1,
						DaemonsetName: "foo",
					},
					{
						Desired:       2,
						Current:       2,
						Ready:         1,
						Available:     1,
						UpToDate:      1,
						DaemonsetName: "bar",
					},
				},
			},
			dsName: "bar",
			wantStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*common.DaemonSetStatus{
					{
						Desired:       1,
						Current:       1,
						Ready:         1,
						Available:     1,
						DaemonsetName: "foo",
					},
				},
			},
		},
		{
			name: "delete only status",
			ddaStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*common.DaemonSetStatus{
					{
						Desired:       2,
						Current:       2,
						Ready:         1,
						Available:     1,
						UpToDate:      1,
						DaemonsetName: "bar",
					},
				},
			},
			dsName: "bar",
			wantStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*common.DaemonSetStatus{},
			},
		},
		{
			name: "agent status is empty",
			ddaStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*common.DaemonSetStatus{},
			},
			dsName: "bar",
			wantStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*common.DaemonSetStatus{},
			},
		},
		{
			name:       "dda status is empty",
			ddaStatus:  &datadoghqv2alpha1.DatadogAgentStatus{},
			dsName:     "bar",
			wantStatus: &datadoghqv2alpha1.DatadogAgentStatus{},
		},
		{
			name:       "status is nil",
			ddaStatus:  nil,
			dsName:     "bar",
			wantStatus: nil,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			removeStaleStatus(tt.ddaStatus, tt.dsName)
			assert.Equal(t, tt.wantStatus, tt.ddaStatus)
		})
	}
}
