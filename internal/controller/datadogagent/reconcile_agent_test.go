package datadogagent

import (
	"context"
	"maps"
	"testing"
	"time"

	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/config"
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
		useV3Metadata        bool
		existingProviders    map[string]struct{}
		existingProfiles     []v1alpha1.DatadogAgentProfile
		wantDS               map[string]struct{}
	}{
		// V2 Metadata test cases
		{
			name:                 "v2: introspection disabled, profiles disabled",
			dsName:               "foo",
			introspectionEnabled: false,
			profilesEnabled:      false,
			useV3Metadata:        false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo": {}},
		},
		{
			name:                 "v2: introspection enabled, profiles disabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      false,
			useV3Metadata:        false,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo-gke-cos": {}},
		},
		{
			name:                 "v2: introspection enabled, profiles enabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      true,
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
		},
		// V3 Metadata test cases
		{
			name:                 "v3: introspection disabled, profiles disabled",
			dsName:               "foo",
			introspectionEnabled: false,
			profilesEnabled:      false,
			useV3Metadata:        true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo": {}},
		},
		{
			name:                 "v3: introspection enabled, profiles disabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      false,
			useV3Metadata:        true,
			existingProviders: map[string]struct{}{
				gkeCosProvider: {},
			},
			existingProfiles: []v1alpha1.DatadogAgentProfile{},
			wantDS:           map[string]struct{}{"foo-gke-cos": {}},
		},
		{
			name:                 "v3: introspection enabled, profiles enabled",
			dsName:               "foo",
			introspectionEnabled: true,
			profilesEnabled:      true,
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
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				options: ReconcilerOptions{
					IntrospectionEnabled:       tt.introspectionEnabled,
					DatadogAgentProfileEnabled: tt.profilesEnabled,
				},
			}

			validDSNames := r.getValidDaemonSetNames(tt.dsName, tt.existingProviders, tt.existingProfiles, tt.useV3Metadata)
			assert.Equal(t, tt.wantDS, validDSNames)
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
		{
			name:        "delete unschedulable default pod on a node that has a profile",
			description: "pending-default is pinned to node-1 by affinity and has no profile label, so it is deleted; pending-profile-1 matches node-1's profile and is kept",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {Namespace: "foo", Name: "profile-1"},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				pendingAgentPod("pending-default", "node-1", nil),
				pendingAgentPod("pending-profile-1", "node-1", map[string]string{constants.ProfileLabelKey: "profile-1"}),
			},
			wantPods: []corev1.Pod{
				*withResourceVersion(pendingAgentPod("pending-profile-1", "node-1", map[string]string{constants.ProfileLabelKey: "profile-1"})),
			},
		},
		{
			name:        "keep pending default pod on a default node",
			description: "the pod has no profile label and the node uses the default profile, so it is kept",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {Namespace: "", Name: "default"},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				pendingAgentPod("pod-1", "node-1", nil),
			},
			wantPods: []corev1.Pod{
				*withResourceVersion(pendingAgentPod("pod-1", "node-1", nil)),
			},
		},
		{
			name:        "keep pending default pods when there are no profiles",
			description: "every node uses the default profile, so no pod is deleted",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {Namespace: "", Name: "default"},
				"node-2": {Namespace: "", Name: "default"},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				pendingAgentPod("pod-1", "node-1", nil),
				pendingAgentPod("pod-2", "node-2", nil),
			},
			wantPods: []corev1.Pod{
				*withResourceVersion(pendingAgentPod("pod-1", "node-1", nil)),
				*withResourceVersion(pendingAgentPod("pod-2", "node-2", nil)),
			},
		},
		{
			name:        "delete pending profile pod on a default node",
			description: "the node no longer has a profile, so the pod with a profile label is deleted",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {Namespace: "", Name: "default"},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				pendingAgentPod("pod-1", "node-1", map[string]string{constants.ProfileLabelKey: "profile-1"}),
			},
			wantPods: []corev1.Pod{},
		},
		{
			name:        "keep pending pod on a node that is not in profilesByNode",
			description: "the node was held back by the create strategy, so the pod is kept",
			profilesByNode: map[string]types.NamespacedName{
				"node-2": {Namespace: "foo", Name: "profile-1"},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				pendingAgentPod("pod-1", "node-1", nil),
			},
			wantPods: []corev1.Pod{
				*withResourceVersion(pendingAgentPod("pod-1", "node-1", nil)),
			},
		},
		{
			name:        "keep pending pod that is not unschedulable",
			description: "pod has no Unschedulable condition, so it is kept",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {Namespace: "foo", Name: "profile-1"},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				func() client.Object {
					pod := pendingAgentPod("pod-1", "node-1", nil)
					pod.Status.Conditions = nil
					return pod
				}(),
			},
			wantPods: []corev1.Pod{
				*withResourceVersion(func() *corev1.Pod {
					pod := pendingAgentPod("pod-1", "node-1", nil)
					pod.Status.Conditions = nil
					return pod
				}()),
			},
		},
		{
			name:        "keep unschedulable pod without node affinity",
			description: "the node can't be determined, so the pod is kept",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {Namespace: "foo", Name: "profile-1"},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				func() client.Object {
					pod := pendingAgentPod("pod-1", "node-1", nil)
					pod.Spec.Affinity = nil
					return pod
				}(),
			},
			wantPods: []corev1.Pod{
				*withResourceVersion(func() *corev1.Pod {
					pod := pendingAgentPod("pod-1", "node-1", nil)
					pod.Spec.Affinity = nil
					return pod
				}()),
			},
		},
		{
			name:        "keep unschedulable pod not managed by the operator",
			description: "the pod has no managed-by=datadog-operator label, so it is kept",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {Namespace: "foo", Name: "profile-1"},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				func() client.Object {
					pod := pendingAgentPod("pod-1", "node-1", nil)
					delete(pod.Labels, kubernetes.AppKubernetesManageByLabelKey)
					return pod
				}(),
			},
			wantPods: []corev1.Pod{
				*withResourceVersion(func() *corev1.Pod {
					pod := pendingAgentPod("pod-1", "node-1", nil)
					delete(pod.Labels, kubernetes.AppKubernetesManageByLabelKey)
					return pod
				}()),
			},
		},
		{
			name:        "skip pod that is already being deleted",
			description: "the pod matches the delete rule but has a deletion timestamp, so no delete is sent and it stays until Kubernetes removes it",
			profilesByNode: map[string]types.NamespacedName{
				"node-1": {Namespace: "foo", Name: "profile-1"},
			},
			ddaNamespace: "foo",
			existingPods: []client.Object{
				func() client.Object {
					pod := pendingAgentPod("pod-1", "node-1", nil)
					pod.DeletionTimestamp = new(metav1.NewTime(time.Unix(1700000000, 0)))
					pod.Finalizers = []string{"test/keep"}
					return pod
				}(),
			},
			wantPods: []corev1.Pod{
				*withResourceVersion(func() *corev1.Pod {
					pod := pendingAgentPod("pod-1", "node-1", nil)
					pod.DeletionTimestamp = new(metav1.NewTime(time.Unix(1700000000, 0)))
					pod.Finalizers = []string{"test/keep"}
					return pod
				}()),
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.existingPods...).
				WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
					return []string{obj.(*corev1.Pod).Spec.NodeName}
				}).Build()

			r := &Reconciler{
				client:  fakeClient,
				options: ReconcilerOptions{APIReader: fakeClient},
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

// pendingAgentPod returns an unschedulable agent pod pinned to nodeName by node affinity, like the ones the DaemonSet controller creates.
func pendingAgentPod(name, nodeName string, extraLabels map[string]string) *corev1.Pod {
	labels := map[string]string{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
	}
	maps.Copy(labels, extraLabels)

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "foo",
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchFields: []corev1.NodeSelectorRequirement{{
								Key:      common.FieldPathMetaName,
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{nodeName},
							}},
						}},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodScheduled,
				Status: corev1.ConditionFalse,
				Reason: corev1.PodReasonUnschedulable,
			}},
		},
	}
}

