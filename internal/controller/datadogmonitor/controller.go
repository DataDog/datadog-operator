// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	datadogV1 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
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

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/config"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	pkgutils "github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	defaultRequeuePeriod           = 60 * time.Second
	defaultErrRequeuePeriod        = 5 * time.Second
	defaultForceSyncPeriod         = 60 * time.Minute
	maxTriggeredStateGroups        = 10
	DDMonitorForceSyncPeriodEnvVar = "DD_MONITOR_FORCE_SYNC_PERIOD"
)

var supportedMonitorTypes = map[string]bool{
	string(datadogV1.MONITORTYPE_METRIC_ALERT):          true,
	string(datadogV1.MONITORTYPE_QUERY_ALERT):           true,
	string(datadogV1.MONITORTYPE_SERVICE_CHECK):         true,
	string(datadogV1.MONITORTYPE_EVENT_ALERT):           true,
	string(datadogV1.MONITORTYPE_LOG_ALERT):             true,
	string(datadogV1.MONITORTYPE_PROCESS_ALERT):         true,
	string(datadogV1.MONITORTYPE_RUM_ALERT):             true,
	string(datadogV1.MONITORTYPE_TRACE_ANALYTICS_ALERT): true,
	string(datadogV1.MONITORTYPE_SLO_ALERT):             true,
	string(datadogV1.MONITORTYPE_EVENT_V2_ALERT):        true,
	string(datadogV1.MONITORTYPE_AUDIT_ALERT):           true,
}

const requiredTag = "generated:kubernetes"

// Reconciler reconciles a DatadogMonitor object
type Reconciler struct {
	client                 client.Client
	datadogClient          *datadogV1.MonitorsApi
	datadogAuth            context.Context
	log                    logr.Logger
	scheme                 *runtime.Scheme
	recorder               record.EventRecorder
	operatorMetricsEnabled bool
	forwarders             pkgutils.MetricsForwardersManager
}

// NewReconciler returns a new Reconciler object
func NewReconciler(client client.Client, creds config.Creds, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder, operatorMetricsEnabled bool, metricForwardersMgr pkgutils.MetricsForwardersManager) (*Reconciler, error) {
	ddClient, err := datadogclient.InitDatadogMonitorClient(log, creds)
	if err != nil {
		return &Reconciler{}, err
	}

	return &Reconciler{
		client:                 client,
		datadogClient:          ddClient.Client,
		datadogAuth:            ddClient.Auth,
		scheme:                 scheme,
		log:                    log,
		recorder:               recorder,
		operatorMetricsEnabled: operatorMetricsEnabled,
		forwarders:             metricForwardersMgr,
	}, nil
}

func (r *Reconciler) UpdateDatadogClient(newCreds config.Creds) error {
	r.log.Info("Recreating Datadog client due to credential change", "reconciler", "DatadogMonitor")
	ddClient, err := datadogclient.InitDatadogMonitorClient(r.log, newCreds)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client in DatadogMonitor: %w", err)
	}
	r.datadogClient = ddClient.Client
	r.datadogAuth = ddClient.Auth
	r.log.Info("Successfully recreated datadog client due to credential change", "reconciler", "DatadogMonitor")
	return nil
}

// Reconcile is similar to reconciler.Reconcile interface, but taking a context
func (r *Reconciler) Reconcile(ctx context.Context, instance *datadoghqv1alpha1.DatadogMonitor) (reconcile.Result, error) {
	res, err := r.internalReconcile(ctx, instance)

	if r.operatorMetricsEnabled {
		r.forwarders.ProcessError(instance, err)
	}

	return res, err
}

