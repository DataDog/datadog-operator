// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogslo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/internal/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/config"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

const (
	defaultRequeuePeriod    = 60 * time.Second
	defaultErrRequeuePeriod = 5 * time.Second
	defaultForceSyncPeriod  = 60 * time.Minute
	datadogSLOKind          = "DatadogSLO"
	datadogSLOFinalizer     = "finalizer.slo.datadoghq.com"
)

type Reconciler struct {
	client        client.Client
	datadogClient *datadogV1.ServiceLevelObjectivesApi
	apiURL        *datadogclient.ParsedAPIURL
	credsManager  *config.CredentialManager
	log           logr.Logger
	recorder      record.EventRecorder
}

func NewReconciler(client client.Client, credsManager *config.CredentialManager, log logr.Logger, recorder record.EventRecorder) (*Reconciler, error) {
	apiURL, err := datadogclient.ParseURL(log)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API URL: %w", err)
	}

	return &Reconciler{
		client:        client,
		datadogClient: datadogclient.InitSLOClient(),
		apiURL:        apiURL,
		credsManager:  credsManager,
		log:           log,
		recorder:      recorder,
	}, nil
}

var _ reconcile.Reconciler = (*Reconciler)(nil)

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	res, err := r.internalReconcile(ctx, req)
	return res, err
}

func (r *Reconciler) internalReconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := r.log.WithValues("datadogslo", req.NamespacedName)
	logger.Info("Reconciling Datadog SLO")
	now := metav1.NewTime(time.Now())

	// Get instance
	instance := &v1alpha1.DatadogSLO{}
	var result ctrl.Result
	var err error
	if err = r.client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
	}

	// Get fresh credentials and create auth context for this reconcile
	creds, err := r.credsManager.GetCredentials()
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get credentials: %w", err)
	}
	datadogAuth := datadogclient.GetAuth(creds, r.apiURL)

	final := finalizer.NewFinalizer(
		logger,
		r.client,
		r.deleteResource(logger, instance, datadogAuth),
		defaultRequeuePeriod,
		defaultErrRequeuePeriod,
	)
	if result, err = final.HandleFinalizer(ctx, instance, instance.Status.ID, datadogSLOFinalizer); ctrutils.ShouldReturn(result, err) {
		return result, err
	}

	status := instance.Status.DeepCopy()
	statusSpecHash := instance.Status.CurrentHash

	// Validate the SLO spec
	if err = v1alpha1.IsValidDatadogSLO(&instance.Spec); err != nil {
		logger.Error(err, "invalid SLO")
		updateErrStatus(status, now, v1alpha1.DatadogSLOSyncStatusValidateError, "ValidatingSLO", err)
		return r.updateStatusIfNeeded(logger, instance, status, result)
	}

	instanceSpecHash, err := comparison.GenerateMD5ForSpec(&instance.Spec)
	if err != nil {
		logger.Error(err, "error generating hash")
		updateErrStatus(status, now, v1alpha1.DatadogSLOSyncStatusUpdateError, "GeneratingSLOSpecHash", err)
		return r.updateStatusIfNeeded(logger, instance, status, result)
	}

	shouldCreate := false
	shouldUpdate := false

	if instance.Status.ID == "" {
		shouldCreate = true
	} else {
		if instanceSpecHash != statusSpecHash {
			shouldUpdate = true
		} else if instance.Status.LastForceSyncTime == nil || (defaultForceSyncPeriod-now.Sub(instance.Status.LastForceSyncTime.Time)) <= 0 {
			// Periodically force a sync with the API SLO to ensure parity
			// Get SLO to make sure it exists before trying any updates. If it doesn't, set shouldCreate
			_, err = r.get(datadogAuth, instance)
			if err != nil {
				logger.Error(err, "error getting SLO", "SLO ID", instance.Status.ID)
				if strings.Contains(err.Error(), ctrutils.NotFoundString) {
					shouldCreate = true
				}
			} else {
				shouldUpdate = true
			}
			status.LastForceSyncTime = &now
		}
	}

	if shouldCreate || shouldUpdate {
		// Check that required tags are present
		tagsUpdated, err := r.checkRequiredTags(logger, instance)
		if err != nil {
			result.RequeueAfter = defaultErrRequeuePeriod
			return r.updateStatusIfNeeded(logger, instance, status, result)
		} else if tagsUpdated {
			// A reconcile is triggered by the update
			return r.updateStatusIfNeeded(logger, instance, status, result)
		}

		if shouldCreate {
			err = r.create(datadogAuth, logger, instance, status, now, instanceSpecHash)
		} else if shouldUpdate {
			err = r.update(datadogAuth, logger, instance, status, now, instanceSpecHash)
		}

		if err != nil {
			result.RequeueAfter = defaultErrRequeuePeriod
		}
	}

	// If reconcile was successful and uneventful, requeue with period defaultRequeuePeriod
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}

	return r.updateStatusIfNeeded(logger, instance, status, result)
}

