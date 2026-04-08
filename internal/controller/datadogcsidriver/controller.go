// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	// fieldOwner identifies this controller for SSA
	fieldOwner = "datadogcsidriver-controller"
)

// Reconciler reconciles a DatadogCSIDriver object
type Reconciler struct {
	client   client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	recorder record.EventRecorder
}

// NewReconciler creates a new DatadogCSIDriver reconciler
func NewReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder) *Reconciler {
	return &Reconciler{
		client:   client,
		scheme:   scheme,
		log:      log,
		recorder: recorder,
	}
}

// Reconcile handles the reconciliation loop for DatadogCSIDriver
func (r *Reconciler) Reconcile(ctx context.Context, instance *v1alpha1.DatadogCSIDriver) (result ctrl.Result, retErr error) {
	logger := r.log.WithValues("datadogcsidriver", fmt.Sprintf("%s/%s", instance.Namespace, instance.Name))

	// Handle deletion via finalizer
	if !instance.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, logger, instance)
	}

	// Ensure finalizer is set
	if !controllerutil.ContainsFinalizer(instance, finalizerName) {
		controllerutil.AddFinalizer(instance, finalizerName)
		if err := r.client.Update(ctx, instance); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
		}
	}

	// Deferred SSA status patch — always runs, even on error.
	// Named returns let the defer propagate patch failures so the request is retried.
	defer func() {
		instance.Status.ObservedGeneration = instance.Generation

		r.populateDaemonSetStatus(ctx, instance)

		instance.Status.CSIDriverName = csiDriverName

		statusApply := v1alpha1.DatadogCSIDriver{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "DatadogCSIDriver",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
			Status: instance.Status,
		}
		if err := r.client.Status().Patch(ctx, &statusApply,
			client.Apply, client.FieldOwner(fieldOwner), client.ForceOwnership); err != nil {
			logger.Error(err, "Failed to apply status")
			retErr = errors.Join(retErr, err)
		}
	}()

	logger.Info("Reconciling DatadogCSIDriver")

	// Reset conditions so the SSA patch only contains conditions we explicitly
	// set below. Without this, stale conditions from the fetched object (e.g.
	// from a previous controller version) would be carried forward.
	instance.Status.Conditions = nil

	// Reconcile CSIDriver object (cluster-scoped)
	if err := r.reconcileCSIDriver(ctx, logger, instance); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "CSIDriverError",
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return ctrl.Result{}, err
	}

	// Reconcile DaemonSet (namespaced)
	if err := r.reconcileDaemonSet(ctx, logger, instance); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "DaemonSetError",
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "ReconcileSucceeded",
		Message:            "DatadogCSIDriver reconciled successfully",
		ObservedGeneration: instance.Generation,
	})

	return ctrl.Result{}, nil
}

func (r *Reconciler) handleDeletion(ctx context.Context, logger logr.Logger, instance *v1alpha1.DatadogCSIDriver) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(instance, finalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("Handling deletion, cleaning up CSIDriver object", "csidriver", csiDriverName)

	csiDriver := &storagev1.CSIDriver{}
	err := r.client.Get(ctx, types.NamespacedName{Name: csiDriverName}, csiDriver)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("getting CSIDriver for deletion: %w", err)
		}
		// CSIDriver already gone — nothing to clean up
	} else {
		if err := r.client.Delete(ctx, csiDriver); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("deleting CSIDriver: %w", err)
		}
		logger.Info("Deleted CSIDriver object", "csidriver", csiDriverName)
		r.recorder.Eventf(instance, "Normal", "CSIDriverDeleted", "Deleted CSIDriver %s", csiDriverName)
	}

	controllerutil.RemoveFinalizer(instance, finalizerName)
	if err := r.client.Update(ctx, instance); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileCSIDriver(ctx context.Context, logger logr.Logger, instance *v1alpha1.DatadogCSIDriver) error {
	desired := buildCSIDriverObject(instance)

	current := &storagev1.CSIDriver{}
	err := r.client.Get(ctx, types.NamespacedName{Name: desired.Name}, current)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("getting CSIDriver: %w", err)
		}
		logger.Info("Creating CSIDriver", "csidriver", desired.Name)
		if err := r.client.Create(ctx, desired); err != nil {
			return fmt.Errorf("creating CSIDriver: %w", err)
		}
		return nil
	}

	// Full replacement of spec and labels ensures any external drift is reverted,
	// including fields added manually (e.g. via kubectl edit).
	if !apiequality.Semantic.DeepEqual(current.Spec, desired.Spec) ||
		!apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) {
		logger.Info("Updating CSIDriver to match desired state", "csidriver", desired.Name)
		current.Spec = desired.Spec
		current.Labels = desired.Labels
		if err := r.client.Update(ctx, current); err != nil {
			return fmt.Errorf("updating CSIDriver: %w", err)
		}
	}

	return nil
}