// Reconcile loop for DatadogMonitor
func (r *Reconciler) internalReconcile(ctx context.Context, instance *datadoghqv1alpha1.DatadogMonitor) (reconcile.Result, error) {
	logger := r.log.WithValues("datadogmonitor", pkgutils.GetNamespacedName(instance))
	logger.Info("Reconciling DatadogMonitor")
	now := metav1.NewTime(time.Now())
	forceSyncPeriod := defaultForceSyncPeriod

	if userForceSyncPeriod, ok := os.LookupEnv(DDMonitorForceSyncPeriodEnvVar); ok {
		forceSyncPeriodInt, err := strconv.Atoi(userForceSyncPeriod)
		if err != nil {
			logger.Error(err, "Invalid value for monitor force sync period. Defaulting to 60 minutes.")
		} else {
			logger.V(1).Info("Setting monitor force sync period", "minutes", forceSyncPeriodInt)
			forceSyncPeriod = time.Duration(forceSyncPeriodInt) * time.Minute
		}
	}

	var result ctrl.Result
	var err error

	newStatus := instance.Status.DeepCopy()

	if result, err = r.handleFinalizer(logger, instance); ctrutils.ShouldReturn(result, err) {
		return result, err
	}

	// Validate the DatadogMonitor spec
	if err = datadoghqv1alpha1.IsValidDatadogMonitor(&instance.Spec); err != nil {
		logger.Error(err, "invalid DatadogMonitor spec")

		return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
	}

	instanceSpecHash, err := comparison.GenerateMD5ForSpec(&instance.Spec)
	if err != nil {
		logger.Error(err, "error generating hash")

		return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
	}

	statusSpecHash := instance.Status.CurrentHash

	shouldCreate := false
	shouldUpdate := false

	// Check if we need to create the monitor, update the monitor definition, or update monitor state
	if instance.Status.ID == 0 {
		shouldCreate = true
	} else {
		// Perform drift detection to check if monitor exists in Datadog
		driftDetected, err := r.detectDrift(ctx, logger, instance, newStatus)
		if err != nil {
			logger.Error(err, "error during drift detection", "Monitor ID", instance.Status.ID)
			// Continue with reconciliation even if drift detection fails
		}

		if driftDetected {
			logger.Info("Drift detected: monitor not found in Datadog, will recreate", "Monitor ID", instance.Status.ID)
			err = r.handleMonitorRecreation(ctx, logger, instance, newStatus, now, instanceSpecHash)
			if err != nil {
				logger.Error(err, "error recreating monitor", "Monitor ID", instance.Status.ID)
			}
		} else {
			var m datadogV1.Monitor
			if instanceSpecHash != statusSpecHash {
				// Custom resource manifest has changed, need to update the API
				logger.V(1).Info("DatadogMonitor manifest has changed")
				shouldUpdate = true
			} else if instance.Status.MonitorLastForceSyncTime == nil || (forceSyncPeriod-now.Sub(instance.Status.MonitorLastForceSyncTime.Time)) <= 0 {
				// Periodically force a sync with the API monitor to ensure parity
				// Get monitor to make sure it exists before trying any updates. If it doesn't, set shouldCreate
				_, err = r.get(instance, newStatus)
				if err != nil {
					logger.Error(err, "error getting monitor", "Monitor ID", instance.Status.ID)
					if strings.Contains(err.Error(), ctrutils.NotFoundString) {
						shouldCreate = true
					}
				} else {
					shouldUpdate = true
				}
			} else if instance.Status.MonitorStateLastUpdateTime == nil || (defaultRequeuePeriod-now.Sub(instance.Status.MonitorStateLastUpdateTime.Time)) <= 0 {
				// If other conditions aren't met, and we have passed the defaultRequeuePeriod, then update monitor state
				// Get monitor to make sure it exists before trying any updates. If it doesn't, set shouldCreate
				m, err = r.get(instance, newStatus)
				if err != nil {
					logger.Error(err, "error getting monitor", "Monitor ID", instance.Status.ID)
					if strings.Contains(err.Error(), ctrutils.NotFoundString) {
						shouldCreate = true
					}
				}
				updateMonitorState(m, now, newStatus)
			}
		}
	}

	// Create and update actions
	if shouldCreate {
		if isSupportedMonitorType(instance.Spec.Type) {
			logger.V(1).Info("Creating monitor in Datadog")
			// Make sure required tags are present
			if !apiutils.BoolValue(instance.Spec.ControllerOptions.DisableRequiredTags) {
				if result, err = r.checkRequiredTags(logger, instance); err != nil || result.Requeue {
					return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
				}
			}
			if err = r.create(logger, instance, newStatus, now, instanceSpecHash); err != nil {
				logger.Error(err, "error creating monitor")
			}
		} else {
			err = fmt.Errorf("monitor type %v not supported", instance.Spec.Type)
			logger.Error(err, "error creating monitor")
		}
	} else if shouldUpdate {
		logger.V(1).Info("Updating monitor in Datadog")
		// Make sure required tags are present
		if !apiutils.BoolValue(instance.Spec.ControllerOptions.DisableRequiredTags) {
			if result, err = r.checkRequiredTags(logger, instance); err != nil || result.Requeue {
				return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
			}
		}
		if err = r.update(logger, instance, newStatus, now, instanceSpecHash); err != nil {
			logger.Error(err, "error updating monitor", "Monitor ID", instance.Status.ID)
		}
	}

	// If reconcile was successful, requeue with period defaultRequeuePeriod
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}

	// Update the status
	return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
}

