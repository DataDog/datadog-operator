package datadogagent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	defaultSite = "datadoghq.com"
)

type networkPolicyBuilder interface {
	Name() string
	NetworkPolicySpec() *datadoghqv1alpha1.NetworkPolicySpec
	BuildKubernetesPolicy() *networkingv1.NetworkPolicy
	BuildCiliumPolicy() *cilium.NetworkPolicy
	PodSelector() metav1.LabelSelector
}

func (r *Reconciler) ensureNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, builder networkPolicyBuilder) (reconcile.Result, error) {
	var (
		err    error
		result reconcile.Result
	)

	policyName := builder.Name()
	policySpec := builder.NetworkPolicySpec()

	switch policySpec.Flavor {
	case datadoghqv1alpha1.NetworkPolicyFlavorKubernetes:
		policy := &networkingv1.NetworkPolicy{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: policyName, Namespace: dda.Namespace}, policy)
		if err != nil {
			if errors.IsNotFound(err) {
				return r.createKubernetesNetworkPolicy(logger, dda, builder)
			}

			return reconcile.Result{}, err
		}

		result, err = r.updateKubernetesNetworkPolicy(logger, dda, policy, builder)
		if err != nil {
			return result, err
		}

		if r.options.SupportCilium {
			err = r.cleanupCiliumNetworkPolicy(logger, dda, policyName)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	case datadoghqv1alpha1.NetworkPolicyFlavorCilium:
		if !r.options.SupportCilium {
			return reconcile.Result{}, fmt.Errorf("cilium network policy support is not enabled in the operator")
		}

		policy := cilium.EmptyCiliumUnstructuredPolicy()
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: policyName, Namespace: dda.Namespace}, policy)
		if err != nil {
			if errors.IsNotFound(err) {
				return r.createCiliumNetworkPolicy(logger, dda, builder)
			}

			return reconcile.Result{}, err
		}

		result, err = r.updateCiliumNetworkPolicy(logger, dda, policy, builder)
		if err != nil {
			return result, err
		}

		err = r.cleanupKubernetesNetworkPolicy(logger, dda, policyName)
		if err != nil {
			return reconcile.Result{}, err
		}
	default:
		return reconcile.Result{}, fmt.Errorf("invalid network policy flavor: %q", policySpec.Flavor)
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) cleanupNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name string) (reconcile.Result, error) {
	var err error

	err = r.cleanupKubernetesNetworkPolicy(logger, dda, name)
	if err != nil {
		return reconcile.Result{}, err
	}

	if r.options.SupportCilium {
		err = r.cleanupCiliumNetworkPolicy(logger, dda, name)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) cleanupKubernetesNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name string) error {
	policy := &networkingv1.NetworkPolicy{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: dda.Namespace}, policy)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		return err
	}

	if !CheckOwnerReference(dda, policy) {
		return nil
	}

	logger.V(1).Info("deleteNetworkPolicy", "networkPolicy.name", policy.Name, "networkPolicy.Namespace", policy.Namespace, "networkPolicy.Flavor", datadoghqv1alpha1.NetworkPolicyFlavorKubernetes)
	event := buildEventInfo(policy.Name, policy.Namespace, networkPolicyKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)

	return r.client.Delete(context.TODO(), policy)
}

func (r *Reconciler) cleanupCiliumNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name string) error {
	policy := cilium.EmptyCiliumUnstructuredPolicy()
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: dda.Namespace}, policy)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		return err
	}

	if !CheckOwnerReference(dda, policy) {
		return nil
	}

	logger.V(1).Info("deleteNetworkPolicy", "networkPolicy.name", policy.GetName(), "networkPolicy.Namespace", policy.GetNamespace(), "networkPolicy.Flavor", datadoghqv1alpha1.NetworkPolicyFlavorCilium)
	event := buildEventInfo(policy.GetName(), policy.GetNamespace(), ciliumNetworkPolicyKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)

	return r.client.Delete(context.TODO(), policy)
}

