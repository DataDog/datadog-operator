package datadogagent

import (
	"context"

	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

type networkPolicyBuilder func(dda *datadoghqv1alpha1.DatadogAgent, name string) *networkingv1.NetworkPolicy

func (r *Reconciler) ensureNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, policyName string, builder networkPolicyBuilder) (reconcile.Result, error) {
	policy := &networkingv1.NetworkPolicy{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: policyName, Namespace: dda.Namespace}, policy)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createNetworkPolicy(logger, dda, policyName, builder)
		}

		return reconcile.Result{}, err
	}

	result, err := r.updateNetworkPolicy(logger, dda, policyName, policy, builder)
	if err != nil {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) cleanupNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name string) (reconcile.Result, error) {
	policy := &networkingv1.NetworkPolicy{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: dda.Namespace}, policy)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	if !CheckOwnerReference(dda, policy) {
		return reconcile.Result{}, nil
	}

	logger.V(1).Info("deleteNetworkPolicy", "networkPolicy.name", policy.Name, "networkPolicy.Namespace", policy.Namespace)
	event := buildEventInfo(policy.Name, policy.Namespace, networkPolicyKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{}, r.client.Delete(context.TODO(), policy)
}

func (r *Reconciler) createNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name string, builder networkPolicyBuilder) (reconcile.Result, error) {
	policy := builder(dda, name)

	err := controllerutil.SetControllerReference(dda, policy, r.scheme)
	if err != nil {
		return reconcile.Result{}, err
	}

	logger.V(1).Info("createNetworkPolicy", "networkPolicy.name", policy.Name, "networkPolicy.Namespace", policy.Namespace)
	event := buildEventInfo(policy.Name, policy.Namespace, networkPolicyKind, datadog.CreationEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{}, r.client.Create(context.TODO(), policy)
}

func (r *Reconciler) updateNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name string, policy *networkingv1.NetworkPolicy, builder networkPolicyBuilder) (reconcile.Result, error) {
	newPolicy := builder(dda, name)

	if !apiequality.Semantic.DeepEqual(newPolicy.Spec, policy.Spec) {
		logger.V(1).Info("createNetworkPolicy", "networkPolicy.name", policy.Name, "networkPolicy.Namespace", policy.Namespace)

		err := r.client.Update(context.TODO(), newPolicy)
		if err != nil {
			return reconcile.Result{}, err
		}

		event := buildEventInfo(newPolicy.Name, newPolicy.Namespace, networkPolicyKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}

	return reconcile.Result{}, nil
}