func (r *Reconciler) create(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time, instanceSpecHash string) error {
	return r.createInternal(logger, datadogMonitor, status, now, instanceSpecHash, false)
}

func (r *Reconciler) createInternal(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time, instanceSpecHash string, isRecreation bool) error {
	// Validate monitor in Datadog
	if err := validateMonitor(r.datadogAuth, logger, r.datadogClient, datadogMonitor); err != nil {
		return err
	}

	// Create monitor in Datadog
	m, err := createMonitor(r.datadogAuth, logger, r.datadogClient, datadogMonitor)
	if err != nil {
		return err
	}

	// Determine event type based on whether this is recreation or initial creation
	var eventType pkgutils.EventType
	var logMessage string
	if isRecreation {
		eventType = pkgutils.RecreationEvent
		logMessage = "Recreated DatadogMonitor"
	} else {
		eventType = pkgutils.CreationEvent
		logMessage = "Created a new DatadogMonitor"
	}

	event := buildEventInfo(datadogMonitor.Name, datadogMonitor.Namespace, eventType)
	r.recordEvent(datadogMonitor, event)

	// Update status with new monitor information
	status.ID = int(m.GetId())
	creator := m.GetCreator()
	status.Creator = creator.GetEmail()
	createdTime := metav1.NewTime(m.GetCreated())
	status.Created = &createdTime
	status.Primary = true
	status.MonitorStateSyncStatus = ""
	status.CurrentHash = instanceSpecHash

	// Set appropriate condition based on operation type
	if isRecreation {
		condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeRecreated, corev1.ConditionTrue, "DatadogMonitor Recreated")
	} else {
		condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeCreated, corev1.ConditionTrue, "DatadogMonitor Created")
	}

	logger.Info(logMessage, "Monitor Namespace", datadogMonitor.Namespace, "Monitor Name", datadogMonitor.Name, "Monitor ID", m.GetId())

	return nil
}

// detectDrift checks if the monitor referenced by the DatadogMonitor exists in Datadog
func (r *Reconciler) detectDrift(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus) (bool, error) {
	// If no monitor ID is set, no drift can be detected
	if instance.Status.ID == 0 {
		return false, nil
	}

	// Attempt to get the monitor from Datadog
	_, err := getMonitor(r.datadogAuth, r.datadogClient, instance.Status.ID)
	if err != nil {
		// Check if the error indicates the monitor was not found
		if strings.Contains(err.Error(), ctrutils.NotFoundString) {
			logger.Info("Drift detected: monitor not found in Datadog", "Monitor ID", instance.Status.ID)
			// Update status to indicate drift was detected
			status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusGetError
			// Set drift detected condition with detailed message
			now := metav1.Now()
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeDriftDetected, corev1.ConditionTrue, fmt.Sprintf("Monitor ID %d not found in Datadog API", instance.Status.ID))
			return true, nil
		}

		// Handle different types of API errors gracefully with detailed error reporting
		errorMessage := err.Error()
		now := metav1.Now()

		if strings.Contains(errorMessage, "rate limit") || strings.Contains(errorMessage, "429") {
			logger.V(1).Info("Rate limit encountered during drift detection, will retry later", "Monitor ID", instance.Status.ID)
			status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusGetError
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Rate limit during drift detection for monitor ID %d: %s", instance.Status.ID, errorMessage))
			return false, fmt.Errorf("rate limit during drift detection, will retry: %w", err)
		}

		if strings.Contains(errorMessage, "unauthorized") || strings.Contains(errorMessage, "401") {
			logger.Error(err, "Authentication error during drift detection", "Monitor ID", instance.Status.ID)
			status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusGetError
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Authentication error during drift detection for monitor ID %d: credentials may be invalid", instance.Status.ID))
			return false, err
		}

		if strings.Contains(errorMessage, "forbidden") || strings.Contains(errorMessage, "403") {
			logger.Error(err, "Authorization error during drift detection", "Monitor ID", instance.Status.ID)
			status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusGetError
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Authorization error during drift detection for monitor ID %d: insufficient permissions", instance.Status.ID))
			return false, err
		}

		if strings.Contains(errorMessage, "timeout") || strings.Contains(errorMessage, "context deadline exceeded") {
			logger.V(1).Info("Timeout during drift detection, will retry", "Monitor ID", instance.Status.ID)
			status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusGetError
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Timeout during drift detection for monitor ID %d: API request timed out", instance.Status.ID))
			return false, fmt.Errorf("timeout during drift detection, will retry: %w", err)
		}

		// For other errors (API unavailable, service errors, etc.), handle gracefully
		logger.V(1).Info("Error during drift detection, will retry", "Monitor ID", instance.Status.ID, "error", errorMessage)
		status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusGetError
		condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("API error during drift detection for monitor ID %d: %s", instance.Status.ID, errorMessage))
		return false, fmt.Errorf("error during drift detection, will retry: %w", err)
	}

	// Monitor exists, no drift detected - clear any previous error conditions
	now := metav1.Now()
	condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionFalse, "")
	return false, nil
}

