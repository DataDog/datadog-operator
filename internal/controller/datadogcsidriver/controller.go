// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	// annotation key for spec hash comparison
	csiDriverHashAnnotationKey = "datadoghq.com/csi-driver-spec-hash"
	// annotation key to track the DaemonSet generation we last reconciled
	csiDriverGenerationAnnotationKey = "datadoghq.com/csi-driver-generation"
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
func (r *Reconciler) Reconcile(ctx context.Context, instance *datadoghqv1alpha1.DatadogCSIDriver) (result ctrl.Result, retErr error) {
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

		instance.Status.CSIDriverName = getCSIDriverName(instance)

		statusApply := datadoghqv1alpha1.DatadogCSIDriver{
			TypeMeta: metav1.TypeMeta{
				APIVersion: datadoghqv1alpha1.GroupVersion.String(),
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

func (r *Reconciler) handleDeletion(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogCSIDriver) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(instance, finalizerName) {
		return ctrl.Result{}, nil
	}

	driverName := getCSIDriverName(instance)
	logger.Info("Handling deletion, cleaning up CSIDriver object", "csidriver", driverName)

	csiDriver := &storagev1.CSIDriver{}
	err := r.client.Get(ctx, types.NamespacedName{Name: driverName}, csiDriver)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("getting CSIDriver for deletion: %w", err)
		}
		// CSIDriver already gone — nothing to clean up
	} else {
		if err := r.client.Delete(ctx, csiDriver); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("deleting CSIDriver: %w", err)
		}
		logger.Info("Deleted CSIDriver object", "csidriver", driverName)
		r.recorder.Eventf(instance, "Normal", "CSIDriverDeleted", "Deleted CSIDriver %s", driverName)
	}

	controllerutil.RemoveFinalizer(instance, finalizerName)
	if err := r.client.Update(ctx, instance); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileCSIDriver(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogCSIDriver) error {
	desired := buildCSIDriverObject(instance)
	driverName := desired.Name

	current := &storagev1.CSIDriver{}
	err := r.client.Get(ctx, types.NamespacedName{Name: driverName}, current)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("getting CSIDriver: %w", err)
		}

		// CSIDriver doesn't exist yet — create it
		logger.Info("Creating CSIDriver object", "csidriver", driverName)
		if err := r.client.Create(ctx, desired); err != nil {
			return fmt.Errorf("creating CSIDriver: %w", err)
		}
		r.recorder.Eventf(instance, "Normal", "CSIDriverCreated", "Created CSIDriver %s", driverName)
		return nil
	}

	// CSIDriver exists — ensure our ownership labels are set.
	// This path is hit on first reconcile after adoption of an existing CSIDriver,
	// or if someone manually removed the labels.
	expectedPartOf := object.NewPartOfLabelValue(instance).String()
	if current.Labels == nil ||
		current.Labels[kubernetes.AppKubernetesManageByLabelKey] != "datadog-operator" ||
		current.Labels[kubernetes.AppKubernetesPartOfLabelKey] != expectedPartOf {
		logger.Info("Updating CSIDriver ownership labels", "csidriver", driverName)
		if current.Labels == nil {
			current.Labels = map[string]string{}
		}
		current.Labels[kubernetes.AppKubernetesManageByLabelKey] = "datadog-operator"
		current.Labels[kubernetes.AppKubernetesPartOfLabelKey] = expectedPartOf
		if err := r.client.Update(ctx, current); err != nil {
			return fmt.Errorf("updating CSIDriver labels: %w", err)
		}
	}

	return nil
}

