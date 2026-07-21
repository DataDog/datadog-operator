package datadoggenericresource

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlhandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
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
	defaultRequeuePeriod                   = 60 * time.Second
	defaultErrRequeuePeriod                = 5 * time.Second
	defaultForceSyncPeriod                 = 60 * time.Minute
	datadogGenericResourceKind             = "DatadogGenericResource"
	ddGenericResourceForceSyncPeriodEnvVar = "DD_GENERIC_RESOURCE_FORCE_SYNC_PERIOD"
)

type Reconciler struct {
	client          client.Client
	credsManager    *config.CredentialManager
	handlers        map[v1alpha1.SupportedResourcesType]ResourceHandler
	requeuePeriod   time.Duration
	forceSyncPeriod time.Duration
	scheme          *runtime.Scheme
	log             logr.Logger
	recorder        record.EventRecorder
}

type ReconcilerOptions struct {
	RequeuePeriod time.Duration
}

func NewReconciler(client client.Client, credsManager *config.CredentialManager, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder, opts ...ReconcilerOptions) *Reconciler {
	options := ReconcilerOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}

	return &Reconciler{
		client:          client,
		credsManager:    credsManager,
		handlers:        buildHandlers(datadogclient.InitGenericClients()),
		requeuePeriod:   requeuePeriod(log, options.RequeuePeriod),
		forceSyncPeriod: forceSyncPeriodFromEnv(log),
		scheme:          scheme,
		log:             log,
		recorder:        recorder,
	}
}

func requeuePeriod(logger logr.Logger, configured time.Duration) time.Duration {
	if configured <= 0 {
		configured = defaultRequeuePeriod
	}
	logger.Info("Setting generic resource requeue period", "duration", configured.String())
	return configured
}

func forceSyncPeriodFromEnv(logger logr.Logger) time.Duration {
	forceSyncPeriod := defaultForceSyncPeriod

	if userForceSyncPeriod, ok := os.LookupEnv(ddGenericResourceForceSyncPeriodEnvVar); ok {
		forceSyncPeriodInt, err := strconv.Atoi(userForceSyncPeriod)
		if err != nil {
			logger.Error(err, "Invalid value for generic resource force sync period. Defaulting to 60 minutes.")
		} else if forceSyncPeriodInt < 1 {
			logger.Error(fmt.Errorf("force sync period must be at least 1 minute"), "Invalid value for generic resource force sync period. Defaulting to 60 minutes.")
		} else {
			forceSyncPeriod = time.Duration(forceSyncPeriodInt) * time.Minute
		}
	}

	logger.Info("Setting generic resource force sync period", "minutes", int(forceSyncPeriod.Minutes()))
	return forceSyncPeriod
}

func (r *Reconciler) getHandler(resourceType v1alpha1.SupportedResourcesType) ResourceHandler {
	h, ok := r.handlers[resourceType]
	if !ok {
		panic(unsupportedInstanceType(resourceType))
	}
	return h
}