func (r *Reconciler) reconcileDaemonSet(ctx context.Context, logger logr.Logger, instance *v1alpha1.DatadogCSIDriver) error {
	desired := buildDaemonSet(instance)

	if err := controllerutil.SetControllerReference(instance, desired, r.scheme); err != nil {
		return fmt.Errorf("setting owner reference: %w", err)
	}

	nsName := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}
	current := &appsv1.DaemonSet{}
	err := r.client.Get(ctx, nsName, current)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("getting DaemonSet: %w", err)
		}
		logger.Info("Creating DaemonSet", "daemonset", nsName)
		if err := r.client.Create(ctx, desired); err != nil {
			return fmt.Errorf("creating DaemonSet: %w", err)
		}
		return nil
	}

	// Full replacement of spec and labels ensures any external drift is reverted,
	// including fields added manually (e.g. via kubectl edit).
	// The DeepEqual guard avoids unnecessary writes that would trigger the Owns()
	// watch and cause a reconcile loop.
	if !apiequality.Semantic.DeepEqual(current.Spec, desired.Spec) ||
		!apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) {
		logger.Info("Updating DaemonSet to match desired state", "daemonset", nsName)
		current.Spec = desired.Spec
		current.Labels = desired.Labels
		if err := r.client.Update(ctx, current); err != nil {
			return fmt.Errorf("updating DaemonSet: %w", err)
		}
	}

	return nil
}

func (r *Reconciler) populateDaemonSetStatus(ctx context.Context, instance *v1alpha1.DatadogCSIDriver) {
	ds := &appsv1.DaemonSet{}
	err := r.client.Get(ctx, types.NamespacedName{Name: csiDsName, Namespace: instance.Namespace}, ds)
	if err != nil {
		instance.Status.DaemonSet = nil
		return
	}

	now := metav1.NewTime(time.Now())
	instance.Status.DaemonSet = &v2alpha1.DaemonSetStatus{
		Desired:    ds.Status.DesiredNumberScheduled,
		Current:    ds.Status.CurrentNumberScheduled,
		Ready:      ds.Status.NumberReady,
		Available:  ds.Status.NumberAvailable,
		UpToDate:   ds.Status.UpdatedNumberScheduled,
		LastUpdate: &now,
		Status:     getDaemonSetStatusString(ds),
	}
}

func getDaemonSetStatusString(ds *appsv1.DaemonSet) string {
	if ds.Status.DesiredNumberScheduled == ds.Status.NumberReady &&
		ds.Status.DesiredNumberScheduled == ds.Status.UpdatedNumberScheduled {
		return "Running"
	}
	if ds.Status.UpdatedNumberScheduled > 0 && ds.Status.UpdatedNumberScheduled < ds.Status.DesiredNumberScheduled {
		return "Updating"
	}
	if ds.Status.NumberReady == 0 {
		return "Pending"
	}
	return "Progressing"
}
