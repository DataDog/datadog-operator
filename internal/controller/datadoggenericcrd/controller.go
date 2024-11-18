package datadoggenericcrd

import (
	"context"
	"fmt"
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

// type Operation string

// const (
// 	Create Operation = "create"
// 	Get    Operation = "get"
// 	Update Operation = "update"
// 	Delete Operation = "delete"
// )

// type HandlerFunc func(instance *v1alpha1.DatadogGenericCRD) error

// var handlers = map[Operation]HandlerFunc{
// 	"synthetics_browser_test": {
// 		Create: createSyntheticsBrowserTest,
// 		Get:    getSyntheticsBrowserTest,
// 		Update: updateSyntheticsBrowserTest,
// 		Delete: deleteSyntheticsBrowserTest,
// 	},
// 	"synthetics_api_test": {
// 		Create: createSyntheticsAPITest,
// 		Get:    getSyntheticsAPITest,
// 		Update: updateSyntheticsAPITest,
// 		Delete: deleteSyntheticsAPITest,
// 	},
// }

// func (instance *v1alpha1.DatadogGenericCRD) PerformOperation(op Operation) error {
// 	if typeHandlers, exists := handlers[instance.Spec.Type]; exists {
//         if handler, opExists := typeHandlers[op]; opExists {
//             return handler(q)
//         }
//         return fmt.Errorf("operation %s not supported for type %s", op, q.Spec.Type)
//     }
//     return fmt.Errorf("unsupported type: %s", q.Spec.Type)
// }

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

	instance := &v1alpha1.DatadogGenericCRD{}
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

	instanceSpecHash, err := comparison.GenerateMD5ForSpec(&instance.Spec)

	if err != nil {
		logger.Error(err, "error generating hash")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusUpdateError, "GeneratingGenericCRDSpecHash", err)
		return r.updateStatusIfNeeded(logger, instance, status, result)
	}

	shouldCreate := false
	shouldUpdate := false

	if instance.Status.ID == "" {
		shouldCreate = true
	} else {
		if instanceSpecHash != statusSpecHash {
			logger.Info("DatadogGenericCRD manifest has changed")
			shouldUpdate = true
		} else if instance.Status.LastForceSyncTime == nil || ((defaultForceSyncPeriod - now.Sub(instance.Status.LastForceSyncTime.Time)) <= 0) {
			// Periodically force a sync with the API to ensure parity
			// Get GenericCRD to make sure it exists before trying any updates. If it doesn't, set shouldCreate
			err = r.get(instance)
			if err != nil {
				logger.Error(err, "error getting custom resource", "custom resource ID", instance.Status.ID, "resource type", instance.Spec.Type)
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

func (r *Reconciler) get(instance *v1alpha1.DatadogGenericCRD) error {
	var err error
	switch instance.Spec.Type {
	case "synthetics_browser_test":
		_, err = getSyntheticsTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.ID)
	case "notebook":
		_, err = getNotebook(r.datadogAuth, r.datadogNotebooksClient, instance.Status.ID)
	default:
		err = fmt.Errorf("unsupported type: %s", instance.Spec.Type)
	}
	return err
}

func (r *Reconciler) update(logger logr.Logger, instance *v1alpha1.DatadogGenericCRD, status *v1alpha1.DatadogGenericCRDStatus, now metav1.Time, hash string) error {
	var err error
	switch instance.Spec.Type {
	case "synthetics_browser_test":
		if _, err = updateSyntheticsBrowserTest(r.datadogAuth, r.datadogSyntheticsClient, instance); err != nil {
			logger.Error(err, "error updating generic CRD", "generic CRD ID", instance.Status.ID)
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusUpdateError, "UpdatingGenericCRD", err)
			return err
		}
	case "notebook":
		if _, err = updateNotebook(r.datadogAuth, r.datadogNotebooksClient, instance); err != nil {
			logger.Error(err, "error updating generic CRD", "generic CRD ID", instance.Status.ID)
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusUpdateError, "UpdatingGenericCRD", err)
			return err
		}
	default:
		err = fmt.Errorf("unsupported type: %s", instance.Spec.Type)
		return err
	}

	event := buildEventInfo(instance.Name, instance.Namespace, datadog.UpdateEvent)
	r.recordEvent(instance, event)

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeUpdated, metav1.ConditionTrue, "UpdatingGenericCRD", "DatadogGenericCRD Update")
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash
	status.LastForceSyncTime = &now

	logger.Info("Updated Datadog Generic CRD", "Generic CRD ID", instance.Status.ID)
	return nil
}