func (r *Reconciler) Reconcile(ctx context.Context, instance *v1alpha1.DatadogGenericResource) (reconcile.Result, error) {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Reconciling DatadogGenericResource")

	auth, credErr := r.credsManager.GetAuth()
	if credErr != nil {
		return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, fmt.Errorf("unable to get credentials: %w", credErr)
	}

	now := metav1.NewTime(time.Now())

	var result ctrl.Result
	var err error

	handler := r.getHandler(instance.Spec.Type)

	final := finalizer.NewFinalizer(logger, r.client, r.deleteResource(logger, auth, handler), r.requeuePeriod, defaultErrRequeuePeriod)
	if result, err = final.HandleFinalizer(ctx, instance, instance.Status.Id, datadogGenericResourceFinalizer); ctrutils.ShouldReturn(result, err) {
		return result, err
	}

	status := instance.Status.DeepCopy()
	statusSpecHash := instance.Status.CurrentHash
	instanceSpecHash, err := comparison.GenerateMD5ForSpec(&instance.Spec)

	if err != nil {
		logger.Error(err, "error generating hash")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusUpdateError, "GeneratingGenericResourceSpecHash", err)
		return r.updateStatusIfNeeded(ctx, instance, status, result)
	}

	shouldCreate := false
	shouldUpdate := false
	shouldRefreshStatus := false

	if instance.Status.Id == "" {
		shouldCreate = true
	} else {
		if instanceSpecHash != statusSpecHash {
			logger.Info("DatadogGenericResource manifest has changed")
			shouldUpdate = true
		} else if instance.Status.LastForceSyncTime == nil || ((r.forceSyncPeriod - now.Sub(instance.Status.LastForceSyncTime.Time)) <= 0) {
			// Periodically force a sync with the API to ensure parity
			// Make sure it exists before trying any updates. If it doesn't, set shouldCreate
			err = handler.getResource(auth, instance)
			if err != nil {
				logger.Error(err, "error getting custom resource", "custom resource Id", instance.Status.Id, "resource type", instance.Spec.Type)
				updateErrStatus(status, now, v1alpha1.DatadogSyncStatusGetError, "GettingCustomResource", err)
				if strings.Contains(err.Error(), ctrutils.NotFoundString) {
					shouldCreate = true
				}
			} else {
				shouldUpdate = true
			}
			status.LastForceSyncTime = &now
		} else if instance.Status.StateLastUpdateTime == nil || ((r.requeuePeriod - now.Sub(instance.Status.StateLastUpdateTime.Time)) <= 0) {
			// Idle tick: refresh Datadog-side state for resource types that expose it.
			// No-op for resource types without live state.
			shouldRefreshStatus = true
		}
	}

	if shouldCreate || shouldUpdate {

		if shouldCreate {
			err = r.create(ctx, auth, handler, instance, status, now, instanceSpecHash)
		} else if shouldUpdate {
			err = r.update(ctx, auth, handler, instance, status, now, instanceSpecHash)
		}

		if err != nil {
			result.RequeueAfter = defaultErrRequeuePeriod
		}
	} else if shouldRefreshStatus {
		result.Priority = ptr.To(ctrlhandler.LowPriority)
		state, refreshErr := handler.refreshState(auth, instance)
		if refreshErr != nil {
			logger.V(1).Info("state refresh failed", "err", refreshErr, "custom resource Id", instance.Status.Id, "resource type", instance.Spec.Type)
			// Preserve last-known state and surface the failure via the StateSynced condition.
			// Do not fail reconcile.
			condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeStateSynced, metav1.ConditionFalse, "GetError", refreshErr.Error())
		} else if state != nil {
			// Resource type exposes live state — merge it into status.
			applyResourceState(*state, status, now)
			condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeStateSynced, metav1.ConditionTrue, "Synced", "State refreshed from Datadog")
		}
		// state == nil && refreshErr == nil: resource type does not expose live state, no-op.
	}

	// If reconcile was successful and uneventful, requeue with the configured period.
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = r.requeuePeriod
	}

	return r.updateStatusIfNeeded(ctx, instance, status, result)
}

func (r *Reconciler) update(ctx context.Context, auth context.Context, handler ResourceHandler, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	logger := ctrl.LoggerFrom(ctx)
	// Update hash to reflect the spec we're attempting to sync (whether it succeeds or fails)
	status.CurrentHash = hash

	err := handler.updateResource(auth, instance)
	if err != nil {
		if strings.Contains(err.Error(), ctrutils.NotFoundString) {
			// If the remote resource was deleted out-of-band after we stored its ID,
			// treat an update-time 404 as drift from the Kubernetes source of truth
			// and recreate it immediately instead of waiting for the next force sync.
			logger.Info("generic resource missing in Datadog during update; recreating", "generic resource Id", instance.Status.Id)
			return r.create(ctx, auth, handler, instance, status, now, hash)
		}
		logger.Error(err, "error updating generic resource", "generic resource Id", instance.Status.Id)
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusUpdateError, "UpdatingGenericResource", err)
		return err
	}

	event := buildEventInfo(instance.Name, instance.Namespace, datadog.UpdateEvent)
	r.recordEvent(instance, event)

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeUpdated, metav1.ConditionTrue, "UpdatingGenericResource", "DatadogGenericResource Update")
	// Clear error condition on successful update
	condition.RemoveStatusCondition(&status.Conditions, condition.DatadogConditionTypeError)
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.LastForceSyncTime = &now

	logger.Info("Updated DatadogGenericResource", "Generic Resource Id", instance.Status.Id)
	return nil
}

