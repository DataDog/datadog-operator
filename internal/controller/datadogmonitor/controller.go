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
	DDMonitorRequeuePeriodEnvVar   = "DD_MONITOR_REQUEUE_PERIOD"
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
	requeuePeriod := defaultRequeuePeriod

	if userForceSyncPeriod, ok := os.LookupEnv(DDMonitorForceSyncPeriodEnvVar); ok {
		forceSyncPeriodInt, err := strconv.Atoi(userForceSyncPeriod)
		if err != nil {
			logger.Error(err, "Invalid value for monitor force sync period. Defaulting to 60 minutes.")
		} else {
			logger.V(1).Info("Setting monitor force sync period", "minutes", forceSyncPeriodInt)
			forceSyncPeriod = time.Duration(forceSyncPeriodInt) * time.Minute
		}
	}

	if userRequeuePeriod, ok := os.LookupEnv(DDMonitorRequeuePeriodEnvVar); ok {
		requeuePeriodInt, err := strconv.Atoi(userRequeuePeriod)
		if err != nil {
			logger.Error(err, "Invalid value for monitor requeue period. Defaulting to 60 seconds.")
		} else {
			logger.V(1).Info("Setting monitor force sync period", "seconds", requeuePeriodInt)
			requeuePeriod = time.Duration(requeuePeriodInt) * time.Second
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
		} else if instance.Status.MonitorStateLastUpdateTime == nil || (requeuePeriod-now.Sub(instance.Status.MonitorStateLastUpdateTime.Time)) <= 0 {
			// If other conditions aren't met, and we have passed the requeuePeriod, then update monitor state
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

	// If reconcile was successful, requeue with period requeuePeriod
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = requeuePeriod
	}

	// Update the status
	return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
}

func (r *Reconciler) create(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time, instanceSpecHash string) error {
	// Validate monitor in Datadog
	if err := validateMonitor(r.datadogAuth, logger, r.datadogClient, datadogMonitor); err != nil {
		return err
	}

	// Create monitor in Datadog
	m, err := createMonitor(r.datadogAuth, logger, r.datadogClient, datadogMonitor)
	if err != nil {
		return err
	}
	event := buildEventInfo(datadogMonitor.Name, datadogMonitor.Namespace, pkgutils.CreationEvent)
	r.recordEvent(datadogMonitor, event)

	// As this is a new monitor, add static information to status
	status.ID = int(m.GetId())
	creator := m.GetCreator()
	status.Creator = creator.GetEmail()
	createdTime := metav1.NewTime(m.GetCreated())
	status.Created = &createdTime
	status.Primary = true
	status.MonitorStateSyncStatus = ""
	status.CurrentHash = instanceSpecHash

	// Set Created Condition
	condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeCreated, corev1.ConditionTrue, "DatadogMonitor Created")
	logger.Info("Created a new DatadogMonitor", "Monitor Namespace", datadogMonitor.Namespace, "Monitor Name", datadogMonitor.Name, "Monitor ID", m.GetId())

	return nil
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