func (r *Reconciler) checkRequiredTags(logger logr.Logger, instance *v1alpha1.DatadogSLO) (bool, error) {
	if instance.Spec.ControllerOptions != nil && apiutils.BoolValue(instance.Spec.ControllerOptions.DisableRequiredTags) {
		return false, nil
	}

	tags := instance.Spec.Tags
	tagsToAdd := utils.GetTagsToAdd(instance.Spec.Tags)

	if len(tagsToAdd) > 0 {
		tags = append(tags, tagsToAdd...)
		instance.Spec.Tags = tags
		err := r.client.Update(context.TODO(), instance)
		if err != nil {
			logger.Error(err, "failed to update DatadogSLO with required tags")

			return false, err
		}
		logger.Info("Added required tags", "SLO ID", instance.Status.ID)
		return true, nil
	}

	return false, nil
}

// TODO implement when /search endpoint has been updated
// func updateSLOState(logger logr.Logger, instance *v1alpha1.DatadogSLO, sloHistory *datadogV1.SLOHistoryResponseData, status *v1alpha1.DatadogSLOStatus, now metav1.Time, err error) {
// 	if err != nil {
// 		status.SLIValue = nil
// 		status.ErrorBudgetRemaining = nil
// 		return
// 	}
// 	rawSLIVal := sloHistory.Overall.SliValue.Get()
// 	if rawSLIVal == nil && len(sloHistory.Overall.Errors) > 0 {
// 		logger.Info("Problem with Datadog SLO", "error message", sloHistory.Overall.Errors[0].ErrorMessage, "SLO ID", instance.Status.ID)
// 		status.Message = fmt.Sprintf("SLO Error: %s", sloHistory.Overall.Errors[0].ErrorMessage)
// 		return
// 	}
// 	sliVal := fmt.Sprintf("%.2f", *rawSLIVal)
// 	status.SLIValue = &sliVal
// 	// There should be only one element in map
// 	for _, v := range sloHistory.Overall.ErrorBudgetRemaining {
// 		ebr := fmt.Sprintf("%.2f", v)
// 		status.ErrorBudgetRemaining = &ebr
// 	}
// }

func updateErrStatus(status *v1alpha1.DatadogSLOStatus, now metav1.Time, syncStatus v1alpha1.DatadogSLOSyncStatus, reason string, err error) {
	condition.UpdateFailureStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeError, reason, err)
	status.SyncStatus = syncStatus
}

func (r *Reconciler) updateStatusIfNeeded(logger logr.Logger, instance *v1alpha1.DatadogSLO, status *v1alpha1.DatadogSLOStatus, result ctrl.Result) (ctrl.Result, error) {
	if !apiequality.Semantic.DeepEqual(&instance.Status, status) {
		instance.Status = *status
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			if apierrors.IsConflict(err) {
				logger.Error(err, "unable to update DatadogSLO status due to update conflict")
				return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, nil
			}
			logger.Error(err, "unable to update DatadogSLO status")
			return ctrl.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod}, err
		}
	}
	return result, nil
}

