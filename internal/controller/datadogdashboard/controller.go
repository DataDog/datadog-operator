package datadogdashboard

import (
	"context"
	"fmt"
	"os"
	"strconv"
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
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"github.com/DataDog/datadog-operator/internal/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/config"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

const (
	defaultRequeuePeriod             = 60 * time.Second
	defaultErrRequeuePeriod          = 5 * time.Second
	defaultForceSyncPeriod           = 60 * time.Minute
	datadogDashboardKind             = "DatadogDashboard"
	DDDashboardForceSyncPeriodEnvVar = "DD_DASHBOARD_FORCE_SYNC_PERIOD"
)

type Reconciler struct {
	client        client.Client
	datadogClient *datadogV1.DashboardsApi
	datadogAuth   context.Context
	scheme        *runtime.Scheme
	log           logr.Logger
	recorder      record.EventRecorder
}

func NewReconciler(client client.Client, creds config.Creds, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder) (*Reconciler, error) {
	ddClient, err := datadogclient.InitDatadogDashboardClient(log, creds)
	if err != nil {
		return &Reconciler{}, err
	}

	return &Reconciler{
		client:        client,
		datadogClient: ddClient.Client,
		datadogAuth:   ddClient.Auth,
		scheme:        scheme,
		log:           log,
		recorder:      recorder,
	}, nil
}

func (r *Reconciler) UpdateDatadogClient(newCreds config.Creds) error {
	r.log.Info("Recreating Datadog client due to credential change", "reconciler", "DatadogDashboard")
	ddClient, err := datadogclient.InitDatadogDashboardClient(r.log, newCreds)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client in DatadogDashboard: %w", err)
	}
	r.datadogClient = ddClient.Client
	r.datadogAuth = ddClient.Auth

	r.log.Info("Successfully recreated datadog client due to credential change", "reconciler", "DatadogDashboard")

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	return r.internalReconcile(ctx, request)
}

