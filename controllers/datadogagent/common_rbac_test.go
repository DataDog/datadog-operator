package datadogagent

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	assert "github.com/stretchr/testify/require"
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

func Test_cleanupClusterRole(t *testing.T) {
	tests := []struct {
		name              string
		dda               *v1alpha1.DatadogAgent
		clusterRoleLabels map[string]string
		expectToBeDeleted bool
	}{
		{
			name: "ClusterRole belongs to DatadogAgent",
			dda:  test.NewDefaultedDatadogAgent("some_namespace", "some_name", nil),
			clusterRoleLabels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:   "some_namespace-some_name",
			},
			expectToBeDeleted: true,
		},
		{
			name: "ClusterRole does not belong to DatadogAgent (not managed by operator)",
			dda:  test.NewDefaultedDatadogAgent("some_namespace", "some_name", nil),
			clusterRoleLabels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: "not-the-datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:   "some_namespace-some_name",
			},
			expectToBeDeleted: false,
		},
		{
			name: "ClusterRole does not belong to DatadogAgent (belongs to other)",
			dda:  test.NewDefaultedDatadogAgent("some_namespace", "some_name", nil),
			clusterRoleLabels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:   "other-datadog-agent",
			},
			expectToBeDeleted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterRole := rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:   getAgentRbacResourcesName(tt.dda),
					Labels: tt.clusterRoleLabels,
				},
			}
			fakeClient := fake.NewClientBuilder().WithObjects(&clusterRole).Build()
			r := newReconcilerForRbacTests(fakeClient)

			_, err := r.cleanupClusterRole(
				logf.Log.WithName(tt.name), r.client, tt.dda, getAgentRbacResourcesName(tt.dda),
			)
			assert.NoError(t, err)

			err = fakeClient.Get(
				context.TODO(), types.NamespacedName{Name: getAgentRbacResourcesName(tt.dda)}, &rbacv1.ClusterRole{},
			)
			if tt.expectToBeDeleted {
				assert.True(t, err != nil && apierrors.IsNotFound(err), "ClusterRole not deleted")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_cleanupClusterRoleBinding(t *testing.T) {
	tests := []struct {
		name                     string
		dda                      *v1alpha1.DatadogAgent
		clusterRoleBindingLabels map[string]string
		expectToBeDeleted        bool
	}{
		{
			name: "ClusterRoleBinding belongs to DatadogAgent",
			dda:  test.NewDefaultedDatadogAgent("some_namespace", "some_name", nil),
			clusterRoleBindingLabels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:   "some_namespace-some_name",
			},
			expectToBeDeleted: true,
		},
		{
			name: "ClusterRoleBinding does not belong to DatadogAgent (not managed by operator)",
			dda:  test.NewDefaultedDatadogAgent("some_namespace", "some_name", nil),
			clusterRoleBindingLabels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: "not-the-datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:   "some_namespace-some_name",
			},
			expectToBeDeleted: false,
		},
		{
			name: "ClusterRoleBinding does not belong to DatadogAgent (belongs to other)",
			dda:  test.NewDefaultedDatadogAgent("some_namespace", "some_name", nil),
			clusterRoleBindingLabels: map[string]string{
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:   "other-datadog-agent",
			},
			expectToBeDeleted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterRoleBinding := rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:   getAgentRbacResourcesName(tt.dda),
					Labels: tt.clusterRoleBindingLabels,
				},
			}
			fakeClient := fake.NewClientBuilder().WithObjects(&clusterRoleBinding).Build()
			reconciler := newReconcilerForRbacTests(fakeClient)

			_, err := reconciler.cleanupClusterRoleBinding(
				logf.Log.WithName(tt.name), reconciler.client, tt.dda, getAgentRbacResourcesName(tt.dda),
			)
			assert.NoError(t, err)

			err = fakeClient.Get(
				context.TODO(), types.NamespacedName{Name: getAgentRbacResourcesName(tt.dda)}, &rbacv1.ClusterRoleBinding{},
			)
			if tt.expectToBeDeleted {
				assert.True(t, err != nil && apierrors.IsNotFound(err), "ClusterRoleBinding not deleted")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func newReconcilerForRbacTests(client client.Client) *Reconciler {
	reconcilerScheme := scheme.Scheme
	reconcilerScheme.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{}, &rbacv1.ClusterRole{})

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{})

	return &Reconciler{
		client:   client,
		scheme:   reconcilerScheme,
		recorder: recorder,
	}
}
