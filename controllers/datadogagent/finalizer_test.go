package datadogagent

import (
	"context"
	"fmt"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
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

func Test_handleFinalizer(t *testing.T) {
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
	reconciler := reconcilerForFinalizerTest(initialKubeObjects)

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

func reconcilerForFinalizerTest(initialKubeObjects []client.Object) Reconciler {
	reconcilerScheme := scheme.Scheme
	reconcilerScheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{}, &rbacv1.ClusterRole{})
	reconcilerScheme.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogAgent{})

	fakeClient := fake.NewClientBuilder().WithObjects(initialKubeObjects...).WithScheme(reconcilerScheme).Build()

	return Reconciler{
		client:     fakeClient,
		scheme:     reconcilerScheme,
		recorder:   record.NewBroadcaster().NewRecorder(reconcilerScheme, corev1.EventSource{}),
		forwarders: dummyManager{},
	}
}
