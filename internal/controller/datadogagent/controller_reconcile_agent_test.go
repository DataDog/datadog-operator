package datadogagent

import (
	"context"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const defaultProvider = kubernetes.DefaultProvider
const gkeCosProvider = kubernetes.GKECloudProvider + "-" + kubernetes.GKECosType

func Test_getValidDaemonSetNames(t *testing.T) {
	testCases := []struct {
		name                 string
		dsName               string
		introspectionEnabled bool
		profilesEnabled      bool
		edsEnabled           bool
		existingProviders    map[string]struct{}
		existingProfiles     []v1alpha1.DatadogAgentProfile
		wantDS               map[string]struct{}
		wantEDS              map[string]struct{}
	}{
		{
			name:                 "introspection disabled, profiles disabled, eds disabled",
			dsName:               "foo",
			introspectionEnabled: false,
			profilesEnabled:      false,
			edsEnabled:           false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo": {}},
			wantEDS:          map[string]struct{}{},
		},
		{
			name:                 "introspection disabled, profiles disabled, eds enabled",
			dsName:               "foo",
			introspectionEnabled: false,
			profilesEnabled:      false,
			edsEnabled:           true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{},
			wantEDS:          map[string]struct{}{"foo": {}},
		},
		{
			name:                 "introspection enabled, profiles disabled, eds disabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      false,
			edsEnabled:           false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo-gke-cos": {}},
			wantEDS:          map[string]struct{}{},
		},
		{
			name:                 "introspection enabled, profiles disabled, eds enabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      false,
			edsEnabled:           true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{},
			wantEDS:          map[string]struct{}{"foo-gke-cos": {}},
		},
		{
			name:                 "introspection enabled, profiles enabled, eds disabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           false,
			existingProviders: map[string]struct{}{
				gkeCosProvider:  {},
				defaultProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS: map[string]struct{}{
				"foo-default": {},
				"foo-gke-cos": {},
				"datadog-agent-with-profile-ns-1-profile-1-default": {},
				"datadog-agent-with-profile-ns-1-profile-1-gke-cos": {},
			},
			wantEDS: map[string]struct{}{},
		},
		{
			name:                 "introspection enabled, profiles enabled, eds enabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           true,
			existingProviders: map[string]struct{}{
				gkeCosProvider:  {},
				defaultProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS: map[string]struct{}{
				"datadog-agent-with-profile-ns-1-profile-1-default": {},
				"datadog-agent-with-profile-ns-1-profile-1-gke-cos": {},
			},
			wantEDS: map[string]struct{}{
				"foo-default": {},
				"foo-gke-cos": {},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				options: ReconcilerOptions{
					IntrospectionEnabled:       tt.introspectionEnabled,
					DatadogAgentProfileEnabled: tt.profilesEnabled,
					ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
						Enabled: tt.edsEnabled,
					},
				},
			}

			validDSNames, validEDSNames := r.getValidDaemonSetNames(tt.dsName, tt.existingProviders, tt.existingProfiles)
			assert.Equal(t, tt.wantDS, validDSNames)
			assert.Equal(t, tt.wantEDS, validEDSNames)
		})
	}
}

func Test_getDaemonSetNameFromDatadogAgent(t *testing.T) {
	testCases := []struct {
		name       string
		dda        *datadoghqv2alpha1.DatadogAgent
		wantDSName string
	}{
		{
			name: "no node override",
			dda: &datadoghqv2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantDSName: "foo-agent",
		},
		{
			name: "node override with no name override",
			dda: &datadoghqv2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.NodeAgentComponentName: {
							Replicas: apiutils.NewInt32Pointer(10),
						},
					},
				},
			},
			wantDSName: "foo-agent",
		},
		{
			name: "node override with name override",
			dda: &datadoghqv2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.NodeAgentComponentName: {
							Name:     apiutils.NewStringPointer("bar"),
							Replicas: apiutils.NewInt32Pointer(10),
						},
					},
				},
			},
			wantDSName: "bar",
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
							Name:     apiutils.NewStringPointer("bar"),
							Replicas: apiutils.NewInt32Pointer(10),
						},
					},
				},
			},
			wantDSName: "foo-agent",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			dsName := component.GetDaemonSetNameFromDatadogAgent(tt.dda)
			assert.Equal(t, tt.wantDSName, dsName)
		})
	}
}