func (r *Reconciler) internalReconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := r.log.WithValues("datadogdashboard", req.NamespacedName)
	logger.Info("Reconciling Datadog Dashboard")
	now := metav1.NewTime(time.Now())

	forceSyncPeriod := defaultForceSyncPeriod

	if userForceSyncPeriod, ok := os.LookupEnv(DDDashboardForceSyncPeriodEnvVar); ok {
		forceSyncPeriodInt, err := strconv.Atoi(userForceSyncPeriod)
		if err != nil {
			logger.Error(err, "Invalid value for dashboard force sync period. Defaulting to 60 minutes.")
		} else {
			logger.V(1).Info("Setting dashboard force sync period", "minutes", forceSyncPeriodInt)
			forceSyncPeriod = time.Duration(forceSyncPeriodInt) * time.Minute
		}
	}

	// Try to get v1alpha1 first, then v1alpha2
	var dashboard interface{}
	var result ctrl.Result
	var err error

	v1alpha1Instance := &v1alpha1.DatadogDashboard{}
	if err = r.client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, v1alpha1Instance); err == nil {
		dashboard = v1alpha1Instance
		logger.V(1).Info("Found v1alpha1 DatadogDashboard")
	} else if apierrors.IsNotFound(err) {
		// Try v1alpha2
		v1alpha2Instance := &v1alpha2.DatadogDashboard{}
		if err = r.client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, v1alpha2Instance); err == nil {
			dashboard = v1alpha2Instance
			logger.V(1).Info("Found v1alpha2 DatadogDashboard")
		} else if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, err
		}
	} else {
		return ctrl.Result{}, err
	}

	// Get the appropriate widget processor
	processor, err := GetWidgetProcessor(dashboard, logger)
	if err != nil {
		logger.Error(err, "Failed to get widget processor")
		return ctrl.Result{}, err
	}

	logger.V(1).Info("Using widget processor", "version", processor.GetAPIVersion())

	// Handle finalizer based on version
	if result, err = r.handleFinalizerVersionAware(logger, dashboard); ctrutils.ShouldReturn(result, err) {
		return result, err
	}

	// Get status and spec hash based on version
	var status interface{}
	var statusSpecHash string
	var instanceSpecHash string

	switch d := dashboard.(type) {
	case *v1alpha1.DatadogDashboard:
		status = d.Status.DeepCopy()
		statusSpecHash = d.Status.CurrentHash
		instanceSpecHash, err = comparison.GenerateMD5ForSpec(&d.Spec)
	case *v1alpha2.DatadogDashboard:
		status = d.Status.DeepCopy()
		statusSpecHash = d.Status.CurrentHash
		instanceSpecHash, err = comparison.GenerateMD5ForSpec(&d.Spec)
	}

	if err != nil {
		logger.Error(err, "error generating hash")
		r.updateErrStatusVersionAware(status, now, "DatadogDashboardSyncStatusUpdateError", "GeneratingDashboardSpecHash", err)
		return r.updateStatusIfNeededVersionAware(logger, dashboard, status, result)
	}

	// Validate using the processor
	if err = processor.ValidateWidgets(dashboard); err != nil {
		logger.Error(err, "invalid Dashboard")
		r.updateErrStatusVersionAware(status, now, "DatadogDashboardSyncStatusValidateError", "ValidatingDashboard", err)
		return r.updateStatusIfNeededVersionAware(logger, dashboard, status, result)
	}

	shouldCreate := false
	shouldUpdate := false

	// Get dashboard ID based on version
	var dashboardID string
	var lastForceSyncTime *metav1.Time

	switch d := dashboard.(type) {
	case *v1alpha1.DatadogDashboard:
		dashboardID = d.Status.ID
		lastForceSyncTime = d.Status.LastForceSyncTime
	case *v1alpha2.DatadogDashboard:
		dashboardID = d.Status.ID
		lastForceSyncTime = d.Status.LastForceSyncTime
	}

	if dashboardID == "" {
		shouldCreate = true
	} else {
		if instanceSpecHash != statusSpecHash {
			logger.Info("DatadogDashboard manifest has changed")
			shouldUpdate = true
		} else if lastForceSyncTime == nil || ((forceSyncPeriod - now.Sub(lastForceSyncTime.Time)) <= 0) {
			// Periodically force a sync with the API to ensure parity
			_, err = r.getDashboard(dashboardID)
			if err != nil {
				logger.Error(err, "error getting Dashboard", "Dashboard ID", dashboardID)
				r.updateErrStatusVersionAware(status, now, "DatadoggDashboardSyncStatusGetError", "GettingDashboard", err)
				if strings.Contains(err.Error(), ctrutils.NotFoundString) {
					shouldCreate = true
				}
			} else {
				shouldUpdate = true
			}
			r.setLastForceSyncTimeVersionAware(status, now)
		}
	}

	if shouldCreate || shouldUpdate {
		if shouldCreate {
			err = r.createVersionAware(logger, dashboard, status, now, instanceSpecHash, processor)
		} else if shouldUpdate {
			err = r.updateVersionAware(logger, dashboard, status, now, instanceSpecHash, processor)
		}

		if err != nil {
			result.RequeueAfter = defaultErrRequeuePeriod
		}
	}

	// If reconcile was successful and uneventful, requeue with period defaultRequeuePeriod
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}

	return r.updateStatusIfNeededVersionAware(logger, dashboard, status, result)
}

func (r *Reconciler) update(logger logr.Logger, instance *v1alpha1.DatadogDashboard, status *v1alpha1.DatadogDashboardStatus, now metav1.Time, hash string) error {
	// Update hash to reflect the spec we're attempting to sync (whether it succeeds or fails)
	status.CurrentHash = hash

	if _, err := updateDashboard(r.datadogAuth, logger, r.datadogClient, instance); err != nil {
		logger.Error(err, "error updating Dashboard", "Dashboard ID", instance.Status.ID)
		updateErrStatus(status, now, v1alpha1.DatadogDashboardSyncStatusUpdateError, "UpdatingDasboard", err)
		return err
	}

	event := buildEventInfo(instance.Name, instance.Namespace, datadog.UpdateEvent)
	r.recordEvent(instance, event)

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeUpdated, metav1.ConditionTrue, "UpdatingDashboard", "DatadogDashboard Update")
	status.SyncStatus = v1alpha1.DatadogDashboardSyncStatusOK
	status.LastForceSyncTime = &now

	logger.Info("Updated DatadogDashboard", "Dashboard ID", instance.Status.ID)
	return nil
}

