package datadogagent

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// roleBindingInfo contains the required information to build a Cluster Role Binding
type roleBindingInfo struct {
	name               string
	roleName           string
	serviceAccountName string
}

// buildRoleBinding creates a RoleBinding object
func buildRoleBinding(dda *datadoghqv1alpha1.DatadogAgent, info roleBindingInfo, agentVersion string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    getDefaultLabels(dda, info.name, agentVersion),
			Name:      info.name,
			Namespace: dda.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: datadoghqv1alpha1.RbacAPIGroup,
			Kind:     datadoghqv1alpha1.RoleKind,
			Name:     info.roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      datadoghqv1alpha1.ServiceAccountKind,
				Name:      info.serviceAccountName,
				Namespace: dda.Namespace,
			},
		},
	}
}

// buildServiceAccount creates a ServiceAccount object
func buildServiceAccount(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    getDefaultLabels(dda, name, agentVersion),
			Name:      name,
			Namespace: dda.Namespace,
		},
	}
}

// getEventCollectionPolicyRule returns the policy rule for event collection
func getEventCollectionPolicyRule() rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
		APIGroups:     []string{datadoghqv1alpha1.CoreAPIGroup},
		Resources:     []string{datadoghqv1alpha1.ConfigMapsResource},
		ResourceNames: []string{datadoghqv1alpha1.DatadogTokenResourceName},
		Verbs:         []string{datadoghqv1alpha1.GetVerb, datadoghqv1alpha1.UpdateVerb},
	}
}

// getLeaderElectionPolicyRule returns the policy rules for leader election
func getLeaderElectionPolicyRule() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups:     []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources:     []string{datadoghqv1alpha1.ConfigMapsResource},
			ResourceNames: []string{datadoghqv1alpha1.DatadogLeaderElectionResourceName},
			Verbs:         []string{datadoghqv1alpha1.GetVerb, datadoghqv1alpha1.UpdateVerb},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{datadoghqv1alpha1.ConfigMapsResource},
			Verbs:     []string{datadoghqv1alpha1.CreateVerb},
		},
	}
}

func (r *Reconciler) createServiceAccount(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	serviceAccount := buildServiceAccount(dda, name, agentVersion)
	if err := controllerutil.SetControllerReference(dda, serviceAccount, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createServiceAccount", "serviceAccount.name", serviceAccount.Name, "serviceAccount.Namespace", serviceAccount.Namespace)
	event := buildEventInfo(serviceAccount.Name, serviceAccount.Namespace, serviceAccountKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), serviceAccount)
}

func (r *Reconciler) createClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, info roleBindingInfo, agentVersion string) (reconcile.Result, error) {
	clusterRoleBinding := buildClusterRoleBinding(dda, info, agentVersion)
	if err := SetOwnerReference(dda, clusterRoleBinding, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterRoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
	event := buildEventInfo(clusterRoleBinding.Name, clusterRoleBinding.Namespace, clusterRoleBindingKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{}, r.client.Create(context.TODO(), clusterRoleBinding)
}

func (r *Reconciler) cleanupClusterRole(logger logr.Logger, client client.Client, dda *datadoghqv1alpha1.DatadogAgent, name string) (reconcile.Result, error) {
	clusterRole := &rbacv1.ClusterRole{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name}, clusterRole)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if !CheckOwnerReference(dda, clusterRole) {
		return reconcile.Result{}, nil
	}
	logger.V(1).Info("deleteClusterRole", "clusterRole.name", clusterRole.Name, "clusterRole.Namespace", clusterRole.Namespace)
	event := buildEventInfo(clusterRole.Name, clusterRole.Namespace, clusterRoleKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{}, client.Delete(context.TODO(), clusterRole)
}

func (r *Reconciler) cleanupClusterRoleBinding(logger logr.Logger, client client.Client, dda *datadoghqv1alpha1.DatadogAgent, name string) (reconcile.Result, error) {
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name}, clusterRoleBinding)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if !CheckOwnerReference(dda, clusterRoleBinding) {
		return reconcile.Result{}, nil
	}
	logger.V(1).Info("deleteClusterRoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name, "clusterRoleBinding.Namespace", clusterRoleBinding.Namespace)
	event := buildEventInfo(clusterRoleBinding.Name, clusterRoleBinding.Namespace, clusterRoleBindingKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{}, client.Delete(context.TODO(), clusterRoleBinding)
}

func (r *Reconciler) cleanupServiceAccount(logger logr.Logger, client client.Client, dda *datadoghqv1alpha1.DatadogAgent, name string) (reconcile.Result, error) {
	serviceAccount := &corev1.ServiceAccount{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name}, serviceAccount)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if !CheckOwnerReference(dda, serviceAccount) {
		return reconcile.Result{}, nil
	}
	logger.V(1).Info("deleteServiceAccount", "serviceAccount.name", serviceAccount.Name, "serviceAccount.Namespace", serviceAccount.Namespace)
	event := buildEventInfo(serviceAccount.Name, serviceAccount.Namespace, serviceAccountKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{}, client.Delete(context.TODO(), serviceAccount)
}