func (r *Reconciler) create(ctx context.Context, auth context.Context, handler ResourceHandler, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Generic resource Id is not set; creating resource in Datadog")

	result, err := handler.createResource(auth, instance)
	if err != nil {
		logger.Error(err, "error creating resource", "type", instance.Spec.Type)
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	createdTime := result.CreatedTime
	if createdTime == nil {
		createdTime = &now
	}
	status.Id = result.ID
	status.Created = createdTime
	status.LastForceSyncTime = createdTime
	status.Creator = result.Creator
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash

	event := buildEventInfo(instance.Name, instance.Namespace, datadog.CreationEvent)
	r.recordEvent(instance, event)

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeCreated, metav1.ConditionTrue, "CreatingGenericResource", "DatadogGenericResource Created")
	// Clear error condition on successful creation
	condition.RemoveStatusCondition(&status.Conditions, condition.DatadogConditionTypeError)
	logger.Info("created a new DatadogGenericResource", "generic resource Id", status.Id)

	return nil
}

func updateErrStatus(status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, syncStatus v1alpha1.DatadogSyncStatus, reason string, err error) {
	condition.UpdateFailureStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeError, reason, err)
	status.SyncStatus = syncStatus
}

// applyResourceState merges a refreshed Datadog-side state into the CR status,
// bumping StateLastTransitionTime only when the state value actually changes,
// and always advancing StateLastUpdateTime to now.
func applyResourceState(state string, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time) {
	oldState := status.State
	status.State = state
	if status.State != oldState {
		status.StateLastTransitionTime = &now
	}
	status.StateLastUpdateTime = &now
}

func (r *Reconciler) updateStatusIfNeeded(ctx context.Context, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, result ctrl.Result) (ctrl.Result, error) {
	if !apiequality.Semantic.DeepEqual(&instance.Status, status) {
		desiredStatus := status.DeepCopy()
		instance.Status = *desiredStatus.DeepCopy()
		err := r.client.Status().Update(ctx, instance)
		if apierrors.IsConflict(err) {
			// The Datadog API operation may already have succeeded, so preserve
			// the resulting status (most importantly the remote resource ID) and
			// retry only the Kubernetes status write against the latest object.
			key := client.ObjectKeyFromObject(instance)
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				latest := &v1alpha1.DatadogGenericResource{}
				if getErr := r.client.Get(ctx, key, latest); getErr != nil {
					return getErr
				}

				if apiequality.Semantic.DeepEqual(&latest.Status, desiredStatus) {
					instance.ResourceVersion = latest.ResourceVersion
					instance.Status = latest.Status
					return nil
				}

				latest.Status = *desiredStatus.DeepCopy()
				if updateErr := r.client.Status().Update(ctx, latest); updateErr != nil {
					return updateErr
				}

				instance.ResourceVersion = latest.ResourceVersion
				instance.Status = latest.Status
				return nil
			})
		}
		if err != nil {
			logger := ctrl.LoggerFrom(ctx)
			if apierrors.IsConflict(err) {
				logger.Error(err, "unable to update DatadogGenericResource status due to update conflict")
				return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod, Priority: result.Priority}, nil
			}
			logger.Error(err, "unable to update DatadogGenericResource status")
			return ctrl.Result{Requeue: true, RequeueAfter: r.requeuePeriod, Priority: result.Priority}, err
		}
	}
	return result, nil
}

// buildEventInfo creates a new EventInfo instance.
func buildEventInfo(name, ns string, eventType datadog.EventType) utils.EventInfo {
	return utils.BuildEventInfo(name, ns, datadogGenericResourceKind, eventType)
}

// recordEvent wraps the manager event recorder
func (r *Reconciler) recordEvent(genericresource runtime.Object, info utils.EventInfo) {
	r.recorder.Event(genericresource, corev1.EventTypeNormal, info.GetReason(), info.GetMessage())
}
