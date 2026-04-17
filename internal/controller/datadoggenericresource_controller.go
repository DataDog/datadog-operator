// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ddgr "github.com/DataDog/datadog-operator/internal/controller/datadoggenericresource"
	"github.com/DataDog/datadog-operator/pkg/config"
)

// DatadogGenericResourceReconciler reconciles a DatadogGenericResource object
type DatadogGenericResourceReconciler struct {
	Client   client.Client
	Creds    config.Creds
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	internal *ddgr.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadoggenericresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadoggenericresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadoggenericresources/finalizers,verbs=get;list;watch;create;update;patch;delete

func (r *DatadogGenericResourceReconciler) Reconcile(ctx context.Context, instance *v1alpha1.DatadogGenericResource) (ctrl.Result, error) {
	return r.internal.ReconcileInstance(ctx, instance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatadogGenericResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	internal, err := ddgr.NewReconciler(r.Client, r.Creds, r.Scheme, r.Log, r.Recorder)
	if err != nil {
		return err
	}
	r.internal = internal

	or := reconcile.AsReconciler[*v1alpha1.DatadogGenericResource](r.Client, r)
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DatadogGenericResource{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		// WithLogConstructor replaces the default log constructor. The default one adds
		// both a nested "DatadogGenericResource":{name, namespace} object AND flat
		// "namespace"/"name" fields, causing duplication. This constructor emits only
		// flat fields, matching the format of other controllers.
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			log := mgr.GetLogger().WithName("controllers").WithName("DatadogGenericResource")
			if req != nil {
				log = log.WithValues("namespace", req.Namespace, "name", req.Name)
			}
			return log
		}).
		Complete(or)
}

// Callback function for credential change from credential manager
func (r *DatadogGenericResourceReconciler) onCredentialChange(newCreds config.Creds) error {
	return r.internal.UpdateDatadogClient(newCreds)
}
