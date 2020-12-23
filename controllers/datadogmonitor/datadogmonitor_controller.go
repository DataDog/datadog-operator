// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package datadogmonitor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

const (
	defaultRequeuePeriod    = 60 * time.Second
	defaultErrRequeuePeriod = 5 * time.Second
	maxTriggeredStateGroups = 10
)

// Reconciler reconciles a DatadogMonitor object
type Reconciler struct {
	Client        client.Client
	datadogClient *datadogapiclientv1.APIClient
	datadogAuth   context.Context
	VersionInfo   *version.Info
	Log           logr.Logger
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogmonitors/finalizers,verbs=get;list;watch;create;update;patch;delete

func (r *Reconciler) initDatadogClient() error {
	// TODO support secret configuration in the operator
	apiKey := os.Getenv(config.DDAPIKeyEnvVar)
	appKey := os.Getenv(config.DDAppKeyEnvVar)

	if apiKey == "" || appKey == "" {
		return errors.New("error obtaining api key and/or app key")
	}

	// Initialize the official Datadog V1 API client
	authV1 := context.WithValue(
		context.Background(),
		datadogapiclientv1.ContextAPIKeys,
		map[string]datadogapiclientv1.APIKey{
			"apiKeyAuth": {
				Key: apiKey,
			},
			"appKeyAuth": {
				Key: appKey,
			},
		},
	)
	configV1 := datadogapiclientv1.NewConfiguration()

	if apiURL := os.Getenv(config.DDURLEnvVar); apiURL != "" {
		parsedAPIURL, parseErr := url.Parse(apiURL)
		if parseErr != nil {
			return fmt.Errorf(`invalid API Url : %v`, parseErr)
		}
		if parsedAPIURL.Host == "" || parsedAPIURL.Scheme == "" {
			return fmt.Errorf(`missing protocol or host : %v`, apiURL)
		}
		// If api url is passed, set and use the api name and protocol on ServerIndex{1}
		authV1 = context.WithValue(authV1, datadogapiclientv1.ContextServerIndex, 1)
		authV1 = context.WithValue(authV1, datadogapiclientv1.ContextServerVariables, map[string]string{
			"name":     parsedAPIURL.Host,
			"protocol": parsedAPIURL.Scheme,
		})
	}
	r.datadogClient = datadogapiclientv1.NewAPIClient(configV1)
	r.datadogAuth = authV1

	return nil
}

// Reconcile loop for DatadogMonitor
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("datadogmonitor", req.NamespacedName)
	logger.Info("Reconciling DatadogMonitor")
	now := metav1.NewTime(time.Now())

	// Get instance
	instance := &datadoghqv1alpha1.DatadogMonitor{}
	var result ctrl.Result
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return result, nil
		}
		// Error reading the object - requeue the request
		return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
	}

	newStatus := instance.Status.DeepCopy()

	if result, err = r.handleFinalizer(logger, instance); err != nil || result.Requeue {
		return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
	}

	if r.datadogClient == nil {
		err = r.initDatadogClient()
		if err != nil {
			logger.Error(err, "Error initializing Datadog client")
			return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
		}
	}

	// Validate the DatadogMonitor spec
	if err = datadoghqv1alpha1.IsValidDatadogMonitor(&instance.Spec); err != nil {
		logger.Error(err, "Invalid DatadogMonitor spec")
		return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)

	}

	instanceSpecHash, err := comparison.GenerateMD5ForSpec(&instance.Spec)
	if err != nil {
		logger.Error(err, "Error generating hash", "err", err)
		return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)

	}
	statusSpecHash := instance.Status.CurrentHash

	// Create or update monitor, or check monitor state. Fall through this block (without returning)
	// if the result should be requeued with the default period
	if instance.Status.ID == 0 {
		// If the monitor ID is 0, then it doesn't exist yet in Datadog. Create the monitor (only metric alerts)
		if string(instance.Spec.Type) != string(datadogapiclientv1.MONITORTYPE_METRIC_ALERT) {
			err = errors.New("monitor type not supported")
			logger.Error(err, "For the alpha version, only metric alert type monitors are supported")
			return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
		}
		// Make sure required tags are present
		if result, err = r.checkRequiredTags(logger, instance); err != nil || result.Requeue {
			return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
		}
		err = r.create(logger, instance, newStatus, now)
		if err != nil {
			logger.Error(err, "Error creating monitor")
		}
		newStatus.CurrentHash = instanceSpecHash
	} else {
		// Check if instance needs to be updated
		if instanceSpecHash != statusSpecHash {
			// Make sure required tags are present
			if result, err = r.checkRequiredTags(logger, instance); err != nil || result.Requeue {
				return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
			}
			// Update action
			err = r.update(logger, instance, newStatus, now)
			if err != nil {
				logger.Error(err, "Error updating monitor")
			}
			newStatus.CurrentHash = instanceSpecHash
		} else {
			// Spec has not changed, just check if monitor state has changed (alert, warn, OK, etc.)
			err = r.get(logger, instance, newStatus)
			if err != nil {
				logger.Error(err, "Error getting monitor")
			}
		}
	}

	// Requeue
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}

	// Update the status via the Kubernetes API
	return r.updateStatusIfNeeded(logger, instance, now, newStatus, err, result)
}

