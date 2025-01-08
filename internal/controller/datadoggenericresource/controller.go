package datadoggenericcr

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
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

	"github.com/DataDog/datadog-operator/internal/controller/utils"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

const (
	defaultRequeuePeriod    = 60 * time.Second
	defaultErrRequeuePeriod = 5 * time.Second
	defaultForceSyncPeriod  = 60 * time.Minute
	datadogGenericCRKind    = "DatadogGenericCustomResource"
)

type Reconciler struct {
	client                  client.Client
	datadogSyntheticsClient *datadogV1.SyntheticsApi
	datadogNotebooksClient  *datadogV1.NotebooksApi
	// TODO: add other clients
	datadogAuth context.Context
	scheme      *runtime.Scheme
	log         logr.Logger
	recorder    record.EventRecorder
}

func NewReconciler(client client.Client, ddClient datadogclient.DatadogGenericClient, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder) *Reconciler {
	return &Reconciler{
		client:                  client,
		datadogSyntheticsClient: ddClient.SyntheticsClient,
		datadogNotebooksClient:  ddClient.NotebooksClient,
		// TODO: add other clients
		// datadogOtherClient: ddClient.OtherClient,
		datadogAuth: ddClient.Auth,
		scheme:      scheme,
		log:         log,
		recorder:    recorder,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	return r.internalReconcile(ctx, request)
}

func (r *Reconciler) internalReconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := r.log.WithValues("datadoggenericcr", req.NamespacedName)
	logger.Info("Reconciling Datadog Generic Custom Resource")
	now := metav1.NewTime(time.Now())

	instance := &v1alpha1.DatadogGenericResource{}
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

	if err = v1alpha1.IsValidDatadogGenericResource(&instance.Spec); err != nil {
		logger.Error(err, "invalid DatadogGenericCR")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusValidateError, "ValidatingGenericCR", err)
		return r.updateStatusIfNeeded(logger, instance, status, result)
	}

	instanceSpecHash, err := comparison.GenerateMD5ForSpec(&instance.Spec)

	if err != nil {
		logger.Error(err, "error generating hash")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusUpdateError, "GeneratingGenericCRSpecHash", err)
		return r.updateStatusIfNeeded(logger, instance, status, result)
	}

	shouldCreate := false
	shouldUpdate := false

	if instance.Status.Id == "" {
		shouldCreate = true
	} else {
		if instanceSpecHash != statusSpecHash {
			logger.Info("DatadogGenericCR manifest has changed")
			shouldUpdate = true
		} else if instance.Status.LastForceSyncTime == nil || ((defaultForceSyncPeriod - now.Sub(instance.Status.LastForceSyncTime.Time)) <= 0) {
			// Periodically force a sync with the API to ensure parity
			// Get GenericCR to make sure it exists before trying any updates. If it doesn't, set shouldCreate
			err = r.get(instance)
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

func (r *Reconciler) get(instance *v1alpha1.DatadogGenericResource) error {
	return apiGet(r, instance)
}

func (r *Reconciler) update(logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	err := apiUpdate(r, instance)
	if err != nil {
		logger.Error(err, "error updating generic CR", "generic CR Id", instance.Status.Id)
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusUpdateError, "UpdatingGenericCR", err)
		return err
	}

	event := buildEventInfo(instance.Name, instance.Namespace, datadog.UpdateEvent)
	r.recordEvent(instance, event)

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeUpdated, metav1.ConditionTrue, "UpdatingGenericCR", "DatadogGenericCR Update")
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash
	status.LastForceSyncTime = &now

	logger.Info("Updated Datadog Generic CR", "Generic CR Id", instance.Status.Id)
	return nil
}

func (r *Reconciler) create(logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	logger.V(1).Info("Custom resource Id is not set; creating custom resource in Datadog")

	err := apiCreateAndUpdateStatus(r, logger, instance, status, now, hash)
	if err != nil {
		return err
	}
	event := buildEventInfo(instance.Name, instance.Namespace, datadog.CreationEvent)
	r.recordEvent(instance, event)

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeCreated, metav1.ConditionTrue, "CreatingGenericCR", "DatadogGenericCR Created")
	logger.Info("created a new DatadogGenericCR", "generic CR Id", status.Id)

	return nil
}

func updateErrStatus(status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, syncStatus v1alpha1.DatadogSyncStatus, reason string, err error) {
	condition.UpdateFailureStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeError, reason, err)
	status.SyncStatus = syncStatus
}

func (r *Reconciler) updateStatusIfNeeded(logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, result ctrl.Result) (ctrl.Result, error) {
	if !apiequality.Semantic.DeepEqual(&instance.Status, status) {
		instance.Status = *status
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			if apierrors.IsConflict(err) {
				logger.Error(err, "unable to update DatadogGenericCR status due to update conflict")
				return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, nil
			}
			logger.Error(err, "unable to update DatadogGenericCR status")
			return ctrl.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod}, err
		}
	}
	return result, nil
}

// buildEventInfo creates a new EventInfo instance.
func buildEventInfo(name, ns string, eventType datadog.EventType) utils.EventInfo {
	return utils.BuildEventInfo(name, ns, datadogGenericCRKind, eventType)
}

// recordEvent wraps the manager event recorder
func (r *Reconciler) recordEvent(genericcr runtime.Object, info utils.EventInfo) {
	r.recorder.Event(genericcr, corev1.EventTypeNormal, info.GetReason(), info.GetMessage())
}
