// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// Reconciler is the internal reconciler for a DatadogAgentInternal object
type Reconciler struct {
	options      datadogagent.ReconcilerOptions
	client       client.Client
	platformInfo kubernetes.PlatformInfo
	scheme       *runtime.Scheme
	log          logr.Logger
	recorder     record.EventRecorder
	forwarders   datadog.MetricForwardersManager
}

// NewReconciler returns a new Reconciler object
func NewReconciler(options datadogagent.ReconcilerOptions, client client.Client, platformInfo kubernetes.PlatformInfo, scheme *runtime.Scheme, log logr.Logger,
	recorder record.EventRecorder, metricForwardersMgr datadog.MetricForwardersManager) (*Reconciler, error) {
	reconciler := &Reconciler{
		options:      options,
		client:       client,
		platformInfo: platformInfo,
		scheme:       scheme,
		log:          log,
		recorder:     recorder,
		forwarders:   metricForwardersMgr,
	}
	return reconciler, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DatadogAgentInternal object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.log.WithValues("datadogagentinternal", req.NamespacedName)
	r.log.Info("Reconciling DatadogAgentInternal")

	// Fetch the DatadogAgent instance
	instance := &v1alpha1.DatadogAgentInternal{}
	var result reconcile.Result
	err := r.client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return result, nil
		}
		// Error reading the object - requeue the request.
		return result, err
	}

	// TODO: finalizer

	instanceCopy := instance.DeepCopy()

	// fake DDA
	fakeDDA := &v2alpha1.DatadogAgent{
		ObjectMeta: instanceCopy.ObjectMeta,
		Spec:       instanceCopy.Spec,
	}

	ddaReconciler, err := datadogagent.NewReconciler(r.options, r.client, r.platformInfo, r.scheme, r.log, r.recorder, r.forwarders)
	if err != nil {
		return result, err
	}

	defaults.DefaultDatadogAgent(fakeDDA)

	return ddaReconciler.ReconcileInstanceV2(ctx, r.log, fakeDDA)
}