func (r *Reconciler) create(logger logr.Logger, instance *v1alpha1.DatadogGenericCRD, status *v1alpha1.DatadogGenericCRDStatus, now metav1.Time, hash string) error {
	logger.V(1).Info("Custom resource ID is not set; creating custom resource in Datadog")

	switch instance.Spec.Type {
	case "synthetics_browser_test":
		createdTest, err := createSyntheticBrowserTest(r.datadogAuth, r.datadogSyntheticsClient, instance)
		logger.Info("pretty printing test", "browser test ID", createdTest.GetPublicId(), "test", createdTest)
		if err != nil {
			logger.Error(err, "error creating browser test")
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
			return err
		}
		logger.Info("created a new browser test", "browser test ID", createdTest.GetPublicId())
		status.ID = createdTest.GetPublicId()
		createdTimeString := createdTest.AdditionalProperties["created_at"].(string)
		createdTimeParsed, err := time.Parse(time.RFC3339, createdTimeString)
		if err != nil {
			logger.Error(err, "error parsing created time")
			createdTimeParsed = time.Now()
		}
		createdTime := metav1.NewTime(createdTimeParsed)
		status.Created = &createdTime
		status.LastForceSyncTime = &createdTime
		status.Creator = createdTest.AdditionalProperties["created_by"].(map[string]interface{})["handle"].(string)
		status.SyncStatus = v1alpha1.DatadogSyncStatusOK
		status.CurrentHash = hash
	case "notebook":
		createdNotebook, err := createNotebook(r.datadogAuth, r.datadogNotebooksClient, instance)
		logger.Info("pretty printing notebook", "notebook ID", createdNotebook.Data.GetId(), "notebook", createdNotebook)
		if err != nil {
			logger.Error(err, "error creating notebook")
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
			return err
		}
		logger.Info("created a new notebook", "notebook ID", createdNotebook.Data.GetId())
		status.ID = notebookInt64ToString(createdNotebook.Data.GetId())
		createdTime := metav1.NewTime(*createdNotebook.Data.GetAttributes().Created)
		status.Created = &createdTime
		status.LastForceSyncTime = &createdTime
		status.Creator = *createdNotebook.Data.GetAttributes().Author.Handle
		status.SyncStatus = v1alpha1.DatadogSyncStatusOK
		status.CurrentHash = hash
	default:
		err := fmt.Errorf("unsupported type: %s", instance.Spec.Type)
		return err
	}
	event := buildEventInfo(instance.Name, instance.Namespace, datadog.CreationEvent)
	r.recordEvent(instance, event)

	// Set condition and status
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeCreated, metav1.ConditionTrue, "CreatingGenericCRD", "DatadogGenericCRD Created")
	logger.Info("created a new DatadogGenericCRD", "generic CRD ID", status.ID)

	return nil
}

func updateErrStatus(status *v1alpha1.DatadogGenericCRDStatus, now metav1.Time, syncStatus v1alpha1.DatadogSyncStatus, reason string, err error) {
	condition.UpdateFailureStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeError, reason, err)
	status.SyncStatus = syncStatus
}

func (r *Reconciler) updateStatusIfNeeded(logger logr.Logger, instance *v1alpha1.DatadogGenericCRD, status *v1alpha1.DatadogGenericCRDStatus, result ctrl.Result) (ctrl.Result, error) {
	if !apiequality.Semantic.DeepEqual(&instance.Status, status) {
		instance.Status = *status
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			if apierrors.IsConflict(err) {
				logger.Error(err, "unable to update DatadogGenericCRD status due to update conflict")
				return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, nil
			}
			logger.Error(err, "unable to update DatadogGenericCRD status")
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
func (r *Reconciler) recordEvent(genericcrd runtime.Object, info utils.EventInfo) {
	r.recorder.Event(genericcrd, corev1.EventTypeNormal, info.GetReason(), info.GetMessage())
}
