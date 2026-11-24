package datadogagent

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/internal/controller/testutils"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_handleFinalizer(t *testing.T) {
	// This is not an exhaustive test. The finalizer should remove all the
	// kubernetes resources associated with the Datadog Agent being removed, but
	// to simplify a bit, this test doesn't check all the resources, it just
	// checks a few ones (cluster roles, cluster role bindings, profile labels).

	now := metav1.Now()

	dda := &datadoghqv2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "foo",
			Name:       "bar",
			Finalizers: []string{"finalizer.agent.datadoghq.com"},
		},
		Spec: datadoghqv2alpha1.DatadogAgentSpec{
			Global: &datadoghqv2alpha1.GlobalConfig{
				Credentials: &datadoghqv2alpha1.DatadogCredentials{
					APIKey: ptr.To("apiKey"),
					AppKey: ptr.To("appKey"),
				},
			},
		},
	}
	dda.DeletionTimestamp = &now // Mark for deletion

	initialKubeObjects := []client.Object{dda}

	// These are some cluster roles that we know that the reconciler creates by
	// default
	existingClusterRoles := []*rbacv1.ClusterRole{
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: agent.GetAgentRoleName(dda),
				Labels: map[string]string{
					"operator.datadoghq.com/managed-by-store": "true",
					"app.kubernetes.io/part-of":               "foo-bar",
					"app.kubernetes.io/managed-by":            "datadog-operator",
				},
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: component.GetClusterAgentName(dda),
				Labels: map[string]string{
					"operator.datadoghq.com/managed-by-store": "true",
					"app.kubernetes.io/part-of":               "foo-bar",
					"app.kubernetes.io/managed-by":            "datadog-operator",
				},
			},
		},
	}

	// These are some cluster role bindings that we know that the reconciler
	// creates by default
	existingClusterRoleBindings := []*rbacv1.ClusterRoleBinding{
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleBindingKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: agent.GetAgentRoleName(dda), // Same name as the cluster role
				Labels: map[string]string{
					"operator.datadoghq.com/managed-by-store": "true",
					"app.kubernetes.io/part-of":               "foo-bar",
					"app.kubernetes.io/managed-by":            "datadog-operator",
				},
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleBindingKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: component.GetClusterAgentName(dda),
				Labels: map[string]string{
					"operator.datadoghq.com/managed-by-store": "true",
					"app.kubernetes.io/part-of":               "foo-bar",
					"app.kubernetes.io/managed-by":            "datadog-operator",
				},
			},
		},
	}

	nodes := []*corev1.Node{
		testutils.NewNode("node-1", nil),
		testutils.NewNode("node-2", map[string]string{
			constants.ProfileLabelKey:          "legacy-profile",
			agentprofile.NodeLabelKey("foo"):   "profile",
			agentprofile.NodeLabelKey("other"): "other-profile",
		}),
		testutils.NewNode("node-3", map[string]string{agentprofile.NodeLabelKey("foo"): "profile"}),
	}

	for _, clusterRole := range existingClusterRoles {
		initialKubeObjects = append(initialKubeObjects, clusterRole)
	}

	for _, clusterRoleBinding := range existingClusterRoleBindings {
		initialKubeObjects = append(initialKubeObjects, clusterRoleBinding)
	}

	for _, node := range nodes {
		initialKubeObjects = append(initialKubeObjects, node)
	}

	reconciler := reconcilerForFinalizerTest(initialKubeObjects)

	err := reconciler.finalizeDad(context.Background(), dda)
	assert.NoError(t, err)

	// With DDAI always enabled, the DDA finalizer no longer deletes cluster-level resources
	// (the DDAI controller handles cleanup). Verify they still exist.
	for _, clusterRole := range existingClusterRoles {
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: clusterRole.Name}, &rbacv1.ClusterRole{})
		assert.NoError(t, err, fmt.Sprintf("ClusterRole %s should still exist (DDAI handles cleanup)", clusterRole.Name))
	}

	for _, clusterRoleBinding := range existingClusterRoleBindings {
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: clusterRoleBinding.Name}, &rbacv1.ClusterRoleBinding{})
		assert.NoError(t, err, fmt.Sprintf("ClusterRoleBinding %s should still exist (DDAI handles cleanup)", clusterRoleBinding.Name))
	}

	// Check that the nodes don't have the profile label anymore
	for _, node := range nodes {
		currentNode := &corev1.Node{}
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: node.Name}, currentNode)
		assert.NoError(t, err)
		assert.NotContains(t, currentNode.Labels, constants.ProfileLabelKey)
		assert.NotContains(t, currentNode.Labels, agentprofile.NodeLabelKey("foo"))
		if node.Name == "node-2" {
			assert.Equal(t, "other-profile", currentNode.Labels[agentprofile.NodeLabelKey("other")])
		}
	}
}

func reconcilerForFinalizerTest(initialKubeObjects []client.Object) Reconciler {
	s := agenttestutils.TestScheme()

	fakeClient := fake.NewClientBuilder().WithObjects(initialKubeObjects...).WithScheme(s).Build()

	return Reconciler{
		client:     fakeClient,
		scheme:     s,
		recorder:   record.NewBroadcaster().NewRecorder(s, corev1.EventSource{}),
		forwarders: dummyManager{},
		options:    ReconcilerOptions{},
	}
}

// legacyAffinityDaemonSet returns an operator-managed DaemonSet that still
// selects nodes with the legacy shared profile label.
func legacyAffinityDaemonSet(namespace, ddaName string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "profile-agent",
			Labels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
				apicommon.AgentDeploymentNameLabelKey:    ddaName,
			},
		},
		Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key: constants.ProfileLabelKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"profile"},
					}}}},
				},
			}},
		}}},
	}
}

// Test_profilesCleanup_legacyLabel checks that deleting a DatadogAgent keeps
// the legacy shared label while another DatadogAgent's DaemonSets still select
// with it, and ignores its own DaemonSets, which are about to be deleted.
func Test_profilesCleanup_legacyLabel(t *testing.T) {
	tests := []struct {
		name            string
		daemonSet       *appsv1.DaemonSet
		wantLegacyLabel bool
	}{
		{name: "no DaemonSet uses the legacy label", wantLegacyLabel: false},
		{name: "own DaemonSet is ignored", daemonSet: legacyAffinityDaemonSet("foo", "dda"), wantLegacyLabel: false},
		{name: "another DatadogAgent's DaemonSet keeps it", daemonSet: legacyAffinityDaemonSet("bar", "dda"), wantLegacyLabel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []client.Object{testutils.NewNode("node", map[string]string{
				constants.ProfileLabelKey:        "profile",
				agentprofile.NodeLabelKey("foo"): "profile",
			})}
			if tt.daemonSet != nil {
				objects = append(objects, tt.daemonSet)
			}
			reconciler := reconcilerForFinalizerTest(objects)

			assert.NoError(t, reconciler.profilesCleanup(context.Background(), "foo", "dda"))

			node := &corev1.Node{}
			assert.NoError(t, reconciler.client.Get(context.Background(), types.NamespacedName{Name: "node"}, node))
			assert.NotContains(t, node.Labels, agentprofile.NodeLabelKey("foo"))
			if tt.wantLegacyLabel {
				assert.Equal(t, "profile", node.Labels[constants.ProfileLabelKey])
			} else {
				assert.NotContains(t, node.Labels, constants.ProfileLabelKey)
			}
		})
	}
}