func Test_cleanupExtraneousDaemonSets(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = edsdatadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	testCases := []struct {
		name                 string
		description          string
		existingAgents       []client.Object
		introspectionEnabled bool
		profilesEnabled      bool
		edsEnabled           bool
		providerList         map[string]struct{}
		profiles             []v1alpha1.DatadogAgentProfile
		wantDS               *appsv1.DaemonSetList
		wantEDS              *edsdatadoghqv1alpha1.ExtendedDaemonSetList
	}{
		{
			name:        "no unused ds, introspection disabled, profiles disabled",
			description: "DS `dda-foo-agent` should not be deleted",
			existingAgents: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dda-foo-agent",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: false,
			profilesEnabled:      false,
			edsEnabled:           false,
			providerList:         map[string]struct{}{},
			profiles:             []v1alpha1.DatadogAgentProfile{},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-agent",
							ResourceVersion: "999",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
							},
						},
					},
				},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{},
		},
		{
			name:        "no unused eds, introspection disabled, profiles disabled",
			description: "EDS `dda-foo-agent` should not be deleted",
			existingAgents: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dda-foo-agent",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: false,
			profilesEnabled:      false,
			edsEnabled:           true,
			providerList:         map[string]struct{}{},
			profiles:             []v1alpha1.DatadogAgentProfile{},
			wantDS:               &appsv1.DaemonSetList{},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-agent",
							ResourceVersion: "999",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
							},
						},
					},
				},
			},
		},
		{
			name:        "no unused ds, introspection enabled, profiles enabled",
			description: "DS `datadog-agent-with-profile-ns-1-profile-1-gke-cos` should not be deleted",
			existingAgents: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           false,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{},
		},
		{
			name:        "no unused eds, introspection enabled, profiles enabled",
			description: "EDS `dda-foo-agent-gke-cos` should not be deleted. The EDS name comes from the default profile",
			existingAgents: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           true,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dda-foo-agent-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name:        "multiple unused ds, introspection enabled, profiles enabled",
			description: "All DS except `datadog-agent-with-profile-ns-1-profile-1-gke-cos` should be deleted",
			existingAgents: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						}},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           false,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{},
		},
		{
			name:        "multiple unused eds, introspection enabled, profiles enabled",
			description: "All but EDS `dda-foo-agent-gke-cos` and DS `datadog-agent-with-profile-ns-1-profile-1-gke-cos` should be deleted",
			existingAgents: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           true,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dda-foo-agent-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name:        "multiple unused ds, introspection enabled, profiles disabled",
			description: "All DS except `dda-foo-agent-gke-cos` should be deleted",
			existingAgents: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						}},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						}},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: true,
			profilesEnabled:      false,
			edsEnabled:           false,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dda-foo-agent-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{},
		},
		{
			name:        "multiple unused eds, introspection enabled, profiles disabled",
			description: "All but EDS `dda-foo-agent-gke-cos` should be deleted",
			existingAgents: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: true,
			profilesEnabled:      false,
			edsEnabled:           true,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dda-foo-agent-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name:        "multiple unused ds, introspection disabled, profiles enabled",
			description: "DS `datadog-agent-with-profile-ns-1-profile-1-gke-cos` should be deleted",
			existingAgents: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						}},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: false,
			profilesEnabled:      true,
			edsEnabled:           false,
			providerList: map[string]struct{}{
				kubernetes.LegacyProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "",
						Name:      "default",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1",
							Namespace: "ns-1",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dda-foo-agent",
							Namespace: "ns-1",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{},
		},
		{
			name:        "multiple unused eds, introspection disabled, profiles enabled",
			description: "All but EDS `dda-foo-agent` and DS `datadog-agent-with-profile-ns-1-profile-1` should be deleted",
			existingAgents: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:       "ns--1-dda--foo",
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
							kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
						},
					},
				},
			},
			introspectionEnabled: false,
			profilesEnabled:      true,
			edsEnabled:           true,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: "",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1",
							Namespace: "ns-1",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dda-foo-agent",
							Namespace: "ns-1",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
								kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
								kubernetes.AppKubernetesPartOfLabelKey:     "ns--1-dda--foo",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name:        "DSs are not created by the operator (do not have the expected labels) and should not be removed",
			description: "No DSs should be deleted",
			existingAgents: []client.Object{
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "ns-1",
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
						},
					},
				},
			},
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           false,
			providerList: map[string]struct{}{
				kubernetes.LegacyProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "",
						Name:      "default",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1",
							Namespace: "ns-1",
							Labels: map[string]string{
								"foo": "bar",
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-agent",
							Namespace:       "ns-1",
							ResourceVersion: "999",
						},
					},
				},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{},
		},
		{
			name:        "EDSs are not created by the operator (do not have the expected labels) and should not be removed",
			description: "Nothing should be deleted",
			existingAgents: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent",
						Namespace: "ns-1",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
						},
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dda-foo-agent-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
						Namespace: "ns-1",
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
							kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
						},
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "datadog-agent-with-profile-ns-1-profile-1",
						Namespace: "ns-1",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
						},
					},
				},
			},
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           true,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: "",
					},
				},
			},
			wantDS: &appsv1.DaemonSetList{
				Items: []appsv1.DaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1",
							Namespace: "ns-1",
							Labels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							},
							ResourceVersion: "999",
						},
					},
				},
			},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{
				Items: []edsdatadoghqv1alpha1.ExtendedDaemonSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1",
							Namespace: "ns-1",
							Labels: map[string]string{
								"foo": "bar",
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-agent-with-profile-ns-1-profile-1-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								kubernetes.AppKubernetesManageByLabelKey:     "datadog-operator",
							},
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "dda-foo-agent",
							Namespace:       "ns-1",
							ResourceVersion: "999",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dda-foo-agent-gke-cos",
							Namespace: "ns-1",
							Labels: map[string]string{
								constants.MD5AgentDeploymentProviderLabelKey: gkeCosProvider,
								apicommon.AgentDeploymentComponentLabelKey:   constants.DefaultAgentResourceSuffix,
							},
							ResourceVersion: "999",
						},
					},
				},
			},
		},
		{
			name:                 "no existing ds, introspection enabled, profiles enabled",
			description:          "DS list should be empty (nothing to delete)",
			existingAgents:       []client.Object{},
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           false,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS:  &appsv1.DaemonSetList{},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{},
		},
		{
			name:                 "no existing eds, introspection enabled, profiles enabled",
			description:          "DS and EDS list should be empty (nothing to delete)",
			existingAgents:       []client.Object{},
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           true,
			providerList: map[string]struct{}{
				gkeCosProvider: {},
			},
			profiles: []v1alpha1.DatadogAgentProfile{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "profile-1",
						Namespace: "ns-1",
					},
				},
			},
			wantDS:  &appsv1.DaemonSetList{},
			wantEDS: &edsdatadoghqv1alpha1.ExtendedDaemonSetList{},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.existingAgents...).Build()
			logger := logf.Log.WithName("test_cleanupExtraneousDaemonSets")
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "test_cleanupExtraneousDaemonSets"})

			r := &Reconciler{
				client:   fakeClient,
				log:      logger,
				recorder: recorder,
				options: ReconcilerOptions{
					IntrospectionEnabled:       tt.introspectionEnabled,
					DatadogAgentProfileEnabled: tt.profilesEnabled,
					ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
						Enabled: tt.edsEnabled,
					},
				},
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

			err := r.cleanupExtraneousDaemonSets(ctx, logger, &dda, &ddaStatus, tt.providerList, tt.profiles)
			assert.NoError(t, err)

			dsList := &appsv1.DaemonSetList{}
			edsList := &edsdatadoghqv1alpha1.ExtendedDaemonSetList{}

			err = fakeClient.List(ctx, dsList)
			assert.NoError(t, err)
			err = fakeClient.List(ctx, edsList)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantDS, dsList)
			assert.Equal(t, tt.wantEDS, edsList)
		})
	}
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
				AgentList: []*datadoghqv2alpha1.DaemonSetStatus{
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
				AgentList: []*datadoghqv2alpha1.DaemonSetStatus{
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
				AgentList: []*datadoghqv2alpha1.DaemonSetStatus{
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
				AgentList: []*datadoghqv2alpha1.DaemonSetStatus{
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
				AgentList: []*datadoghqv2alpha1.DaemonSetStatus{
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
				AgentList: []*datadoghqv2alpha1.DaemonSetStatus{},
			},
		},
		{
			name: "agent status is empty",
			ddaStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*datadoghqv2alpha1.DaemonSetStatus{},
			},
			dsName: "bar",
			wantStatus: &datadoghqv2alpha1.DatadogAgentStatus{
				AgentList: []*datadoghqv2alpha1.DaemonSetStatus{},
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

func Test_labelNodesWithProfiles(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	ctx := context.Background()

	testCases := []struct {
		name           string
		description    string
		profilesByNode map[string]types.NamespacedName
		nodes          []client.Object
		wantNodeLabels map[string]map[string]string
	}{
		{
			name:        "label multiple profile nodes and default node",
			description: "node-1 and node-2 should be labeled with profile name, node-default should stay nil",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"1": "1",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-default",
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					agentprofile.ProfileLabelKey: "profile-1",
					"1":                          "1",
				},
				"node-2": {
					agentprofile.ProfileLabelKey: "profile-2",
				},
				"node-default": nil,
			},
		},
		{
			name:        "label multiple profile nodes, default node has profile label",
			description: "node-1 and node-2 should be labeled with profile name, profile label should be removed from node-default",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"1": "1",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-default",
						Labels: map[string]string{
							agentprofile.ProfileLabelKey: "profile-1",
							"foo":                        "bar",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					agentprofile.ProfileLabelKey: "profile-1",
					"1":                          "1",
				},
				"node-2": {
					agentprofile.ProfileLabelKey: "profile-2",
				},
				"node-default": {
					"foo": "bar",
				},
			},
		},
		{
			name:        "remove old profile label",
			description: "old profile label should be removed from node-2 and node-default",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"1": "1",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
						Labels: map[string]string{
							agentprofile.OldProfileLabelKey: "profile-2",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-default",
						Labels: map[string]string{
							agentprofile.ProfileLabelKey:    "profile-1",
							agentprofile.OldProfileLabelKey: "profile-2",
							"foo":                           "bar",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					agentprofile.ProfileLabelKey: "profile-1",
					"1":                          "1",
				},
				"node-2": {
					agentprofile.ProfileLabelKey: "profile-2",
				},
				"node-default": {
					"foo": "bar",
				},
			},
		},
		{
			name:        "outdated label value",
			description: "profile label value should be replaced with the profile-1",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"1":                          "1",
							agentprofile.ProfileLabelKey: "profile-2",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
						Labels: map[string]string{
							agentprofile.OldProfileLabelKey: "profile-2",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-default",
						Labels: map[string]string{
							agentprofile.ProfileLabelKey:    "profile-1",
							agentprofile.OldProfileLabelKey: "profile-2",
							"foo":                           "bar",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					agentprofile.ProfileLabelKey: "profile-1",
					"1":                          "1",
				},
				"node-2": {
					agentprofile.ProfileLabelKey: "profile-2",
				},
				"node-default": {
					"foo": "bar",
				},
			},
		},
		{
			name:        "no changes",
			description: "no changes needed",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"1":                          "1",
							agentprofile.ProfileLabelKey: "profile-1",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
						Labels: map[string]string{
							agentprofile.ProfileLabelKey: "profile-2",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-default",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					agentprofile.ProfileLabelKey: "profile-1",
					"1":                          "1",
				},
				"node-2": {
					agentprofile.ProfileLabelKey: "profile-2",
				},
				"node-default": {
					"foo": "bar",
				},
			},
		},
		{
			name:        "labels to remove only",
			description: "node-1 old profile label key and node-default profile label key should be removed",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"1":                             "1",
							agentprofile.ProfileLabelKey:    "profile-1",
							agentprofile.OldProfileLabelKey: "profile-2",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
						Labels: map[string]string{
							agentprofile.ProfileLabelKey: "profile-2",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-default",
						Labels: map[string]string{
							"foo":                        "bar",
							agentprofile.ProfileLabelKey: "profile-2",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					agentprofile.ProfileLabelKey: "profile-1",
					"1":                          "1",
				},
				"node-2": {
					agentprofile.ProfileLabelKey: "profile-2",
				},
				"node-default": {
					"foo": "bar",
				},
			},
		},
		{
			name:        "labels to add/change only",
			description: "node-1 profile label key should be changed and node-2 profile label key should be added",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
						Labels: map[string]string{
							"1":                          "1",
							agentprofile.ProfileLabelKey: "profile-2",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "node-2",
						Labels: map[string]string{},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-default",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					agentprofile.ProfileLabelKey: "profile-1",
					"1":                          "1",
				},
				"node-2": {
					agentprofile.ProfileLabelKey: "profile-2",
				},
				"node-default": {
					"foo": "bar",
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.nodes...).Build()

			r := &Reconciler{
				client: fakeClient,
			}

			err := r.labelNodesWithProfiles(ctx, tt.profilesByNode)
			assert.NoError(t, err)

			nodeList := &corev1.NodeList{}
			err = fakeClient.List(ctx, nodeList)
			assert.NoError(t, err)
			assert.Len(t, nodeList.Items, len(tt.wantNodeLabels))

			for _, node := range nodeList.Items {
				expectedNodeLabels, ok := tt.wantNodeLabels[node.Name]
				assert.True(t, ok)
				assert.Equal(t, expectedNodeLabels, node.Labels)
			}
		})
	}
}