func (r *Reconciler) create(logger logr.Logger, instance *v1alpha1.DatadogDashboard, status *v1alpha1.DatadogDashboardStatus, now metav1.Time, hash string) error {
	logger.V(1).Info("Dashboard ID is not set; creating Dashboard in Datadog")

	// Create Dashboard in Datadog
	createdDashboard, err := createDashboard(r.datadogAuth, logger, r.datadogClient, instance)
	if err != nil {
		logger.Error(err, "error creating Dashboard")
		updateErrStatus(status, now, v1alpha1.DatadogDashboardSyncStatusCreateError, "CreatingDashboard", err)
		return err
	}
	event := buildEventInfo(instance.Name, instance.Namespace, datadog.CreationEvent)
	r.recordEvent(instance, event)

	// Add static information to status
	status.ID = createdDashboard.GetId()
	createdTime := metav1.NewTime(createdDashboard.GetCreatedAt())
	status.Creator = createdDashboard.GetAuthorHandle()
	status.Created = &createdTime
	status.SyncStatus = v1alpha1.DatadogDashboardSyncStatusOK
	status.LastForceSyncTime = &createdTime
	status.CurrentHash = hash

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeCreated, metav1.ConditionTrue, "CreatingDashboard", "DatadogDashboard Created")
	logger.Info("created a new DatadogDashboard", "dashboard ID", status.ID)

	return nil
}

// NOTE: commented out for now since 'generated:kubernetes' is not allowed as a tag in the dashboards API
// func (r *Reconciler) checkRequiredTags(logger logr.Logger, instance *v1alpha1.DatadogDashboard) (bool, error) {
// 	tags := instance.Spec.Tags
// 	// TagsToAdd is an empty string for now because "generated" tag keys are not allowed in the Dashboards API
// 	tagsToAdd := []string{}
// 	if len(tagsToAdd) > 0 {
// 		tags = append(tags, tagsToAdd...)
// 		instance.Spec.Tags = tags
// 		err := r.client.Update(context.TODO(), instance)
// 		if err != nil {
// 			logger.Error(err, "failed to update DatadogDashboard with required tags")
// 			return false, err
// 		}
// 		logger.Info("Added required tags", "Dashboard ID", instance.Status.ID)
// 		return true, nil
// 	}

// 	return false, nil
// }

func updateErrStatus(status *v1alpha1.DatadogDashboardStatus, now metav1.Time, syncStatus v1alpha1.DatadogDashboardSyncStatus, reason string, err error) {
	condition.UpdateFailureStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeError, reason, err)
	status.SyncStatus = syncStatus
}

func (r *Reconciler) updateStatusIfNeeded(logger logr.Logger, instance *v1alpha1.DatadogDashboard, status *v1alpha1.DatadogDashboardStatus, result ctrl.Result) (ctrl.Result, error) {
	if !apiequality.Semantic.DeepEqual(&instance.Status, status) {
		instance.Status = *status
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			if apierrors.IsConflict(err) {
				logger.Error(err, "unable to update DatadogDashboard status due to update conflict")
				return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, nil
			}
			logger.Error(err, "unable to update DatadogDashboard status")
			return ctrl.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod}, err
		}
	}
	return result, nil
}

// buildEventInfo creates a new EventInfo instance.
func buildEventInfo(name, ns string, eventType datadog.EventType) utils.EventInfo {
	return utils.BuildEventInfo(name, ns, datadogDashboardKind, eventType)
}

