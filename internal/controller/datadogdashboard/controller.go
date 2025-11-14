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
	credsManager  *config.CredentialManager
	scheme        *runtime.Scheme
	log           logr.Logger
	recorder      record.EventRecorder
}

func NewReconciler(client client.Client, credsManager *config.CredentialManager, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder) (*Reconciler, error) {
	return &Reconciler{
		client:        client,
		datadogClient: datadogclient.InitDashboardClient(),
		credsManager:  credsManager,
		scheme:        scheme,
		log:           log,
		recorder:      recorder,
	}, nil
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

	instance := &v1alpha1.DatadogDashboard{}
	var result ctrl.Result
	var err error

	if err = r.client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if result, err = r.handleFinalizer(logger, instance); ctrutils.ShouldReturn(result, err) {
		return result, err
	}

	status := instance.Status.DeepCopy()
	statusSpecHash := instance.Status.CurrentHash

	if err = v1alpha1.IsValidDatadogDashboard(&instance.Spec); err != nil {
		logger.Error(err, "invalid Dashboard")

		updateErrStatus(status, now, v1alpha1.DatadogDashboardSyncStatusValidateError, "ValidatingDashboard", err)
		return r.updateStatusIfNeeded(logger, instance, status, result)
	}

	instanceSpecHash, err := comparison.GenerateMD5ForSpec(&instance.Spec)

	if err != nil {
		logger.Error(err, "error generating hash")
		updateErrStatus(status, now, v1alpha1.DatadogDashboardSyncStatusUpdateError, "GeneratingDashboardSpecHash", err)
		return r.updateStatusIfNeeded(logger, instance, status, result)
	}

	shouldCreate := false
	shouldUpdate := false

	// Get fresh credentials and create auth context for this reconcile
	creds, err := r.credsManager.GetCredentials()
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get credentials: %w", err)
	}
	r.datadogAuth, err = datadogclient.GetAuth(r.log, creds)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to setup auth: %w", err)
	}

	if instance.Status.ID == "" {
		shouldCreate = true
	} else {
		if instanceSpecHash != statusSpecHash {
			logger.Info("DatadogDashboard manifest has changed")
			shouldUpdate = true
		} else if instance.Status.LastForceSyncTime == nil || ((forceSyncPeriod - now.Sub(instance.Status.LastForceSyncTime.Time)) <= 0) {
			// Periodically force a sync with the API to ensure parity
			// Get Dashboard to make sure it exists before trying any updates. If it doesn't, set shouldCreate
			_, err = r.get(instance)
			if err != nil {
				logger.Error(err, "error getting Dashboard", "Dashboard ID", instance.Status.ID)
				updateErrStatus(status, now, v1alpha1.DatadoggDashboardSyncStatusGetError, "GettingDashboard", err)
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
		// tagsUpdated, err := r.checkRequiredTags(logger, instance)
		// if err != nil {
		// 	result.RequeueAfter = defaultErrRequeuePeriod
		// 	return r.updateStatusIfNeeded(logger, instance, status, result)
		// } else if tagsUpdated {
		// 	// A reconcile is triggered by the update
		// 	return r.updateStatusIfNeeded(logger, instance, status, result)
		// }

		if shouldCreate {
			err = r.create(logger, instance, status, now, instanceSpecHash)
		} else if shouldUpdate {
			err = r.update(logger, instance, status, now, instanceSpecHash)
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

func (r *Reconciler) get(instance *v1alpha1.DatadogDashboard) (datadogV1.Dashboard, error) {
	return getDashboard(r.datadogAuth, r.datadogClient, instance.Status.ID)
}

func (r *Reconciler) update(logger logr.Logger, instance *v1alpha1.DatadogDashboard, status *v1alpha1.DatadogDashboardStatus, now metav1.Time, hash string) error {
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
	status.CurrentHash = hash
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
