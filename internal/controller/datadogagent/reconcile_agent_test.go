package datadogagent

import (
	"context"
	"testing"

	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
		useV3Metadata        bool
		existingProviders    map[string]struct{}
		existingProfiles     []v1alpha1.DatadogAgentProfile
		wantDS               map[string]struct{}
		wantEDS              map[string]struct{}
	}{
		// V2 Metadata test cases
		{
			name:                 "v2: introspection disabled, profiles disabled, eds disabled",
			dsName:               "foo",
			introspectionEnabled: false,
			profilesEnabled:      false,
			edsEnabled:           false,
			useV3Metadata:        false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo": {}},
			wantEDS:          map[string]struct{}{},
		},
		{
			name:                 "v2: introspection disabled, profiles disabled, eds enabled",
			dsName:               "foo",
			introspectionEnabled: false,
			profilesEnabled:      false,
			edsEnabled:           true,
			useV3Metadata:        false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{},
			wantEDS:          map[string]struct{}{"foo": {}},
		},
		{
			name:                 "v2: introspection enabled, profiles disabled, eds disabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      false,
			edsEnabled:           false,
			useV3Metadata:        false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo-gke-cos": {}},
			wantEDS:          map[string]struct{}{},
		},
		{
			name:                 "v2: introspection enabled, profiles disabled, eds enabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      false,
			edsEnabled:           true,
			useV3Metadata:        false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{},
			wantEDS:          map[string]struct{}{"foo-gke-cos": {}},
		},
		{
			name:                 "v2: introspection enabled, profiles enabled, eds disabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           false,
			useV3Metadata:        false,
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
			name:                 "v2: introspection enabled, profiles enabled, eds enabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           true,
			useV3Metadata:        false,
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
		// V3 Metadata test cases
		{
			name:                 "v3: introspection disabled, profiles disabled, eds disabled",
			dsName:               "foo",
			introspectionEnabled: false,
			profilesEnabled:      false,
			edsEnabled:           false,
			useV3Metadata:        true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo": {}},
			wantEDS:          map[string]struct{}{},
		},
		{
			name:                 "v3: introspection disabled, profiles disabled, eds enabled",
			dsName:               "foo",
			introspectionEnabled: false,
			profilesEnabled:      false,
			edsEnabled:           true,
			useV3Metadata:        true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{},
			wantEDS:          map[string]struct{}{"foo": {}},
		},
		{
			name:                 "v3: introspection enabled, profiles disabled, eds disabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      false,
			edsEnabled:           false,
			useV3Metadata:        true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo-gke-cos": {}},
			wantEDS:          map[string]struct{}{},
		},
		{
			name:                 "v3: introspection enabled, profiles disabled, eds enabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      false,
			edsEnabled:           true,
			useV3Metadata:        true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{},
			wantEDS:          map[string]struct{}{"foo-gke-cos": {}},
		},
		{
			name:                 "v3: introspection enabled, profiles enabled, eds disabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           false,
			useV3Metadata:        true,
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
				"foo-default":             {},
				"foo-gke-cos":             {},
				"profile-1-agent-default": {},
				"profile-1-agent-gke-cos": {},
			},
			wantEDS: map[string]struct{}{},
		},
		{
			name:                 "v3: introspection enabled, profiles enabled, eds enabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      true,
			edsEnabled:           true,
			useV3Metadata:        true,
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
				"profile-1-agent-default": {},
				"profile-1-agent-gke-cos": {},
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

			validDSNames, validEDSNames := r.getValidDaemonSetNames(tt.dsName, tt.existingProviders, tt.existingProfiles, tt.useV3Metadata)
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
							Replicas: ptr.To[int32](10),
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
			dda: &datadoghqv2alpha1.DatadogAgent{
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
			dsName := component.GetDaemonSetNameFromDatadogAgent(tt.dda, &tt.dda.Spec)
			assert.Equal(t, tt.wantDSName, dsName)
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
					constants.ProfileLabelKey: "profile-1",
					"1":                       "1",
				},
				"node-2": {
					constants.ProfileLabelKey: "profile-2",
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
							constants.ProfileLabelKey: "profile-1",
							"foo":                     "bar",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					constants.ProfileLabelKey: "profile-1",
					"1":                       "1",
				},
				"node-2": {
					constants.ProfileLabelKey: "profile-2",
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
							constants.ProfileLabelKey:       "profile-1",
							agentprofile.OldProfileLabelKey: "profile-2",
							"foo":                           "bar",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					constants.ProfileLabelKey: "profile-1",
					"1":                       "1",
				},
				"node-2": {
					constants.ProfileLabelKey: "profile-2",
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
							"1":                       "1",
							constants.ProfileLabelKey: "profile-2",
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
							constants.ProfileLabelKey:       "profile-1",
							agentprofile.OldProfileLabelKey: "profile-2",
							"foo":                           "bar",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					constants.ProfileLabelKey: "profile-1",
					"1":                       "1",
				},
				"node-2": {
					constants.ProfileLabelKey: "profile-2",
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
							"1":                       "1",
							constants.ProfileLabelKey: "profile-1",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
						Labels: map[string]string{
							constants.ProfileLabelKey: "profile-2",
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
					constants.ProfileLabelKey: "profile-1",
					"1":                       "1",
				},
				"node-2": {
					constants.ProfileLabelKey: "profile-2",
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
							constants.ProfileLabelKey:       "profile-1",
							agentprofile.OldProfileLabelKey: "profile-2",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-2",
						Labels: map[string]string{
							constants.ProfileLabelKey: "profile-2",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-default",
						Labels: map[string]string{
							"foo":                     "bar",
							constants.ProfileLabelKey: "profile-2",
						},
					},
				},
			},
			wantNodeLabels: map[string]map[string]string{
				"node-1": {
					constants.ProfileLabelKey: "profile-1",
					"1":                       "1",
				},
				"node-2": {
					constants.ProfileLabelKey: "profile-2",
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
							"1":                       "1",
							constants.ProfileLabelKey: "profile-2",
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
					constants.ProfileLabelKey: "profile-1",
					"1":                       "1",
				},
				"node-2": {
					constants.ProfileLabelKey: "profile-2",
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
							constants.ProfileLabelKey:                  "profile-1",
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
							constants.ProfileLabelKey:                  "profile-1",
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
							constants.ProfileLabelKey:                  "profile-1",
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
							constants.ProfileLabelKey:                  "profile-1",
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
							constants.ProfileLabelKey:                  "profile-1",
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
							constants.ProfileLabelKey:                  "profile-1",
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
							constants.ProfileLabelKey:                  "profile-2",
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
							constants.ProfileLabelKey:                  "profile-1",
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
