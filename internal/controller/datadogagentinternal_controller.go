// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// DatadogAgentInternalReconciler reconciles a DatadogAgentInternal object.
type DatadogAgentInternalReconciler struct {
	client.Client
	PlatformInfo kubernetes.PlatformInfo
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	Options      datadogagentinternal.ReconcilerOptions
	internal     *datadogagentinternal.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile loop for DatadogAgent.
func (r *DatadogAgentInternalReconciler) Reconcile(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal) (ctrl.Result, error) {
	// Get the logger from context (already has namespace, name, reconcileID from controller-runtime)
	// Add our controller name and kind - this enriched logger will be available to all downstream functions
	logger := ctrl.LoggerFrom(ctx).WithName("controllers").WithName("DatadogAgentInternal").WithValues("kind", ddai.Kind)
	ctx = ctrl.LoggerInto(ctx, logger)
	return r.internal.Reconcile(ctx, ddai)
}

// SetupWithManager creates a new DatadogAgent controller.
func (r *DatadogAgentInternalReconciler) SetupWithManager(mgr ctrl.Manager, metricForwardersMgr datadog.MetricsForwardersManager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&corev1.ServiceAccount{}).
		// We let PlatformInfo supply PDB object based on the current API version
		Owns(r.PlatformInfo.CreatePDBObject()).
		Owns(&networkingv1.NetworkPolicy{})

	// DatadogAgent is namespaced whereas ClusterRole and ClusterRoleBinding are
	// cluster-scoped. That means that DatadogAgent cannot be their owner, and
	// we cannot use .Owns().
	handlerEnqueue := handler.EnqueueRequestsFromMapFunc(enqueueIfOwnedByDatadogAgentInternal)
	builder.Watches(&rbacv1.ClusterRole{}, handlerEnqueue)
	builder.Watches(&rbacv1.ClusterRoleBinding{}, handlerEnqueue)

	if r.Options.ExtendedDaemonsetOptions.Enabled {
		builder = builder.Owns(&edsdatadoghqv1alpha1.ExtendedDaemonSet{})
	}

	if r.Options.SupportCilium {
		policy := &unstructured.Unstructured{}
		policy.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cilium.io",
			Version: "v2",
			Kind:    "CiliumNetworkPolicy",
		})
		builder = builder.Owns(policy)
	}

	var builderOptions []ctrlbuilder.ForOption
	if r.Options.OperatorMetricsEnabled {
		builderOptions = append(builderOptions, ctrlbuilder.WithPredicates(predicate.Funcs{
			// On `DatadogAgent` object creation, we register a metrics forwarder for it.
			CreateFunc: func(e event.CreateEvent) bool {
				metricForwardersMgr.Register(e.Object)
				return true
			},
		}))
	}

	or := reconcile.AsReconciler[*v1alpha1.DatadogAgentInternal](r.Client, r)
	if err := builder.For(&datadoghqv1alpha1.DatadogAgentInternal{}, builderOptions...).WithEventFilter(predicate.GenerationChangedPredicate{}).Complete(or); err != nil {
		return err
	}

	internal, err := datadogagentinternal.NewReconciler(r.Options, r.Client, r.PlatformInfo, r.Scheme, r.Recorder, metricForwardersMgr)
	if err != nil {
		return err
	}
	r.internal = internal

	return nil
}

func enqueueIfOwnedByDatadogAgentInternal(ctx context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()

	if labels[kubernetes.AppKubernetesManageByLabelKey] != "datadog-operator" {
		return nil
	}

	partOfLabelVal := object.PartOfLabelValue{Value: labels[kubernetes.AppKubernetesPartOfLabelKey]}
	owner := partOfLabelVal.NamespacedName()

	return []reconcile.Request{{NamespacedName: owner}}
}
