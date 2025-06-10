// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"reflect"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// DatadogAgentReconciler reconciles a DatadogAgent object.
type DatadogAgentReconciler struct {
	client.Client
	PlatformInfo kubernetes.PlatformInfo
	Log          logr.Logger
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	Options      datadogagent.ReconcilerOptions
	internal     *datadogagent.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagents/finalizers,verbs=get;list;watch;create;update;patch;delete

// RBAC Management
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create

// Finalizer (cluster-scoped resources)
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=deletecollection
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=deletecollection
// +kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=deletecollection

// Configure Admission Controller
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=*
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=get;create;list;watch
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=get;create
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get

// Configure External Metrics server
// +kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=*
// +kubebuilder:rbac:groups=datadoghq.com,resources=watermarkpodautoscalers,verbs=get;list;watch
// +kubebuilder:rbac:groups=external.metrics.k8s.io,resources=*,verbs=get;list;watch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmetrics,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmetrics/status,verbs=update

// Configure Autoscaling product
// (RBACs for events and pods are present below)
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogpodautoscalers,verbs=*
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogpodautoscalers/status,verbs=*
// +kubebuilder:rbac:groups=*,resources=*/scale,verbs=get;update

// Use ExtendedDaemonSet
// +kubebuilder:rbac:groups=datadoghq.com,resources=extendeddaemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=extendeddaemonsetreplicasets,verbs=get;list;watch

// Use CiliumNetworkPolicy
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumnetworkpolicies,verbs=get;list;watch;create;update;patch;delete

// OpenShift
// +kubebuilder:rbac:groups=quota.openshift.io,resources=clusterresourcequotas,verbs=get;list
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=restricted,verbs=use

// +kubebuilder:rbac:urls=/metrics,verbs=get
// +kubebuilder:rbac:urls=/metrics/slis,verbs=get
// +kubebuilder:rbac:groups="",resources=componentstatuses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes/metrics,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes/proxy,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes/spec,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes/stats,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// EKS control plane metrics
// +kubebuilder:rbac:groups="metrics.eks.amazonaws.com",resources=kcm/metrics,verbs=get
// +kubebuilder:rbac:groups="metrics.eks.amazonaws.com",resources=ksh/metrics,verbs=get

// Orchestrator explorer
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=list;watch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=list;watch
// +kubebuilder:rbac:groups=autoscaling.k8s.io,resources=verticalpodautoscalers,verbs=list;watch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=list;watch

// Kubernetes_state_core
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=replicationcontrollers,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=list;watch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=list;watch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=list;watch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses;volumeattachments,verbs=get;list;watch
// +kubebuilder:rbac:groups=autoscaling.k8s.io,resources=verticalpodautoscalers,verbs=list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=list;watch
// +kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=list;watch

// Profiles
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch;patch

// Reconcile loop for DatadogAgent.
func (r *DatadogAgentReconciler) Reconcile(ctx context.Context, dda *v2alpha1.DatadogAgent) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, dda)
}

// SetupWithManager creates a new DatadogAgent controller.
func (r *DatadogAgentReconciler) SetupWithManager(mgr ctrl.Manager, metricForwardersMgr datadog.MetricsForwardersManager) error {
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

	if r.Options.DatadogAgentInternalEnabled {
		builder.Owns(&v1alpha1.DatadogAgentInternal{})
	}

	if r.Options.DatadogAgentProfileEnabled {
		builder.Watches(
			&v1alpha1.DatadogAgentProfile{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsForAllDDAs()),
			ctrlbuilder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool {
					metrics.CleanupMetricsByProfile(e.Object)
					return true
				},
			}),
			))
	}

	// Watch nodes and reconcile all DatadogAgents for node creation, node deletion, and node label change events
	if r.Options.DatadogAgentProfileEnabled || r.Options.IntrospectionEnabled {
		builder.Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsForAllDDAs()),
			ctrlbuilder.WithPredicates(r.enqueueIfNodeLabelsChange()),
		)
	}

	// DatadogAgent is namespaced whereas ClusterRole and ClusterRoleBinding are
	// cluster-scoped. That means that DatadogAgent cannot be their owner, and
	// we cannot use .Owns().
	handlerEnqueue := handler.EnqueueRequestsFromMapFunc(enqueueIfOwnedByDatadogAgent)
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

	or := reconcile.AsReconciler[*v2alpha1.DatadogAgent](r.Client, r)
	if err := builder.For(&v2alpha1.DatadogAgent{}, builderOptions...).WithEventFilter(predicate.GenerationChangedPredicate{}).Complete(or); err != nil {
		return err
	}

	internal, err := datadogagent.NewReconciler(r.Options, r.Client, r.PlatformInfo, r.Scheme, r.Log, r.Recorder, metricForwardersMgr)
	if err != nil {
		return err
	}
	r.internal = internal

	return nil
}

func enqueueIfOwnedByDatadogAgent(ctx context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()

	if labels[kubernetes.AppKubernetesManageByLabelKey] != "datadog-operator" {
		return nil
	}

	partOfLabelVal := object.PartOfLabelValue{Value: labels[kubernetes.AppKubernetesPartOfLabelKey]}
	owner := partOfLabelVal.NamespacedName()

	return []reconcile.Request{{NamespacedName: owner}}
}

func (r *DatadogAgentReconciler) enqueueIfNodeLabelsChange() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !reflect.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels())
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}
}

func (r *DatadogAgentReconciler) enqueueRequestsForAllDDAs() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		var requests []reconcile.Request

		ddaList := v2alpha1.DatadogAgentList{}
		// Should we rather use passed context here?
		// if err := r.List(ctx, &ddaList); err != nil {
		if err := r.List(context.Background(), &ddaList); err != nil {
			return requests
		}

		for _, dda := range ddaList.Items {
			requests = append(
				requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: dda.Namespace,
						Name:      dda.Name,
					},
				},
			)
		}

		return requests
	}
}