func (r *Reconciler) update(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time, instanceSpecHash string) error {
	// Validate monitor in Datadog
	if err := validateMonitor(r.datadogAuth, logger, r.datadogClient, datadogMonitor); err != nil {
		status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusValidateError
		return err
	}

	// Update monitor in Datadog
	if _, err := updateMonitor(r.datadogAuth, logger, r.datadogClient, datadogMonitor); err != nil {
		status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusUpdateError
		return err
	}

	event := buildEventInfo(datadogMonitor.Name, datadogMonitor.Namespace, pkgutils.UpdateEvent)
	r.recordEvent(datadogMonitor, event)

	// Set Updated Condition
	condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated, corev1.ConditionTrue, "DatadogMonitor Updated")
	status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusOK
	status.MonitorLastForceSyncTime = &now
	status.CurrentHash = instanceSpecHash
	logger.V(1).Info("Updated DatadogMonitor", "Monitor Namespace", datadogMonitor.Namespace, "Monitor Name", datadogMonitor.Name, "Monitor ID", datadogMonitor.Status.ID)

	return nil
}

func (r *Reconciler) get(datadogMonitor *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus) (datadogV1.Monitor, error) {
	// Get monitor from Datadog and update resource status if needed
	m, err := getMonitor(r.datadogAuth, r.datadogClient, datadogMonitor.Status.ID)
	if err != nil {
		status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusGetError
		return m, err
	}
	return m, nil
}

func updateMonitorState(m datadogV1.Monitor, now metav1.Time, status *datadoghqv1alpha1.DatadogMonitorStatus) {
	convertStateToStatus(m, status, now)
	status.MonitorStateLastUpdateTime = &now
	status.MonitorStateSyncStatus = datadoghqv1alpha1.MonitorStateSyncStatusOK
}

func (r *Reconciler) updateStatusIfNeeded(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor, now metav1.Time, status *datadoghqv1alpha1.DatadogMonitorStatus, currentErr error, result ctrl.Result) (ctrl.Result, error) {
	// Update Error and Active conditions
	condition.SetErrorActiveConditions(status, now, currentErr)

	if !apiequality.Semantic.DeepEqual(&datadogMonitor.Status, status) {
		datadogMonitor.Status = *status
		if err := r.client.Status().Update(context.TODO(), datadogMonitor); err != nil {
			if apierrors.IsConflict(err) {
				logger.Error(err, "unable to update DatadogMonitor status due to update conflict")
				return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, nil
			}
			logger.Error(err, "unable to update DatadogMonitor status")

			return ctrl.Result{}, err
		}
		// This is brittle; typically if a Spec or Status is updated in the API, the result gets requeued without additional action.
		// However, sometimes apiequality.Semantic.DeepEqual() is false even when the API thinks they are equal (and no update is made).
		// Thus, the result does not get requeued after entering this `if` block. To safeguard this, we will always requeue the result
		// here. The danger of this is potentially requeueing twice for every status update. In most circumstances this is
		// not an issue, but if a monitor has many groups and is "flapping", then it can cause a flood of updates to
		// the Status.TriggeredState and put pressure on the controller. As a safeguard against this, the maximum number
		// of groups stored in Status.TriggeredState should be conservative.
		return ctrl.Result{RequeueAfter: defaultRequeuePeriod}, nil
	}

	return result, nil
}

