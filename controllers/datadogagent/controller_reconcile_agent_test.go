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
const gcpCosContainerdProvider = kubernetes.GCPCloudProvider + "-" + kubernetes.GCPCosContainerdProviderValue

func Test_generateNodeAffinity(t *testing.T) {

	type args struct {
		affinity *corev1.Affinity
		provider string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "nil affinity, default provider",
			args: args{
				affinity: nil,
				provider: defaultProvider,
			},
		},
		{
			name: "nil affinity, gcp cos containerd provider",
			args: args{
				affinity: nil,
				provider: gcpCosContainerdProvider,
			},
		},
		{
			name: "existing affinity, but empty, default provider",
			args: args{
				affinity: &corev1.Affinity{},
				provider: defaultProvider,
			},
		},
		{
			name: "existing affinity, but empty, gcp cos containerd provider",
			args: args{
				affinity: &corev1.Affinity{},
				provider: gcpCosContainerdProvider,
			},
		},
		{
			name: "existing affinity, NodeAffinity empty, default provider",
			args: args{
				affinity: &corev1.Affinity{
					PodAffinity: &corev1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"foo": "bar",
									},
								},
								TopologyKey: "foo/bar",
							},
						},
					},
				},
				provider: defaultProvider,
			},
		},
		{
			name: "existing affinity, NodeAffinity empty, cos containerd provider",
			args: args{
				affinity: &corev1.Affinity{
					PodAffinity: &corev1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"foo": "bar",
									},
								},
								TopologyKey: "foo/bar",
							},
						},
					},
				},
				provider: gcpCosContainerdProvider,
			},
		},
		{
			name: "existing affinity, NodeAffinity filled, default provider",
			args: args{
				affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "foo",
											Operator: corev1.NodeSelectorOpDoesNotExist,
										},
									},
								},
							},
						},
					},
				},
				provider: defaultProvider,
			},
		},
		{
			name: "existing affinity, NodeAffinity filled, cos containerd provider",
			args: args{
				affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "foo",
											Operator: corev1.NodeSelectorOpDoesNotExist,
										},
									},
								},
							},
						},
					},
				},
				provider: gcpCosContainerdProvider,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := kubernetes.NewProviderStore(logf.Log.WithName("test_generateNodeAffinity"))
			r := &Reconciler{
				providerStore: &p,
			}
			existingProviders := map[string]struct{}{
				"gcp-cos": {},
				"default": {},
			}
			r.providerStore.Reset(existingProviders)

			actualAffinity := r.generateNodeAffinity(tt.args.provider, tt.args.affinity)
			na, pa, paa := getAffinityComponents(tt.args.affinity)
			wantedAffinity := generateWantedAffinity(tt.args.provider, na, pa, paa)
			assert.Equal(t, wantedAffinity, actualAffinity)
		})
	}

}

func generateWantedAffinity(provider string, na *corev1.NodeAffinity, pa *corev1.PodAffinity, paa *corev1.PodAntiAffinity) *corev1.Affinity {
	defaultNA := corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      kubernetes.GCPProviderLabel,
							Operator: corev1.NodeSelectorOpNotIn,
							Values:   []string{kubernetes.GCPCosProviderValue},
						},
					},
				},
			},
		},
	}
	if na != nil {
		defaultNA = *na
	}
	if provider == kubernetes.DefaultProvider {
		return &corev1.Affinity{
			NodeAffinity:    &defaultNA,
			PodAffinity:     pa,
			PodAntiAffinity: paa,
		}
	}

	key, value := kubernetes.GetProviderLabelKeyValue(provider)

	providerNA := corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      key,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{value},
						},
					},
				},
			},
		},
	}
	if na != nil {
		providerNA = *na
	}
	return &corev1.Affinity{
		NodeAffinity:    &providerNA,
		PodAffinity:     pa,
		PodAntiAffinity: paa,
	}

}

func getAffinityComponents(affinity *corev1.Affinity) (*corev1.NodeAffinity, *corev1.PodAffinity, *corev1.PodAntiAffinity) {
	if affinity == nil {
		return nil, nil, nil
	}
	return affinity.NodeAffinity, affinity.PodAffinity, affinity.PodAntiAffinity
}

