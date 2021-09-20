package datadogagent

import (
	"context"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
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
	logger.V(1).Info("createClusterRoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
	event := buildEventInfo(clusterRoleBinding.Name, clusterRoleBinding.Namespace, clusterRoleBindingKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	err := r.client.Create(context.TODO(), clusterRoleBinding)
	return reconcile.Result{}, err
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

	if !isOwnerBasedOnLabels(dda, clusterRole.Labels) {
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

	if !isOwnerBasedOnLabels(dda, clusterRoleBinding.Labels) {
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

func (r *Reconciler) updateIfNeededClusterRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, clusterRoleBindingName, roleName, serviceAccountName, version string, clusterRoleBinding *rbacv1.ClusterRoleBinding) (reconcile.Result, error) {
	info := roleBindingInfo{
		name:               clusterRoleBindingName,
		roleName:           roleName,
		serviceAccountName: serviceAccountName,
	}
	newClusterRoleBinding := buildClusterRoleBinding(dda, info, version)
	return r.updateIfNeededClusterRoleBindingRaw(logger, dda, clusterRoleBinding, newClusterRoleBinding)
}

func (r *Reconciler) updateIfNeededClusterRoleBindingRaw(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, clusterRoleBinding, newClusterRoleBinding *rbacv1.ClusterRoleBinding) (reconcile.Result, error) {
	if !isClusterRolesBindingEqual(newClusterRoleBinding, clusterRoleBinding) {
		logger.V(1).Info("updateIfNeededClusterRoleBinding", "clusterRoleBinding.name", clusterRoleBinding.Name)
		// ClusterRoleBinding can't be updated, if we change the RoleRef in it, we need to delete and recreate
		if err := r.client.Delete(context.TODO(), clusterRoleBinding); err != nil {
			return reconcile.Result{}, err
		}
		if err := r.client.Create(context.TODO(), newClusterRoleBinding); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(newClusterRoleBinding.Name, newClusterRoleBinding.Namespace, clusterRoleKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) updateIfNeededClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, clusterRole, newClusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	if !isClusterRolesEqual(newClusterRole, clusterRole) {
		logger.V(1).Info("updateIfNeededClusterRole", "clusterRole.name", clusterRole.Name)
		if err := r.client.Update(context.TODO(), newClusterRole); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(newClusterRole.Name, newClusterRole.Namespace, clusterRoleKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) updateIfNeededRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, role, newRole *rbacv1.Role) (reconcile.Result, error) {
	if !isRolesEqual(newRole, role) {
		logger.V(1).Info("updateIfNeededRole", "role.name", role.Name)
		if err := r.client.Update(context.TODO(), newRole); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(newRole.Name, newRole.Namespace, roleKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) updateIfNeededRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, roleBinding, newRoleBinding *rbacv1.RoleBinding) (reconcile.Result, error) {
	if !isRolesBindingEqual(newRoleBinding, roleBinding) {
		logger.V(1).Info("updateIfNeededRoleBinding", "roleBinding.name", newRoleBinding.Name, "roleBinding.namespace", newRoleBinding.Namespace)
		// RoleBinding can't be updated, if we change the RoleRef in it, we need to delete and recreate
		if err := r.client.Delete(context.TODO(), roleBinding); err != nil {
			return reconcile.Result{}, err
		}
		if err := r.client.Create(context.TODO(), newRoleBinding); err != nil {
			return reconcile.Result{}, err
		}

		event := buildEventInfo(newRoleBinding.Name, newRoleBinding.Namespace, roleBindingKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}
	return reconcile.Result{}, nil
}

// isOwnerBasedOnLabels returns whether a DatadogAgent is the owner of a
// resource based on its labels.
// DatadogAgent objects are namespace-scoped. Some resources like cluster roles
// and cluster role bindings are not. This means that the DatadogAgent objects
// cannot be set as owner ref for those. For those objects, we can use their
// labels to know whether a DatadogAgent object owns them.
func isOwnerBasedOnLabels(dda *datadoghqv1alpha1.DatadogAgent, labels map[string]string) bool {
	isManagedByOperator := labels[kubernetes.AppKubernetesManageByLabelKey] == "datadog-operator"
	isPartOfDDA := labels[kubernetes.AppKubernetesPartOfLabelKey] == NewPartOfLabelValue(dda).String()
	return isManagedByOperator && isPartOfDDA
}

func rbacNamesForDda(dda *datadoghqv1alpha1.DatadogAgent) []string {
	return []string{
		getAgentRbacResourcesName(dda),
		getClusterAgentRbacResourcesName(dda),
		getClusterChecksRunnerRbacResourcesName(dda),
	}
}

func isRolesEqual(a, b *rbacv1.Role) bool {
	return apiequality.Semantic.DeepEqual(a.Rules, b.Rules) && apiequality.Semantic.DeepEqual(a.ObjectMeta.OwnerReferences, b.ObjectMeta.OwnerReferences)
}

func isRolesBindingEqual(a, b *rbacv1.RoleBinding) bool {
	return apiequality.Semantic.DeepEqual(a.RoleRef, b.RoleRef) && apiequality.Semantic.DeepEqual(a.Subjects, b.Subjects) && apiequality.Semantic.DeepEqual(a.ObjectMeta.OwnerReferences, b.ObjectMeta.OwnerReferences)
}

func isClusterRolesEqual(a, b *rbacv1.ClusterRole) bool {
	return apiequality.Semantic.DeepEqual(a.Rules, b.Rules) && apiequality.Semantic.DeepEqual(a.ObjectMeta.OwnerReferences, b.ObjectMeta.OwnerReferences)
}

func isClusterRolesBindingEqual(a, b *rbacv1.ClusterRoleBinding) bool {
	return apiequality.Semantic.DeepEqual(a.RoleRef, b.RoleRef) && apiequality.Semantic.DeepEqual(a.Subjects, b.Subjects) && apiequality.Semantic.DeepEqual(a.ObjectMeta.OwnerReferences, b.ObjectMeta.OwnerReferences)
}