func Test_cleanupPodsForProfilesThatNoLongerApply(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	ctx := context.Background()

	testCases := []struct {
		name           string
		description    string
		profilesByNode map[string]types.NamespacedName
		ddaNamespace   string
		existingPods   []client.Object
		wantPods       []corev1.Pod
	}{
		{
			name:        "delete agent pod that shouldn't be running",
			description: "pod-2 should be deleted",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: "foo",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							agentprofile.ProfileLabelKey:               "profile-1",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node-1",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-2",
						Namespace: "foo",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							agentprofile.ProfileLabelKey:               "profile-1",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node-2",
					},
				},
			},
			wantPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: "foo",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							agentprofile.ProfileLabelKey:               "profile-1",
						},
						ResourceVersion: "999",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-1",
					},
				},
			},
		},
		{
			name:        "delete default agent on profile node",
			description: "pod-2 should be deleted",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: "foo",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							agentprofile.ProfileLabelKey:               "profile-1",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node-1",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-default",
						Namespace: "foo",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node-2",
					},
				},
			},
			wantPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: "foo",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							agentprofile.ProfileLabelKey:               "profile-1",
						},
						ResourceVersion: "999",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-1",
					},
				},
			},
		},
		{
			name:        "delete profile agent on default node",
			description: "pod-2 should be deleted",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {
					Namespace: "foo",
					Name:      "profile-1",
				},
				"node-2": {
					Namespace: "foo",
					Name:      "profile-2",
				},
				"node-default": {
					Namespace: "",
					Name:      "default",
				},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: "foo",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							agentprofile.ProfileLabelKey:               "profile-1",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node-1",
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-2",
						Namespace: "foo",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							agentprofile.ProfileLabelKey:               "profile-2",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "node-default",
					},
				},
			},
			wantPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: "foo",
						Labels: map[string]string{
							apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
							agentprofile.ProfileLabelKey:               "profile-1",
						},
						ResourceVersion: "999",
					},
					Spec: corev1.PodSpec{
						NodeName: "node-1",
					},
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.existingPods...).Build()

			r := &Reconciler{
				client: fakeClient,
			}

			err := r.cleanupPodsForProfilesThatNoLongerApply(ctx, tt.profilesByNode, tt.ddaNamespace)
			assert.NoError(t, err)

			podList := &corev1.PodList{}
			err = fakeClient.List(ctx, podList)
			assert.NoError(t, err)
			assert.Len(t, podList.Items, len(tt.wantPods))
			assert.Equal(t, tt.wantPods, podList.Items)
		})
	}
}