func (r *Reconciler) checkRequiredTags(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor) (ctrl.Result, error) {
	tagsToAdd := []string{}
	var found bool
	tags := datadogMonitor.Spec.Tags
	for _, rT := range getRequiredTags() {
		found = slices.Contains(tags, rT)
		if !found {
			tagsToAdd = append(tagsToAdd, rT)
		}
	}

	if len(tagsToAdd) > 0 {
		tags = append(tags, tagsToAdd...)
		datadogMonitor.Spec.Tags = tags
		err := r.client.Update(context.TODO(), datadogMonitor)
		if err != nil {
			logger.Error(err, "failed to update DatadogMonitor with required tags")

			return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
		}
		logger.Info("Added required tags", "Monitor Namespace", datadogMonitor.Namespace, "Monitor Name", datadogMonitor.Name, "Monitor ID", datadogMonitor.Status.ID)

		return ctrl.Result{RequeueAfter: defaultRequeuePeriod}, nil
	}

	// Proceed in reconcile loop without modifying result.
	return ctrl.Result{}, nil
}

func getRequiredTags() []string {
	return []string{requiredTag}
}

// convertStateToStatus updates status.MonitorState, status.TriggeredState, and status.DowntimeStatus according to the current state of the monitor
func convertStateToStatus(monitor datadogV1.Monitor, newStatus *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time) {
	// If monitor group is in Alert, Warn or No Data, then add its info to the TriggeredState
	triggeredStates := []datadoghqv1alpha1.DatadogMonitorTriggeredState{}
	monitorState, exists := monitor.GetStateOk()
	if exists {
		monitorGroups, exists := monitorState.GetGroupsOk()
		if exists {
			var groupStatus datadogV1.MonitorOverallStates
			for group, monitorStateGroup := range *monitorGroups {
				groupStatus = monitorStateGroup.GetStatus()
				if isTriggered(string(groupStatus)) {
					triggeredStates = append(triggeredStates, datadoghqv1alpha1.DatadogMonitorTriggeredState{
						MonitorGroup:       group,
						State:              datadoghqv1alpha1.DatadogMonitorState(groupStatus),
						LastTransitionTime: metav1.Unix(utils.GetMax(monitorStateGroup.GetLastTriggeredTs(), monitorStateGroup.GetLastNodataTs()), 0),
					})
				}
			}
		}
	}
	sort.SliceStable(triggeredStates, func(i, j int) bool { return triggeredStates[i].MonitorGroup < triggeredStates[j].MonitorGroup })
	if len(triggeredStates) > maxTriggeredStateGroups {
		// Cap the size of Status.TrigggeredState
		triggeredStates = triggeredStates[0:maxTriggeredStateGroups]
	}
	newStatus.TriggeredState = triggeredStates

	oldMonitorState := newStatus.MonitorState
	newStatus.MonitorState = datadoghqv1alpha1.DatadogMonitorState(monitor.GetOverallState())
	// An accurate LastTransitionTime requires looping through four timestamps in every MonitorGroup, so using an approximation based on sync time
	if newStatus.MonitorState != oldMonitorState {
		newStatus.MonitorStateLastTransitionTime = &now
	}
	// TODO Updating this requires having the API client also return any matching downtime objects
	newStatus.DowntimeStatus = datadoghqv1alpha1.DatadogMonitorDowntimeStatus{}
}

func isSupportedMonitorType(monitorType datadoghqv1alpha1.DatadogMonitorType) bool {
	return supportedMonitorTypes[string(monitorType)]
}

func isTriggered(groupStatus string) bool {
	return groupStatus == string(datadoghqv1alpha1.DatadogMonitorStateAlert) || groupStatus == string(datadoghqv1alpha1.DatadogMonitorStateWarn) || groupStatus == string(datadoghqv1alpha1.DatadogMonitorStateNoData)
}

