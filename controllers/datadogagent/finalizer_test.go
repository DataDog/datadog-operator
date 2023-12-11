package datadogagent

import (
	"context"
	"fmt"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	agenttestutils "github.com/DataDog/datadog-operator/controllers/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/controllers/testutils"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_handleFinalizer_V1(t *testing.T) {
	// TODO: This tests that the associated cluster roles and cluster role
	// bindings are deleted when the dda is marked to be deleted. However, the
	// finalizer does more than that.

	now := metav1.Now()
	dda := test.NewDefaultedDatadogAgent("some_namespace", "some_name", nil)
	dda.DeletionTimestamp = &now // Mark for deletion

	var clusterRoles []*rbacv1.ClusterRole
	var clusterRoleBindings []*rbacv1.ClusterRoleBinding

	initialKubeObjects := []client.Object{dda}
	for _, clusterRole := range clusterRoles {
		initialKubeObjects = append(initialKubeObjects, clusterRole)
	}
	for _, clusterRoleBinding := range clusterRoleBindings {
		initialKubeObjects = append(initialKubeObjects, clusterRoleBinding)
	}
	reconciler := reconcilerV1ForFinalizerTest(initialKubeObjects)

	for _, resourceName := range rbacNamesForDda(dda, reconciler.versionInfo) {
		clusterRoles = append(clusterRoles, buildClusterRole(dda, true, resourceName, ""))
		clusterRoleBindings = append(clusterRoleBindings, buildClusterRoleBinding(
			dda, roleBindingInfo{name: resourceName}, "",
		))
	}

	_, err := reconciler.handleFinalizer(logf.Log.WithName("Handle Finalizer test"), dda, reconciler.finalizeDadV1)
	assert.NoError(t, err)

	// Check that the cluster roles associated with the Datadog Agent have been deleted
	for _, clusterRole := range clusterRoles {
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: clusterRole.Name}, &rbacv1.ClusterRole{})
		assert.True(
			t, err != nil && apierrors.IsNotFound(err), fmt.Sprintf("ClusterRole %s not deleted", clusterRole.Name),
		)
	}

	// Check that the cluster role bindings associated with the Datadog Agent have been deleted
	for _, clusterRoleBinding := range clusterRoleBindings {
		err = reconciler.client.Get(
			context.TODO(), types.NamespacedName{Name: clusterRoleBinding.Name}, &rbacv1.ClusterRoleBinding{},
		)
		assert.True(
			t, err != nil && apierrors.IsNotFound(err), fmt.Sprintf("ClusterRoleBinding %s not deleted", clusterRoleBinding.Name),
		)
	}
}

func Test_handleFinalizer_V2(t *testing.T) {
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
				Kind:       clusterRoleKind,
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
				Kind:       clusterRoleKind,
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
				Kind:       clusterRoleBindingKind,
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
				Kind:       clusterRoleBindingKind,
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

	reconciler := reconcilerV2ForFinalizerTest(initialKubeObjects)

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

func reconcilerV1ForFinalizerTest(initialKubeObjects []client.Object) Reconciler {
	reconcilerScheme := scheme.Scheme
	reconcilerScheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{}, &rbacv1.ClusterRole{})
	reconcilerScheme.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogAgent{})

	fakeClient := fake.NewClientBuilder().WithObjects(initialKubeObjects...).WithScheme(reconcilerScheme).Build()

	return Reconciler{
		client:     fakeClient,
		scheme:     reconcilerScheme,
		recorder:   record.NewBroadcaster().NewRecorder(reconcilerScheme, corev1.EventSource{}),
		forwarders: dummyManager{},
		options:    ReconcilerOptions{V2Enabled: false},
	}
}

func reconcilerV2ForFinalizerTest(initialKubeObjects []client.Object) Reconciler {
	s := agenttestutils.TestScheme(true)

	fakeClient := fake.NewClientBuilder().WithObjects(initialKubeObjects...).WithScheme(s).Build()

	return Reconciler{
		client:     fakeClient,
		scheme:     s,
		recorder:   record.NewBroadcaster().NewRecorder(s, corev1.EventSource{}),
		forwarders: dummyManager{},
		options:    ReconcilerOptions{V2Enabled: true},
		log:        logf.Log.WithName("reconciler_v2"),
	}
}