func (r *Reconciler) reconcileDaemonSet(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogCSIDriver) error {
	desired := buildDaemonSet(instance)

	if err := controllerutil.SetControllerReference(instance, desired, r.scheme); err != nil {
		return fmt.Errorf("setting owner reference: %w", err)
	}

	nsName := types.NamespacedName{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}

	current := &appsv1.DaemonSet{}
	err := r.client.Get(ctx, nsName, current)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("getting DaemonSet: %w", err)
		}

		// DaemonSet doesn't exist yet — create it with hash annotation
		if _, hashErr := comparison.SetMD5GenerationAnnotation(&desired.ObjectMeta, desired.Spec, csiDriverHashAnnotationKey); hashErr != nil {
			return fmt.Errorf("generating spec hash: %w", hashErr)
		}
		logger.Info("Creating DaemonSet", "daemonset", nsName)
		if createErr := r.client.Create(ctx, desired); createErr != nil {
			return fmt.Errorf("creating DaemonSet: %w", createErr)
		}
		// Store the generation via metadata-only merge patch (doesn't bump generation).
		// This establishes the baseline for drift detection on subsequent reconciles.
		genPatch := fmt.Appendf(nil, `{"metadata":{"annotations":{%q:%q}}}`,
			csiDriverGenerationAnnotationKey, fmt.Sprintf("%d", desired.Generation))
		if patchErr := r.client.Patch(ctx, desired, client.RawPatch(types.MergePatchType, genPatch)); patchErr != nil {
			return fmt.Errorf("storing generation annotation: %w", patchErr)
		}
		r.recorder.Eventf(instance, "Normal", "DaemonSetCreated", "Created DaemonSet %s", desired.Name)
		return nil
	}

	// DaemonSet exists — check if we need to update it.
	// Preserve the immutable selector from the existing DaemonSet before hashing,
	// so the hash reflects what we'll actually submit to the API server.
	desired.Spec.Selector = current.Spec.Selector
	desired.Spec.Template.Labels = ensureSelectorLabels(current.Spec.Selector, desired.Spec.Template.Labels)

	hash, err := comparison.SetMD5GenerationAnnotation(&desired.ObjectMeta, desired.Spec, csiDriverHashAnnotationKey)
	if err != nil {
		return fmt.Errorf("generating spec hash: %w", err)
	}

	// Decide whether an update is needed using two signals:
	//
	// 1. Spec hash: has the DatadogCSIDriver CR changed, producing a different
	//    desired DaemonSet spec? Detected by comparing the freshly computed hash
	//    against the hash stored in the DaemonSet annotation from the last apply.
	//
	// 2. Generation: has someone manually edited the DaemonSet? The API server
	//    bumps .metadata.generation on every spec change. We store the generation
	//    after each apply, so a mismatch means an external edit occurred.
	//
	// Skip only when BOTH match — our intent is unchanged AND no drift occurred.
	storedGen := current.GetAnnotations()[csiDriverGenerationAnnotationKey]
	currentGen := fmt.Sprintf("%d", current.Generation)
	hashMatch := comparison.IsSameMD5Hash(hash, current.GetAnnotations(), csiDriverHashAnnotationKey)
	genMatch := storedGen == currentGen

	if hashMatch && genMatch {
		// Steady state: CR unchanged, DaemonSet untouched externally — nothing to do.
		return nil
	}

	if hashMatch && !genMatch {
		// Drift detected: CR hasn't changed but someone manually edited the
		// DaemonSet spec (generation bumped without a hash change). Re-apply
		// our desired spec to revert the external modification.
		logger.Info("Drift detected on DaemonSet, reverting to desired state", "daemonset", nsName)
	} else {
		// CR changed: the DatadogCSIDriver spec was updated, producing a new
		// desired DaemonSet spec. Apply the new configuration.
		logger.Info("Updating DaemonSet", "daemonset", nsName)
	}

	current.Spec = desired.Spec
	current.Labels = desired.Labels
	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	current.Annotations[csiDriverHashAnnotationKey] = hash
	if err := r.client.Update(ctx, current); err != nil {
		return fmt.Errorf("updating DaemonSet: %w", err)
	}
	// Store the new generation via metadata-only merge patch (doesn't bump generation).
	// On the next reconcile, if no external edits occur, storedGen == currentGen → skip.
	genPatch := fmt.Appendf(nil, `{"metadata":{"annotations":{%q:%q}}}`,
		csiDriverGenerationAnnotationKey, fmt.Sprintf("%d", current.Generation))
	if err := r.client.Patch(ctx, current, client.RawPatch(types.MergePatchType, genPatch)); err != nil {
		return fmt.Errorf("storing generation annotation: %w", err)
	}
	r.recorder.Eventf(instance, "Normal", "DaemonSetUpdated", "Updated DaemonSet %s", desired.Name)
	return nil
}

func (r *Reconciler) populateDaemonSetStatus(ctx context.Context, instance *datadoghqv1alpha1.DatadogCSIDriver) {
	dsName := fmt.Sprintf("%s-node-server", defaultDaemonSetName)
	ds := &appsv1.DaemonSet{}
	err := r.client.Get(ctx, types.NamespacedName{Name: dsName, Namespace: instance.Namespace}, ds)
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

func ensureSelectorLabels(selector *metav1.LabelSelector, labels map[string]string) map[string]string {
	if selector == nil {
		return labels
	}
	if labels == nil {
		labels = map[string]string{}
	}
	maps.Copy(labels, selector.MatchLabels)
	return labels
}