func (r *Reconciler) create(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time) error {
	// Validate monitor in Datadog
	err := r.validateMonitor(datadogMonitor)
	if err != nil {
		return err
	}

	// Create monitor in Datadog
	m, err := r.createMonitor(datadogMonitor)
	if err != nil {
		return err
	}
	event := buildEventInfo(datadogMonitor.Name, datadogMonitor.Namespace, datadog.CreationEvent)
	r.recordEvent(datadogMonitor, event)

	// As this is a new monitor, add static information to status
	status.ID = int(m.GetId())
	creator := m.GetCreator()
	status.Creator = creator.GetEmail()
	createdTime := metav1.NewTime(m.GetCreated())
	status.Created = &createdTime
	status.Primary = true

	// Set Created Condition
	condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeCreated, corev1.ConditionTrue, "DatadogMonitor Created")
	logger.Info("Created a new DatadogMonitor", "datadogMonitor.Namespace", datadogMonitor.Namespace, "datadogMonitor.Name", datadogMonitor.Name, "m.ID", m.GetId())

	return nil
}

func (r *Reconciler) update(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor, status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time) error {
	// Validate monitor in Datadog
	err := r.validateMonitor(datadogMonitor)
	if err != nil {
		return err
	}

	// Update monitor in Datadog
	_, err = r.updateMonitor(datadogMonitor)
	if err != nil {
		return err
	}

	// Set Updated Condition
	condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated, corev1.ConditionTrue, "DatadogMonitor Updated")
	logger.Info("Updated DatadogMonitor", "DatadogMonitor.Namespace", datadogMonitor.Namespace, "datadogMonitor.Name", datadogMonitor.Name, "datadogMonitor.Status.ID", datadogMonitor.Status.ID)

	return nil
}

func (r *Reconciler) get(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor, newStatus *datadoghqv1alpha1.DatadogMonitorStatus) error {
	// Get monitor from Datadog and update resource status if needed
	m, err := r.getMonitor(datadogMonitor.Status.ID)
	if err != nil {
		return err
	}

	convertStateToStatus(m, newStatus)
	logger.Info("Synced DatadogMonitor state", "DatadogMonitor.Namespace", datadogMonitor.Namespace, "datadogMonitor.Name", datadogMonitor.Name)

	return nil
}

