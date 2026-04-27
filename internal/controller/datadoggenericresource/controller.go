package datadoggenericresource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	defaultRequeuePeriod       = 60 * time.Second
	defaultErrRequeuePeriod    = 5 * time.Second
	defaultForceSyncPeriod     = 60 * time.Minute
	datadogGenericResourceKind = "DatadogGenericResource"
)

// handlerBuilderFunc builds resource handlers for a given auth context.
type handlerBuilderFunc func(auth context.Context) map[v1alpha1.SupportedResourcesType]ResourceHandler

type Reconciler struct {
	client         client.Client
	credsManager   *config.CredentialManager
	handlerBuilder handlerBuilderFunc
	scheme         *runtime.Scheme
	log            logr.Logger
	recorder       record.EventRecorder
}

func NewReconciler(client client.Client, credsManager *config.CredentialManager, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder) (*Reconciler, error) {
	clients := datadogclient.InitGenericClients()
	return &Reconciler{
		client:       client,
		credsManager: credsManager,
		handlerBuilder: func(auth context.Context) map[v1alpha1.SupportedResourcesType]ResourceHandler {
			return buildHandlers(clients, auth)
		},
		scheme:   scheme,
		log:      log,
		recorder: recorder,
	}, nil
}

func (r *Reconciler) getHandler(auth context.Context, resourceType v1alpha1.SupportedResourcesType) ResourceHandler {
	handlers := r.handlerBuilder(auth)
	h, ok := handlers[resourceType]
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

	handler := r.getHandler(auth, instance.Spec.Type)

	final := finalizer.NewFinalizer(logger, r.client, r.deleteResource(logger, handler), defaultRequeuePeriod, defaultErrRequeuePeriod)
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

	if instance.Status.Id == "" {
		shouldCreate = true
	} else {
		if instanceSpecHash != statusSpecHash {
			logger.Info("DatadogGenericResource manifest has changed")
			shouldUpdate = true
		} else if instance.Status.LastForceSyncTime == nil || ((defaultForceSyncPeriod - now.Sub(instance.Status.LastForceSyncTime.Time)) <= 0) {
			// Periodically force a sync with the API to ensure parity
			// Make sure it exists before trying any updates. If it doesn't, set shouldCreate
			err = handler.getResource(instance)
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
		}
	}

	if shouldCreate || shouldUpdate {

		if shouldCreate {
			err = r.create(ctx, handler, instance, status, now, instanceSpecHash)
		} else if shouldUpdate {
			err = r.update(ctx, handler, instance, status, now, instanceSpecHash)
		}

		if err != nil {
			result.RequeueAfter = defaultErrRequeuePeriod
		}
	}

	// If reconcile was successful and uneventful, requeue with period defaultRequeuePeriod
	if result.IsZero() {
		result.RequeueAfter = defaultRequeuePeriod
	}

	return r.updateStatusIfNeeded(ctx, instance, status, result)
}

func (r *Reconciler) update(ctx context.Context, handler ResourceHandler, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	logger := ctrl.LoggerFrom(ctx)
	// Update hash to reflect the spec we're attempting to sync (whether it succeeds or fails)
	status.CurrentHash = hash

	err := handler.updateResource(instance)
	if err != nil {
		if strings.Contains(err.Error(), ctrutils.NotFoundString) {
			// If the remote resource was deleted out-of-band after we stored its ID,
			// treat an update-time 404 as drift from the Kubernetes source of truth
			// and recreate it immediately instead of waiting for the next force sync.
			logger.Info("generic resource missing in Datadog during update; recreating", "generic resource Id", instance.Status.Id)
			return r.create(ctx, handler, instance, status, now, hash)
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

func (r *Reconciler) create(ctx context.Context, handler ResourceHandler, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Generic resource Id is not set; creating resource in Datadog")

	result, err := handler.createResource(instance)
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

func (r *Reconciler) updateStatusIfNeeded(ctx context.Context, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, result ctrl.Result) (ctrl.Result, error) {
	if !apiequality.Semantic.DeepEqual(&instance.Status, status) {
		instance.Status = *status
		if err := r.client.Status().Update(ctx, instance); err != nil {
			logger := ctrl.LoggerFrom(ctx)
			if apierrors.IsConflict(err) {
				logger.Error(err, "unable to update DatadogGenericResource status due to update conflict")
				return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, nil
			}
			logger.Error(err, "unable to update DatadogGenericResource status")
			return ctrl.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod}, err
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