// withResourceVersion sets the resource version the fake client gives to stored objects.
func withResourceVersion(pod *corev1.Pod) *corev1.Pod {
	pod.ResourceVersion = "999"
	return pod
}

// failingReader fails the test if the API server is read.
type failingReader struct {
	client.Reader
	t *testing.T
}

func (f failingReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	f.t.Error("API server should not be read when all agent pods have a node")
	return nil
}

// Test_cleanupPodsForProfilesThatNoLongerApply_noUnscheduledPods checks that the API server is not read when every agent pod has a node.
func Test_cleanupPodsForProfilesThatNoLongerApply_noUnscheduledPods(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "foo",
			Labels:    map[string]string{apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix},
		},
		Spec: corev1.PodSpec{NodeName: "node-1"},
	}
	r := &Reconciler{
		client:  fake.NewClientBuilder().WithScheme(sch).WithObjects(pod).Build(),
		options: ReconcilerOptions{APIReader: failingReader{t: t}},
	}

	err := r.cleanupPodsForProfilesThatNoLongerApply(context.Background(), map[string]types.NamespacedName{"node-1": {Namespace: "", Name: "default"}}, "foo")
	assert.NoError(t, err)
}

// podCacheTransform returns the function the operator's pod cache uses to strip pods.
func podCacheTransform(t *testing.T) func(any) (any, error) {
	t.Setenv(config.AgentWatchNamespaceEnvVar, "foo")
	cacheOptions := config.CacheOptions(logf.Log.WithName(t.Name()), config.WatchOptions{DatadogAgentProfileEnabled: true})
	for obj, byObject := range cacheOptions.ByObject {
		if _, ok := obj.(*corev1.Pod); ok {
			return byObject.Transform
		}
	}
	t.Fatal("no pod cache options found")
	return nil
}