// recordEvent wraps the manager event recorder
func (r *Reconciler) recordEvent(dashboard runtime.Object, info utils.EventInfo) {
	r.recorder.Event(dashboard, corev1.EventTypeNormal, info.GetReason(), info.GetMessage())
}

// Version-aware helper methods

func (r *Reconciler) getDashboard(dashboardID string) (datadogV1.Dashboard, error) {
	return getDashboard(r.datadogAuth, r.datadogClient, dashboardID)
}

func (r *Reconciler) handleFinalizerVersionAware(logger logr.Logger, dashboard interface{}) (ctrl.Result, error) {
	switch d := dashboard.(type) {
	case *v1alpha1.DatadogDashboard:
		return r.handleFinalizer(logger, d)
	case *v1alpha2.DatadogDashboard:
		return r.handleFinalizerV1Alpha2(logger, d)
	default:
		return ctrl.Result{}, fmt.Errorf("unsupported dashboard type: %T", dashboard)
	}
}

func (r *Reconciler) handleFinalizerV1Alpha2(logger logr.Logger, instance *v1alpha2.DatadogDashboard) (ctrl.Result, error) {
	// Similar to v1alpha1 finalizer handling but for v1alpha2
	// For now, we'll implement a basic version
	return ctrl.Result{}, nil
}

func (r *Reconciler) updateErrStatusVersionAware(status interface{}, now metav1.Time, syncStatus string, reason string, err error) {
	switch s := status.(type) {
	case *v1alpha1.DatadogDashboardStatus:
		updateErrStatus(s, now, v1alpha1.DatadogDashboardSyncStatus(syncStatus), reason, err)
	case *v1alpha2.DatadogDashboardStatus:
		updateErrStatusV1Alpha2(s, now, v1alpha2.DatadogDashboardSyncStatus(syncStatus), reason, err)
	}
}

func updateErrStatusV1Alpha2(status *v1alpha2.DatadogDashboardStatus, now metav1.Time, syncStatus v1alpha2.DatadogDashboardSyncStatus, reason string, err error) {
	condition.UpdateFailureStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeError, reason, err)
	status.SyncStatus = syncStatus
}

func (r *Reconciler) setLastForceSyncTimeVersionAware(status interface{}, now metav1.Time) {
	switch s := status.(type) {
	case *v1alpha1.DatadogDashboardStatus:
		s.LastForceSyncTime = &now
	case *v1alpha2.DatadogDashboardStatus:
		s.LastForceSyncTime = &now
	}
}

func (r *Reconciler) updateStatusIfNeededVersionAware(logger logr.Logger, dashboard interface{}, status interface{}, result ctrl.Result) (ctrl.Result, error) {
	switch d := dashboard.(type) {
	case *v1alpha1.DatadogDashboard:
		s := status.(*v1alpha1.DatadogDashboardStatus)
		return r.updateStatusIfNeeded(logger, d, s, result)
	case *v1alpha2.DatadogDashboard:
		s := status.(*v1alpha2.DatadogDashboardStatus)
		return r.updateStatusIfNeededV1Alpha2(logger, d, s, result)
	default:
		return ctrl.Result{}, fmt.Errorf("unsupported dashboard type: %T", dashboard)
	}
}

func (r *Reconciler) updateStatusIfNeededV1Alpha2(logger logr.Logger, instance *v1alpha2.DatadogDashboard, status *v1alpha2.DatadogDashboardStatus, result ctrl.Result) (ctrl.Result, error) {
	if !apiequality.Semantic.DeepEqual(&instance.Status, status) {
		instance.Status = *status
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			if apierrors.IsConflict(err) {
				logger.Error(err, "unable to update DatadogDashboard status due to update conflict")
				return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, nil
			}
			logger.Error(err, "unable to update DatadogDashboard status")
			return ctrl.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod}, err
		}
	}
	return result, nil
}

