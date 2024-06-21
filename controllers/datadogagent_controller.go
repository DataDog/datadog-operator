// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controllers

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// DatadogAgentReconciler reconciles a DatadogAgent object.
type DatadogAgentReconciler struct {
	client.Client
	VersionInfo  *version.Info
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
// +kubebuilder:rbac:groups=roles.rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=roles.rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=roles.rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=roles.rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=pods/exec,verbs=create

// Configure Admission Controller
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=*
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create;get
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create;get
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get

// Configure External Metrics server
// +kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=*
// +kubebuilder:rbac:groups=datadoghq.com,resources=watermarkpodautoscalers,verbs=get;list;watch
// +kubebuilder:rbac:groups=external.metrics.k8s.io,resources=*,verbs=get;list;watch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmetrics,verbs=list;watch;create;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmetrics/status,verbs=update

// Use ExtendedDaemonSet
// +kubebuilder:rbac:groups=datadoghq.com,resources=extendeddaemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=extendeddaemonsetreplicasets,verbs=get

// Use CiliumNetworkPolicy
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumnetworkpolicies,verbs=get;list;watch;create;update;patch;delete

// OpenShift
// +kubebuilder:rbac:groups=quota.openshift.io,resources=clusterresourcequotas,verbs=get;list
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=restricted,verbs=use

// +kubebuilder:rbac:urls=/metrics,verbs=get
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

// Compliance
// +kubebuilder:rbac:groups=policy,resources=podsecuritypolicies,verbs=get;list;watch

// Orchestrator explorer
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=list;watch
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=list;watch
// +kubebuilder:rbac:groups=autoscaling.k8s.io,resources=verticalpodautoscalers,verbs=list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=list;watch

// Kubernetes_state_core
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=replicationcontrollers,verbs=list;watch
// +kubebuilder:rbac:groups=apps;extensions,resources=daemonsets;deployments;replicasets,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=replicationcontrollers,verbs=list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=list;watch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=list;watch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=list;watch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=list;watch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses;volumeattachments,verbs=list;watch
// +kubebuilder:rbac:groups=autoscaling.k8s.io,resources=verticalpodautoscalers,verbs=list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=list;watch
// +kubebuilder:rbac:groups=extensions,resources=customresourcedefinitions,verbs=list;watch
// +kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=list;watch

// Profiles
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch;patch

// Reconcile loop for DatadogAgent.
func (r *DatadogAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(ctx, req)
}

// SetupWithManager creates a new DatadogAgent controller.
func (r *DatadogAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	if r.Options.DatadogAgentProfileEnabled {
		builder.Watches(
			&datadoghqv1alpha1.DatadogAgentProfile{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsForAllDDAs()),
		)
	}

	// Watch nodes and reconcile all DatadogAgents for node creation, node deletion, and node label change events
	if r.Options.V2Enabled {
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

	var metricForwarder datadog.MetricForwardersManager
	var builderOptions []ctrlbuilder.ForOption
	if r.Options.OperatorMetricsEnabled {
		metricForwarder = datadog.NewForwardersManager(r.Client, r.Options.V2Enabled, &r.PlatformInfo)
		builderOptions = append(builderOptions, ctrlbuilder.WithPredicates(predicate.Funcs{
			// On `DatadogAgent` object creation, we register a metrics forwarder for it.
			CreateFunc: func(e event.CreateEvent) bool {
				metricForwarder.Register(e.Object)
				return true
			},
		}))
	}

	if r.Options.V2Enabled {
		if err := builder.For(&datadoghqv2alpha1.DatadogAgent{}, builderOptions...).Complete(r); err != nil {
			return err
		}
	} else {
		if err := builder.For(&datadoghqv1alpha1.DatadogAgent{}, builderOptions...).Complete(r); err != nil {
			return err
		}
	}

	internal, err := datadogagent.NewReconciler(r.Options, r.Client, r.VersionInfo, r.PlatformInfo, r.Scheme, r.Log, r.Recorder, metricForwarder)
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

		ddaList := datadoghqv2alpha1.DatadogAgentList{}
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