func (r *Reconciler) create(auth context.Context, logger logr.Logger, instance *v1alpha1.DatadogSLO, status *v1alpha1.DatadogSLOStatus, now metav1.Time, hash string) error {
	logger.V(1).Info("SLO ID is not set; creating SLO in Datadog")

	// Create SLO in Datadog
	createdSLO, err := createSLO(auth, r.datadogClient, instance)
	if err != nil {
		logger.Error(err, "error creating SLO")
		updateErrStatus(status, now, v1alpha1.DatadogSLOSyncStatusCreateError, "CreatingSLO", err)
		return err
	}

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeCreated, metav1.ConditionTrue, "CreatingSLO", "DatadogSLO Created")
	creator := createdSLO.GetCreator()
	createdTime := metav1.Unix(createdSLO.GetCreatedAt(), 0)

	status.SyncStatus = v1alpha1.DatadogSLOSyncStatusOK
	status.ID = createdSLO.GetId()
	status.Creator = creator.GetEmail()
	status.Created = &createdTime
	status.LastForceSyncTime = &createdTime
	status.CurrentHash = hash

	logger.Info("Created a new DatadogSLO", "SLO ID", status.ID)
	r.recordEvent(instance, buildEventInfo(instance.Name, instance.Namespace, datadog.CreationEvent))

	return nil
}

func (r *Reconciler) get(auth context.Context, instance *v1alpha1.DatadogSLO) (*datadogV1.SLOResponseData, error) {
	return getSLO(auth, r.datadogClient, instance.Status.ID)
}

func (r *Reconciler) update(auth context.Context, logger logr.Logger, instance *v1alpha1.DatadogSLO, status *v1alpha1.DatadogSLOStatus, now metav1.Time, hash string) error {
	if _, err := updateSLO(auth, r.datadogClient, instance); err != nil {
		logger.Error(err, "error updating SLO", "SLO ID", instance.Status.ID)
		updateErrStatus(status, now, v1alpha1.DatadogSLOSyncStatusUpdateError, "UpdatingSLO", err)
		return err
	}
	r.recordEvent(instance, buildEventInfo(instance.Name, instance.Namespace, datadog.UpdateEvent))

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeUpdated, metav1.ConditionTrue, "UpdatingSLO", "DatadogSLO Updated")
	status.SyncStatus = v1alpha1.DatadogSLOSyncStatusOK
	status.CurrentHash = hash
	status.LastForceSyncTime = &now

	logger.Info("Updated DatadogSLO", "SLO ID", instance.Status.ID)
	return nil
}

func (r *Reconciler) deleteResource(logger logr.Logger, instance *v1alpha1.DatadogSLO, auth context.Context) finalizer.ResourceDeleteFunc {
	return func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		if datadogID != "" {
			kind := k8sObj.GetObjectKind().GroupVersionKind().Kind
			httpErrorCode, err := deleteSLO(auth, r.datadogClient, datadogID)
			if err != nil && httpErrorCode != 404 {
				return err
			} else if httpErrorCode == 404 {
				logger.Info("Object not found, nothing to delete", "kind", kind, "ID", datadogID)
			} else {
				logger.Info("Successfully deleted object", "kind", kind, "ID", datadogID)
			}
		}
		r.recordEvent(instance, buildEventInfo(k8sObj.GetName(), k8sObj.GetNamespace(), datadog.DeletionEvent))
		return nil
	}
}

// buildEventInfo creates a new EventInfo instance.
func buildEventInfo(name, ns string, eventType datadog.EventType) utils.EventInfo {
	return utils.BuildEventInfo(name, ns, datadogSLOKind, eventType)
}

// recordEvent wraps the manager event recorder.
func (r *Reconciler) recordEvent(slo runtime.Object, info utils.EventInfo) {
	r.recorder.Event(slo, corev1.EventTypeNormal, info.GetReason(), info.GetMessage())
}