func Test_updateProviderStore(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name              string
		nodes             []client.Object
		existingProviders map[string]struct{}
		wantedProviders   map[string]struct{}
	}{
		{
			name: "recompute all providers",
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gcp-cos-node",
						Labels: map[string]string{
							kubernetes.GCPProviderLabel: kubernetes.GCPCosProviderValue,
						},
					},
				},
			},
			existingProviders: map[string]struct{}{
				"gcp-cos_containerd": {},
				"default":            {},
			},
			wantedProviders: map[string]struct{}{
				"gcp-cos": {},
			},
		},
		{
			name:  "empty node list",
			nodes: []client.Object{},
			existingProviders: map[string]struct{}{
				"gcp-cos_containerd": {},
				"default":            {},
			},
			wantedProviders: map[string]struct{}{
				"gcp-cos_containerd": {},
				"default":            {},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithObjects(tt.nodes...).Build()
			logger := logf.Log.WithName("test_updateProviderStore")
			p := kubernetes.NewProviderStore(logger)
			r := &Reconciler{
				providerStore: &p,
				client:        fakeClient,
				log:           logger,
			}
			r.providerStore.Reset(tt.existingProviders)

			providerList, err := r.updateProviderStore(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantedProviders, providerList)
		})
	}
}

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
						Name: "gcp-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
						},
					},
				},
			},
			edsEnabled: false,
			existingProviders: map[string]struct{}{
				gcpCosContainerdProvider: {},
			},
			wantDS: &appsv1.DaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaemonSetList",
					APIVersion: "apps/v1",
				},
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gcp-cos-node",
							Labels: map[string]string{
								apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
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
						Name: "gcp-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
						},
					},
				},
			},
			edsEnabled: true,
			existingProviders: map[string]struct{}{
				gcpCosContainerdProvider: {},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExtendedDaemonSetList",
					APIVersion: "datadoghq.com/v1alpha1",
				},
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gcp-cos-node",
							Labels: map[string]string{
								apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
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
						Name: "gcp-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
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
						Name: "gcp-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
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
				gcpCosContainerdProvider: {},
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
				gcpCosContainerdProvider: {},
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
						Name: "gcp-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
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
							Name: "gcp-cos-node",
							Labels: map[string]string{
								apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
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
						Name: "gcp-cos-node",
						Labels: map[string]string{
							apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
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
							Name: "gcp-cos-node",
							Labels: map[string]string{
								apicommon.MD5AgentDeploymentProviderLabelKey: gcpCosContainerdProvider,
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
			p := kubernetes.NewProviderStore(logger)
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_createNewExtendedDaemonSet"})

			r := &Reconciler{
				providerStore: &p,
				client:        fakeClient,
				log:           logger,
				recorder:      recorder,
				options: ReconcilerOptions{
					ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
						Enabled: tt.edsEnabled,
					},
				},
			}
			r.providerStore.Reset(tt.existingProviders)

			dda := datadoghqv2alpha1.DatadogAgent{}
			ddaStatus := datadoghqv2alpha1.DatadogAgentStatus{}

			err := r.cleanupDaemonSetsForProvidersThatNoLongerApply(ctx, &dda, &ddaStatus)
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

func Test_removeDaemonSetStatus(t *testing.T) {
	testCases := []struct {
		name       string
		ddaStatus  *datadoghqv2alpha1.DatadogAgentStatus
		dsName     string
		wantStatus *datadoghqv2alpha1.DatadogAgentStatus
	}{
		{
			name: "no status to delete",
			ddaStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				Agent: []*common.DaemonSetStatus{
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
				Agent: []*common.DaemonSetStatus{
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
				Agent: []*common.DaemonSetStatus{
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
				Agent: []*common.DaemonSetStatus{
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
				Agent: []*common.DaemonSetStatus{
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
				Agent: []*common.DaemonSetStatus{},
			},
		},
		{
			name: "agent status is empty",
			ddaStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				Agent: []*common.DaemonSetStatus{},
			},
			dsName: "bar",
			wantStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				Agent: []*common.DaemonSetStatus{},
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
			removeDaemonSetStatus(tt.ddaStatus, tt.dsName)
			assert.Equal(t, tt.wantStatus, tt.ddaStatus)
		})
	}
}
