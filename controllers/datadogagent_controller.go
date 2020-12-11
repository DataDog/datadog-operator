// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/version"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// DatadogAgentReconciler reconciles a DatadogAgent object
type DatadogAgentReconciler struct {
	client.Client
	VersionInfo *version.Info
	Log         logr.Logger
	Scheme      *runtime.Scheme
	Recorder    record.EventRecorder
	Options     datadogagent.ReconcilerOptions
	internal    *datadogagent.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagents/finalizers,verbs=get;list;watch;create;update;patch;delete

// RBAC Management
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=*
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=*
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=*
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=*
// +kubebuilder:rbac:groups=roles.rbac.authorization.k8s.io,resources=clusterroles,verbs=*
// +kubebuilder:rbac:groups=roles.rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=*
// +kubebuilder:rbac:groups=roles.rbac.authorization.k8s.io,resources=roles,verbs=*
// +kubebuilder:rbac:groups=roles.rbac.authorization.k8s.io,resources=rolebindings,verbs=*
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=clusterroles,verbs=*
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=clusterrolebindings,verbs=*
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=roles,verbs=*
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=rolebindings,verbs=*

// Configure Admission Controller
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=*
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get

// Configure External Metrics server
// +kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=*
// +kubebuilder:rbac:groups=datadoghq.com,resources=watermarkpodautoscalers,verbs=get;list;watch

// Use ExtendedDaemonSet
// +kubebuilder:rbac:groups=datadoghq.com,resources=extendeddaemonsets,verbs=*

// OpenShift
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=restricted,verbs=use
// +kubebuilder:rbac:groups=quota.openshift.io,resources=clusterresourcequotas,verbs=get;list
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=restricted,verbs=use

// +kubebuilder:rbac:urls=/metrics,verbs=get
// +kubebuilder:rbac:groups="",resources=componentstatuses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes/metrics,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes/proxy,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes/spec,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes/stats,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=*
// +kubebuilder:rbac:groups="",resources=secrets,verbs=*
// +kubebuilder:rbac:groups="",resources=pods,verbs=*
// +kubebuilder:rbac:groups="",resources=services,verbs=*
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=*
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=*
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=*
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=*
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=*
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=*
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=*

// Reconcile loop for DatadogAgent
func (r *DatadogAgentReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	return r.internal.Reconcile(context.Background(), req)
}

// SetupWithManager creates a new DatadogAgent controller
func (r *DatadogAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	metricForwarder := datadog.NewForwardersManager(r.Client)
	internal, err := datadogagent.NewReconciler(r.Options, r.Client, r.VersionInfo, r.Scheme, r.Log, r.Recorder, metricForwarder)
	if err != nil {
		return err
	}
	r.internal = internal

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.DatadogAgent{}, builder.WithPredicates(predicate.Funcs{
			// On `DatadogAgent` object creation, we register a metrics forwarder for it
			CreateFunc: func(e event.CreateEvent) bool {
				metricForwarder.Register(e.Meta)
				return true
			},
		})).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&networkingv1.NetworkPolicy{})

	if r.Options.SupportExtendedDaemonset {
		builder = builder.Owns(&edsdatadoghqv1alpha1.ExtendedDaemonSet{})
	}

	err = builder.Complete(r)
	if err != nil {
		return err
	}

	return nil
}