func (r *Reconciler) createKubernetesNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, builder networkPolicyBuilder) (reconcile.Result, error) {
	policy := builder.BuildKubernetesPolicy()

	err := controllerutil.SetControllerReference(dda, policy, r.scheme)
	if err != nil {
		return reconcile.Result{}, err
	}

	logger.V(1).Info("createNetworkPolicy", "networkPolicy.name", policy.Name, "networkPolicy.Namespace", policy.Namespace, "networkPolicy.Flavor", datadoghqv1alpha1.NetworkPolicyFlavorKubernetes)
	event := buildEventInfo(policy.Name, policy.Namespace, networkPolicyKind, datadog.CreationEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{}, r.client.Create(context.TODO(), policy)
}

func (r *Reconciler) updateKubernetesNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, policy *networkingv1.NetworkPolicy, builder networkPolicyBuilder) (reconcile.Result, error) {
	newPolicy := builder.BuildKubernetesPolicy()

	if !apiequality.Semantic.DeepEqual(newPolicy.Spec, policy.Spec) {
		logger.V(1).Info("updateNetworkPolicy", "networkPolicy.name", policy.Name, "networkPolicy.Namespace", policy.Namespace, "networkPolicy.Flavor", datadoghqv1alpha1.NetworkPolicyFlavorCilium)

		if err := kubernetes.UpdateFromObject(context.TODO(), r.client, newPolicy, policy.ObjectMeta); err != nil {
			return reconcile.Result{}, err
		}

		event := buildEventInfo(newPolicy.Name, newPolicy.Namespace, ciliumNetworkPolicyKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) createCiliumNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, builder networkPolicyBuilder) (reconcile.Result, error) {
	policy := builder.BuildCiliumPolicy()

	err := controllerutil.SetControllerReference(dda, policy, r.scheme)
	if err != nil {
		return reconcile.Result{}, err
	}

	logger.V(1).Info("createNetworkPolicy", "networkPolicy.name", policy.GetName(), "networkPolicy.Namespace", policy.GetNamespace(), "networkPolicy.Flavor", datadoghqv1alpha1.NetworkPolicyFlavorCilium)
	event := buildEventInfo(policy.GetName(), policy.GetNamespace(), ciliumNetworkPolicyKind, datadog.CreationEvent)
	r.recordEvent(dda, event)

	unstructured := cilium.EmptyCiliumUnstructuredPolicy()
	unstructured.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(policy)
	if err != nil {
		return reconcile.Result{}, err
	}
	unstructured.SetGroupVersionKind(cilium.GroupVersionCiliumNetworkPolicyKind())

	return reconcile.Result{}, r.client.Create(context.TODO(), unstructured)
}

func (r *Reconciler) updateCiliumNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, policy *unstructured.Unstructured, builder networkPolicyBuilder) (reconcile.Result, error) {
	newPolicy := builder.BuildCiliumPolicy()

	var (
		newUnstructured map[string]interface{}
		err             error
	)

	newUnstructured, err = runtime.DefaultUnstructuredConverter.ToUnstructured(newPolicy)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !apiequality.Semantic.DeepEqual(newUnstructured["specs"], policy.Object["specs"]) {
		logger.V(1).Info("updateNetworkPolicy", "networkPolicy.name", policy.GetName(), "networkPolicy.Namespace", policy.GetNamespace(), "networkPolicy.Flavor", datadoghqv1alpha1.NetworkPolicyFlavorCilium)

		newUnstructuredPolicy := cilium.EmptyCiliumUnstructuredPolicy()
		newUnstructuredPolicy.Object = newUnstructured
		newUnstructuredPolicy.SetGroupVersionKind(cilium.GroupVersionCiliumNetworkPolicyKind())
		newUnstructuredPolicy.SetResourceVersion(policy.GetResourceVersion())

		err := r.client.Update(context.TODO(), newUnstructuredPolicy)
		if err != nil {
			return reconcile.Result{}, err
		}

		event := buildEventInfo(newPolicy.GetName(), newPolicy.GetNamespace(), ciliumNetworkPolicyKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}

	return reconcile.Result{}, nil
}

func ciliumEgressMetadataServerRule(b networkPolicyBuilder) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to metadata server",
		EndpointSelector: b.PodSelector(),
		Egress: []cilium.EgressRule{
			{
				ToCIDR: []string{"169.254.169.254/32"},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "80",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

func ciliumEgressDNS(b networkPolicyBuilder) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to DNS",
		EndpointSelector: b.PodSelector(),
		Egress: []cilium.EgressRule{
			{
				ToEndpoints: b.NetworkPolicySpec().DNSSelectorEndpoints,
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "53",
								Protocol: cilium.ProtocolAny,
							},
						},
						Rules: &cilium.L7Rules{
							DNS: []cilium.FQDNSelector{
								{
									MatchPattern: "*",
								},
							},
						},
					},
				},
			},
		},
	}
}

// The agents are susceptible to connect to any pod that would be annotated
// with auto-discovery annotations.
//
// When a user wants to add a check on one of its pod, he needs to
// * annotate its pod
// * add an ingress policy from the agent on its own pod
//
// In order to not ask end-users to inject NetworkPolicy on the agent in the
// agent namespace, the agent must be allowed to probe any pod.
func ciliumEgressChecks(b networkPolicyBuilder) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to anything for checks",
		EndpointSelector: b.PodSelector(),
		Egress: []cilium.EgressRule{
			{
				ToEndpoints: []metav1.LabelSelector{
					{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "k8s:io.kubernetes.pod.namespace",
								Operator: "Exists",
							},
						},
					},
				},
			},
		},
	}
}