// handleMonitorRecreation manages the recreation of a deleted monitor
func (r *Reconciler) handleMonitorRecreation(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time, instanceSpecHash string) error {
	logger.Info("Starting monitor recreation", "Monitor ID", instance.Status.ID, "Monitor Name", instance.Spec.Name)

	// Store the old monitor ID for logging and error recovery
	oldMonitorID := instance.Status.ID

	// Check if the resource was deleted during processing
	if ctx.Err() != nil {
		logger.V(1).Info("Context cancelled during recreation, aborting", "Monitor ID", oldMonitorID)
		return ctx.Err()
	}

	// Validate the monitor spec before attempting recreation
	if err := datadoghqv1alpha1.IsValidDatadogMonitor(&instance.Spec); err != nil {
		logger.Error(err, "Invalid monitor spec, cannot recreate", "Monitor ID", oldMonitorID)
		// Don't attempt recreation for validation errors
		return fmt.Errorf("validation error prevents recreation: %w", err)
	}

	// Implement optimistic locking by checking resource version hasn't changed
	// This helps prevent conflicts during concurrent operations
	originalResourceVersion := instance.ResourceVersion

	// Reset the monitor ID to trigger creation logic
	status.ID = 0

	// Use the internal create method with recreation flag
	err := r.createInternal(logger, instance, status, now, instanceSpecHash, true)
	if err != nil {
		// Restore original ID on error to maintain state consistency
		status.ID = oldMonitorID

		// Check if this is a conflict error (resource was modified concurrently)
		if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "resource version") {
			logger.V(1).Info("Concurrent modification detected during recreation, will retry", "Monitor ID", oldMonitorID, "ResourceVersion", originalResourceVersion)
			now := metav1.Now()
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Concurrent modification detected during recreation of monitor ID %d: resource version conflict", oldMonitorID))
			return fmt.Errorf("concurrent modification during recreation, will retry: %w", err)
		}

		// Categorize and handle different types of creation errors with detailed status reporting
		errorMessage := err.Error()
		now := metav1.Now()

		if strings.Contains(errorMessage, "rate limit") || strings.Contains(errorMessage, "429") {
			logger.V(1).Info("Rate limit during recreation, will retry", "Old Monitor ID", oldMonitorID)
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Rate limit during recreation of monitor ID %d: API rate limit exceeded, will retry", oldMonitorID))
			return fmt.Errorf("rate limit during recreation, will retry: %w", err)
		}

		if strings.Contains(errorMessage, "unauthorized") || strings.Contains(errorMessage, "401") {
			logger.Error(err, "Authentication error during recreation", "Old Monitor ID", oldMonitorID)
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Authentication error during recreation of monitor ID %d: credentials are invalid or expired", oldMonitorID))
			return fmt.Errorf("authentication error during recreation: %w", err)
		}

		if strings.Contains(errorMessage, "forbidden") || strings.Contains(errorMessage, "403") {
			logger.Error(err, "Authorization error during recreation", "Old Monitor ID", oldMonitorID)
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Authorization error during recreation of monitor ID %d: insufficient permissions to create monitors", oldMonitorID))
			return fmt.Errorf("authorization error during recreation: %w", err)
		}

		if strings.Contains(errorMessage, "validation") || strings.Contains(errorMessage, "400") {
			logger.Error(err, "Validation error during recreation", "Old Monitor ID", oldMonitorID)
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Validation error during recreation of monitor ID %d: monitor configuration is invalid", oldMonitorID))
			return fmt.Errorf("validation error during recreation: %w", err)
		}

		if strings.Contains(errorMessage, "timeout") || strings.Contains(errorMessage, "context deadline exceeded") {
			logger.V(1).Info("Timeout during recreation, will retry", "Old Monitor ID", oldMonitorID)
			condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Timeout during recreation of monitor ID %d: API request timed out, will retry", oldMonitorID))
			return fmt.Errorf("timeout during recreation, will retry: %w", err)
		}

		// Generic error handling for other API errors
		logger.Error(err, "Failed to recreate monitor", "Old Monitor ID", oldMonitorID)
		condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("Failed to recreate monitor ID %d: %s", oldMonitorID, errorMessage))
		return fmt.Errorf("failed to recreate monitor: %w", err)
	}

	// Check for context cancellation after recreation but before finalizing status
	if ctx.Err() != nil {
		// Restore original ID since the operation was cancelled
		status.ID = oldMonitorID
		logger.V(1).Info("Context cancelled after recreation, operation may be incomplete", "Old Monitor ID", oldMonitorID)
		return ctx.Err()
	}

	logger.Info("Successfully recreated monitor", "Old Monitor ID", oldMonitorID, "New Monitor ID", status.ID)
	return nil
}