// Test_cleanupPodsForProfilesThatNoLongerApply_withPodCache checks the cleanup when the cached client only has the pod fields the operator's cache keeps.
func Test_cleanupPodsForProfilesThatNoLongerApply_withPodCache(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	ctx := context.Background()
	transform := podCacheTransform(t)

	fullPods := []client.Object{
		pendingAgentPod("pending-default", "node-1", nil),
		pendingAgentPod("pending-profile-1", "node-1", map[string]string{constants.ProfileLabelKey: "profile-1"}),
	}
	cachedPods := make([]client.Object, 0, len(fullPods))
	for _, pod := range fullPods {
		cached, err := transform(pod.DeepCopyObject())
		assert.NoError(t, err)
		cachedPods = append(cachedPods, cached.(client.Object))
	}

	cachedClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(cachedPods...).Build()
	apiReader := fake.NewClientBuilder().WithScheme(sch).WithObjects(fullPods...).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			return []string{obj.(*corev1.Pod).Spec.NodeName}
		}).Build()
	r := &Reconciler{
		client:  cachedClient,
		options: ReconcilerOptions{APIReader: apiReader},
	}

	err := r.cleanupPodsForProfilesThatNoLongerApply(ctx, map[string]types.NamespacedName{"node-1": {Namespace: "foo", Name: "profile-1"}}, "foo")
	assert.NoError(t, err)

	podList := &corev1.PodList{}
	assert.NoError(t, cachedClient.List(ctx, podList))
	remaining := make([]string, 0, len(podList.Items))
	for _, pod := range podList.Items {
		remaining = append(remaining, pod.Name)
	}
	assert.Equal(t, []string{"pending-profile-1"}, remaining, "the default pod should be deleted and the profile pod kept")
}
