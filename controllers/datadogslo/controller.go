// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2023 Datadog, Inc.

package datadogslo

import (
	"context"
	"time"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-operator/pkg/datadogclient"

	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	"github.com/DataDog/datadog-operator/controllers/finalizer"
	"github.com/DataDog/datadog-operator/controllers/utils"
	ctrUtils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	defaultRequeuePeriod    = 60 * time.Second
	defaultErrRequeuePeriod = 5 * time.Second
	datadogSLOKind          = "DatadogSLO"
	datadogSLOFinalizer     = "finalizer.slo.datadoghq.com"
)

type Reconciler struct {
	client        client.Client
	datadogClient *datadogapiclientv1.APIClient
	datadogAuth   context.Context
	versionInfo   *version.Info
	log           logr.Logger
	recorder      record.EventRecorder
}

func NewReconciler(client client.Client, datadogClient datadogclient.DatadogClient, versionInfo *version.Info, log logr.Logger, recorder record.EventRecorder) *Reconciler {
	return &Reconciler{
		client:        client,
		datadogClient: datadogClient.Client,
		datadogAuth:   datadogClient.Auth,
		versionInfo:   versionInfo,
		log:           log,
		recorder:      recorder,
	}
}

var _ reconcile.Reconciler = (*Reconciler)(nil)

func (r *Reconciler) Reconcile(_ context.Context, req reconcile.Request) (reconcile.Result, error) {
	res, err := r.internalReconcile(req)
	return res, err
}

func (r *Reconciler) internalReconcile(req reconcile.Request) (reconcile.Result, error) {
	logger := r.log.WithValues("datadogslo", req.NamespacedName)
	logger.Info("Reconciling Datadog SLO", "version", r.versionInfo.String())
	now := metav1.NewTime(time.Now())

	// Get instance
	instance := &v1alpha1.DatadogSLO{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
	}

	final := finalizer.NewFinalizer(
		logger,
		r.client,
		r.deleteExternalResource(instance, logger),
		defaultRequeuePeriod,
		defaultErrRequeuePeriod,
	)
	if result, err := final.HandleFinalizer(context.TODO(), instance, instance.Status.ID, datadogSLOFinalizer); ctrUtils.ShouldReturn(result, err) {
		return result, err
	}

	status := instance.Status.DeepCopy()
	statusSpecHash := instance.Status.CurrentHash

	// Validate the SLO spec
	if err := v1alpha1.IsValidDatadogSLO(&instance.Spec); err != nil {
		logger.Error(err, "invalid SLO")
		updateErrStatus(status, now, v1alpha1.DatadogSLOSyncStatusValidateError, "ValidatingSLO", err)
		return r.updateStatus(instance, status)
	}

	instanceSpecHash, err := comparison.GenerateMD5ForSpec(&instance.Spec)
	if err != nil {
		logger.Error(err, "error generating hash")
		updateErrStatus(status, now, v1alpha1.DatadogSLOSyncStatusUpdateError, "GeneratingSLOSpecHash", err)
		return r.updateStatus(instance, status)
	}

	if instance.Status.ID == "" {
		if err = r.create(instance, status, now, instanceSpecHash); err != nil {
			logger.Error(err, "error creating SLO")
			updateErrStatus(status, now, v1alpha1.DatadogSLOSyncStatusCreateError, "CreatingSLO", err)
			if res, updErr := r.updateStatus(instance, status); ctrUtils.ShouldReturn(res, updErr) {
				return res, updErr
			}
			return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, err
		}
		return r.updateStatus(instance, status)
	}

	// Check if instance needs to be updated
	if instanceSpecHash != statusSpecHash {
		// Update action
		if err = r.update(instance, status, now); err != nil {
			logger.Error(err, "error updating SLO", "SLO ID", instance.Status.ID)
			updateErrStatus(status, now, v1alpha1.DatadogSLOSyncStatusCreateError, "UpdatingSLO", err)
			if res, updErr := r.updateStatus(instance, status); ctrUtils.ShouldReturn(res, updErr) {
				return res, updErr
			}
			return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, err
		}
	}

	status.CurrentHash = instanceSpecHash
	return r.updateStatus(instance, status)
}