func (r *Reconciler) updateStatusIfNeeded(logger logr.Logger, datadogMonitor *datadoghqv1alpha1.DatadogMonitor, now metav1.Time, status *datadoghqv1alpha1.DatadogMonitorStatus, currentErr error, result ctrl.Result) (ctrl.Result, error) {
	// Update Error and Active conditions
	setErrorActiveConditions(status, now, currentErr)

	if !apiequality.Semantic.DeepEqual(&datadogMonitor.Status, status) {
		datadogMonitorCopy := datadogMonitor.DeepCopy()
		datadogMonitorCopy.Status = *status
		if err := r.Client.Status().Update(context.TODO(), datadogMonitorCopy); err != nil {
			if apierrors.IsConflict(err) {
				logger.Error(err, "Unable to update DatadogMonitor status due to update conflict")
				return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, nil
			}
			logger.Error(err, "Unable to update DatadogMonitor status")
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
		found = false
		for _, t := range tags {
			if t == rT {
				found = true
				break
			}
		}
		if !found {
			tagsToAdd = append(tagsToAdd, rT)
		}
	}

	if len(tagsToAdd) > 0 {
		tags = append(tags, tagsToAdd...)
		datadogMonitor.Spec.Tags = tags
		err := r.Client.Update(context.TODO(), datadogMonitor)
		if err != nil {
			logger.Error(err, "Failed to update DatadogMonitor with required tags")
			return ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod}, err
		}
		return ctrl.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod}, nil
	}

	// Proceed in reconcile loop
	return ctrl.Result{}, nil
}

// SetupWithManager creates a new DatadogMonitor controller
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&datadoghqv1alpha1.DatadogMonitor{}).
		Complete(r)
}

func getRequiredTags() []string {
	return []string{"generated:kubernetes"}
}

// convertStateToStatus updates status.MonitorState, status.TriggeredState, and status.DowntimeStatus according to the current state of the monitor
func convertStateToStatus(monitor datadogapiclientv1.Monitor, newStatus *datadoghqv1alpha1.DatadogMonitorStatus) {

	// If monitor group is in Alert, Warn or No Data, then add its info to the TriggeredState
	triggeredStates := []datadoghqv1alpha1.DatadogMonitorTriggeredState{}
	monitorState, exists := monitor.GetStateOk()
	if exists {
		monitorGroups, exists := monitorState.GetGroupsOk()
		if exists {
			var groupStatus datadogapiclientv1.MonitorOverallStates
			for group, monitorStateGroup := range *monitorGroups {
				groupStatus = monitorStateGroup.GetStatus()
				if isTriggered(string(groupStatus)) {
					triggeredStates = append(triggeredStates, datadoghqv1alpha1.DatadogMonitorTriggeredState{
						MonitorGroup:     group,
						State:            datadoghqv1alpha1.DatadogMonitorState(groupStatus),
						LastTransitionTs: getMax(monitorStateGroup.GetLastTriggeredTs(), monitorStateGroup.GetLastNodataTs()),
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
	newStatus.MonitorState = datadoghqv1alpha1.DatadogMonitorState(monitor.GetOverallState())
	// TODO Updating this requires having the API client also return any matching downtime objects
	newStatus.DowntimeStatus = datadoghqv1alpha1.DatadogMonitorDowntimeStatus{}
}

func setErrorActiveConditions(status *datadoghqv1alpha1.DatadogMonitorStatus, now metav1.Time, err error) {
	if err != nil {
		// Set the error condition to True
		condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionTrue, fmt.Sprintf("%v", err))
		// Set the active condition to False
		condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeActive, corev1.ConditionFalse, "DatadogMonitor error")
	} else {
		// Set the error condition to False
		condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeError, corev1.ConditionFalse, "")
		// Set the active condition to True
		condition.UpdateDatadogMonitorConditions(status, now, datadoghqv1alpha1.DatadogMonitorConditionTypeActive, corev1.ConditionTrue, "DatadogMonitor OK")
	}
}

func isTriggered(groupStatus string) bool {
	return groupStatus == string(datadoghqv1alpha1.DatadogMonitorStateAlert) || groupStatus == string(datadoghqv1alpha1.DatadogMonitorStateWarn) || groupStatus == string(datadoghqv1alpha1.DatadogMonitorStateNoData)
}
