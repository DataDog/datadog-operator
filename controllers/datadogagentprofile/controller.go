// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoghq

import (
	"context"
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	profilesV2 "github.com/DataDog/datadog-operator/pkg/profiles"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	defaultRequeuePeriod = 60
)

// Reconciler reconciles a DatadogAgentProfile object
type Reconciler struct {
	client client.Client
	pv2    *profilesV2.ProfilesV2
	scheme *runtime.Scheme
	log    logr.Logger
}

//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles/finalizers,verbs=update

// NewReconciler returns a new Reconciler object
func NewReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger, pv2 *profilesV2.ProfilesV2) (*Reconciler, error) {
	return &Reconciler{
		client: client,
		// datadogAuth:   ddClient.Auth,
		pv2:    pv2,
		scheme: scheme,
		log:    log,
	}, nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// _ = log.FromContext(ctx)

	// // TODO(user): your logic here

	// return ctrl.Result{}, nil
	return r.internalReconcile(ctx, req)
}

func (r *Reconciler) internalReconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("datadogagentprofile", req.NamespacedName)
	reqLogger.Info("Reconciling DatadogAgentProfile")

	var result reconcile.Result

	// get existing dda
	ddaList := &datadoghqv2alpha1.DatadogAgentList{}
	err := r.client.List(context.TODO(), ddaList)
	if err != nil {
		return result, err
	}
	if len(ddaList.Items) == 0 {
		return result, fmt.Errorf("unable to find existing DatadogAgent, can't reconcile")
	}
	if len(ddaList.Items) > 1 {
		return result, fmt.Errorf("only one DatadogAgent is allowed per cluster, can't reconcile")
	}

	// get dap
	dap := &datadoghqv1alpha1.DatadogAgentProfile{}
	err = r.client.Get(ctx, req.NamespacedName, dap)
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

	// merge config
	ddaCopy := ddaList.Items[0].DeepCopy()
	mergedDDA := r.pv2.MergeObjects(r.client, ddaCopy, dap)
	name := profilesV2.GetMergedDDAName(mergedDDA.GetName(), dap.GetName())
	mergedDDA.SetName(name)

	// check that dapAffinity contains a set of requirements
	if dap.Spec.DAPAffinity == nil {
		return result, fmt.Errorf("no dapAffinity specified, can't reconcile")
	}
	if len(dap.Spec.DAPAffinity.DAPNodeAffinity) < 1 {
		return result, fmt.Errorf("dapAffinity must have at least 1 requirement, can't reconcile")
	}

	// combine node affinity
	profilesV2.GenerateNodeAffinity(dap.Spec.DAPAffinity.DAPNodeAffinity, mergedDDA)
	// store config (original dda modified w/selector + dap merged)
	r.pv2.Add(mergedDDA)

	// opposite dap affinity
	r.pv2.GenerateDefaultAffinity(name, dap.Spec.DAPAffinity)
	// create default dda
	var ddaDefault *datadoghqv2alpha1.DatadogAgent
	if dda := r.pv2.GetDDAByNamespacedName(req.NamespacedName); dda != nil {
		ddaDefault = dda
	} else {
		ddaDefault = ddaList.Items[0].DeepCopy()
	}
	// add opposite affinity to non-merged dda
	r.pv2.AddDefaultAffinity(ddaDefault)
	r.pv2.Add(ddaDefault)

	// If reconcile was successful, requeue with period defaultRequeuePeriod
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}

	// Update the status
	// return r.updateStatusIfNeeded(r.log, instance, now, newStatus, err, result)
	return result, nil
}