func updateErrStatus(status *v1alpha1.DatadogSLOStatus, now metav1.Time, syncStatus v1alpha1.DatadogSLOSyncStatus, reason string, err error) {
	condition.UpdateFailureStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeError, reason, err)
	status.SyncStatus = syncStatus
}

func (r *Reconciler) deleteExternalResource(instance *v1alpha1.DatadogSLO, logger logr.Logger) finalizer.ExternalResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		if !instance.Status.ManagedByDatadogOperator {
			return nil
		}
		kind := k8sObj.GetObjectKind().GroupVersionKind().Kind
		if err := deleteSLO(r.datadogAuth, r.datadogClient, datadogID); err != nil {
			logger.Error(err, "error deleting SLO", "kind", kind, "ID", datadogID)
			return err
		}
		logger.Info("Successfully delete", "kind", kind, "ID", datadogID)
		r.recordEvent(instance, buildEventInfo(k8sObj.GetName(), k8sObj.GetNamespace(), datadog.DeletionEvent))
		return nil
	}
}

func (r *Reconciler) updateStatus(crdSLO *v1alpha1.DatadogSLO, status *v1alpha1.DatadogSLOStatus) (ctrl.Result, error) {
	if !apiequality.Semantic.DeepEqual(&crdSLO.Status, status) {
		crdSLO.Status = *status
		if err := r.client.Status().Update(context.TODO(), crdSLO); err != nil {
			if apierrors.IsConflict(err) {
				r.log.Error(err, "unable to update DatadogSLO status due to update conflict")
				return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, nil
			}
			r.log.Error(err, "unable to update DatadogSLO status")
			return ctrl.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) create(crdSLO *v1alpha1.DatadogSLO, status *v1alpha1.DatadogSLOStatus, now metav1.Time, hash string) error {
	r.log.V(1).Info("SLO ID is not set; creating SLO in Datadog")

	// Create SLO in Datadog
	createdSLO, err := createSLO(r.datadogAuth, r.datadogClient, crdSLO)
	if err != nil {
		return err
	}

	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeCreated, metav1.ConditionTrue, "CreatingSLO", "DatadogSLO Created")
	creator := createdSLO.GetCreator()
	createdTime := metav1.Unix(createdSLO.GetCreatedAt(), 0)

	status.SyncStatus = v1alpha1.DatadogSLOSyncStatusOK
	status.ID = createdSLO.GetId()
	status.Creator = creator.GetEmail()
	status.Created = &createdTime
	status.ManagedByDatadogOperator = true
	status.CurrentHash = hash

	r.log.Info("Created a new DatadogSLO", "SLO Namespace", crdSLO.Namespace, "SLO Name", crdSLO.Name) //, "SLO ID", createdSLO.GetId())
	r.recordEvent(crdSLO, buildEventInfo(crdSLO.Name, crdSLO.Namespace, datadog.CreationEvent))

	return nil
}

func (r *Reconciler) update(crdSLO *v1alpha1.DatadogSLO, status *v1alpha1.DatadogSLOStatus, now metav1.Time) error {
	// Update SLO in Datadog
	if _, err := updateSLO(r.datadogAuth, r.datadogClient, crdSLO); err != nil {
		status.SyncStatus = v1alpha1.DatadogSLOSyncStatusUpdateError
		return err
	}
	r.recordEvent(crdSLO, buildEventInfo(crdSLO.Name, crdSLO.Namespace, datadog.UpdateEvent))

	// Set Updated Condition
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeUpdated, metav1.ConditionTrue, "UpdatingSLO", "DatadogSLO Updated")
	status.SyncStatus = v1alpha1.DatadogSLOSyncStatusOK

	r.log.Info("Updated DatadogSLO", "SLO Namespace", crdSLO.Namespace, "SLO Name", crdSLO.Name, "SLO ID", crdSLO.Status.ID)
	return nil
}

// buildEventInfo creates a new EventInfo instance.
func buildEventInfo(name, ns string, eventType datadog.EventType) utils.EventInfo {
	return utils.BuildEventInfo(name, ns, datadogSLOKind, eventType)
}

// recordEvent wraps the manager event recorder.
func (r *Reconciler) recordEvent(slo runtime.Object, info utils.EventInfo) {
	r.recorder.Event(slo, corev1.EventTypeNormal, info.GetReason(), info.GetMessage())
}