func (r *Reconciler) createVersionAware(logger logr.Logger, dashboard interface{}, status interface{}, now metav1.Time, hash string, processor WidgetProcessor) error {
	switch d := dashboard.(type) {
	case *v1alpha1.DatadogDashboard:
		s := status.(*v1alpha1.DatadogDashboardStatus)
		return r.create(logger, d, s, now, hash)
	case *v1alpha2.DatadogDashboard:
		s := status.(*v1alpha2.DatadogDashboardStatus)
		return r.createV1Alpha2(logger, d, s, now, hash, processor)
	default:
		return fmt.Errorf("unsupported dashboard type: %T", dashboard)
	}
}

func (r *Reconciler) updateVersionAware(logger logr.Logger, dashboard interface{}, status interface{}, now metav1.Time, hash string, processor WidgetProcessor) error {
	switch d := dashboard.(type) {
	case *v1alpha1.DatadogDashboard:
		s := status.(*v1alpha1.DatadogDashboardStatus)
		return r.update(logger, d, s, now, hash)
	case *v1alpha2.DatadogDashboard:
		s := status.(*v1alpha2.DatadogDashboardStatus)
		return r.updateV1Alpha2(logger, d, s, now, hash, processor)
	default:
		return fmt.Errorf("unsupported dashboard type: %T", dashboard)
	}
}

func (r *Reconciler) createV1Alpha2(logger logr.Logger, instance *v1alpha2.DatadogDashboard, status *v1alpha2.DatadogDashboardStatus, now metav1.Time, hash string, processor WidgetProcessor) error {
	logger.V(1).Info("Dashboard ID is not set; creating Dashboard in Datadog")

	// Create Dashboard in Datadog
	createdDashboard, err := createDashboardV1Alpha2(r.datadogAuth, logger, r.datadogClient, instance, processor)
	if err != nil {
		logger.Error(err, "error creating Dashboard")
		updateErrStatusV1Alpha2(status, now, v1alpha2.DatadogDashboardSyncStatusCreateError, "CreatingDashboard", err)
		return err
	}
	event := buildEventInfo(instance.Name, instance.Namespace, datadog.CreationEvent)
	r.recordEvent(instance, event)

	// Add static information to status
	status.ID = createdDashboard.GetId()
	createdTime := metav1.NewTime(createdDashboard.GetCreatedAt())
	status.Creator = createdDashboard.GetAuthorHandle()
	status.Created = &createdTime
	status.SyncStatus = v1alpha2.DatadogDashboardSyncStatusOK
	status.LastForceSyncTime = &createdTime
	status.CurrentHash = hash

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeCreated, metav1.ConditionTrue, "CreatingDashboard", "DatadogDashboard Created")
	logger.Info("created a new DatadogDashboard", "dashboard ID", status.ID)

	return nil
}

func (r *Reconciler) updateV1Alpha2(logger logr.Logger, instance *v1alpha2.DatadogDashboard, status *v1alpha2.DatadogDashboardStatus, now metav1.Time, hash string, processor WidgetProcessor) error {
	// Update hash to reflect the spec we're attempting to sync (whether it succeeds or fails)
	status.CurrentHash = hash

	if _, err := updateDashboardV1Alpha2(r.datadogAuth, logger, r.datadogClient, instance, processor); err != nil {
		logger.Error(err, "error updating Dashboard", "Dashboard ID", instance.Status.ID)
		updateErrStatusV1Alpha2(status, now, v1alpha2.DatadogDashboardSyncStatusUpdateError, "UpdatingDasboard", err)
		return err
	}

	event := buildEventInfo(instance.Name, instance.Namespace, datadog.UpdateEvent)
	r.recordEvent(instance, event)

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeUpdated, metav1.ConditionTrue, "UpdatingDashboard", "DatadogDashboard Update")
	status.SyncStatus = v1alpha2.DatadogDashboardSyncStatusOK
	status.LastForceSyncTime = &now

	logger.Info("Updated DatadogDashboard", "Dashboard ID", instance.Status.ID)
	return nil
}
