package datadogagent

import (
	"context"
	"fmt"
	"testing"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/internal/controller/testutils"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
				},
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: clusteragent.GetClusterAgentName(dda),
				Labels: map[string]string{
					"operator.datadoghq.com/managed-by-store": "true",
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
				},
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleBindingKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: clusteragent.GetClusterAgentName(dda),
				Labels: map[string]string{
					"operator.datadoghq.com/managed-by-store": "true",
				},
			},
		},
	}

	nodes := []*corev1.Node{
		testutils.NewNode("node-1", nil),
		testutils.NewNode("node-2", map[string]string{agentprofile.ProfileLabelKey: "true"}), // The label should be deleted
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

	_, err := reconciler.handleFinalizer(logf.Log.WithName("Handle Finalizer V2 test"), dda, reconciler.finalizeDadV2)
	assert.NoError(t, err)

	// Check that the cluster roles associated with the Datadog Agent have been deleted
	for _, clusterRole := range existingClusterRoles {
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: clusterRole.Name}, &rbacv1.ClusterRole{})
		assert.Error(t, err, fmt.Sprintf("ClusterRole %s not deleted", clusterRole.Name))
		if err != nil {
			assert.True(t, apierrors.IsNotFound(err), fmt.Sprintf("Unexpected error %s", err))
		}
	}

	// Check that the cluster role bindings associated with the Datadog Agent have been deleted
	for _, clusterRoleBinding := range existingClusterRoleBindings {
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: clusterRoleBinding.Name}, &rbacv1.ClusterRoleBinding{})
		assert.Error(t, err, fmt.Sprintf("ClusterRoleBinding %s not deleted", clusterRoleBinding.Name))
		if err != nil {
			assert.True(t, apierrors.IsNotFound(err), fmt.Sprintf("Unexpected error %s", err))
		}
	}

	// Check that the nodes don't have the profile label anymore
	for _, node := range nodes {
		currentNode := &corev1.Node{}
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: node.Name}, currentNode)
		assert.NoError(t, err)
		assert.NotContains(t, currentNode.Labels, agentprofile.ProfileLabelKey)
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
		log:        logf.Log.WithName("reconciler"),
	}
}
