package v2alpha1

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// DatadogAgentState type representing the deployment state of the different Agent components.
type DatadogAgentState string

const (
	// DatadogAgentStateProgressing the deployment is progressing.
	DatadogAgentStateProgressing DatadogAgentState = "Progressing"
	// DatadogAgentStateRunning the deployment is running properly.
	DatadogAgentStateRunning DatadogAgentState = "Running"
	// DatadogAgentStateUpdating the deployment is currently under a rolling update.
	DatadogAgentStateUpdating DatadogAgentState = "Updating"
	// DatadogAgentStateCanary the deployment is currently under a canary testing (EDS only).
	DatadogAgentStateCanary DatadogAgentState = "Canary"
	// DatadogAgentStateFailed the current state of the deployment is considered as Failed.
	DatadogAgentStateFailed DatadogAgentState = "Failed"
)

// UpdateDatadogAgentStatusConditionsFailure used to update the failure StatusConditions
func UpdateDatadogAgentStatusConditionsFailure(status *DatadogAgentStatus, now metav1.Time, conditionType, reason, message string, err error) {
	if err != nil {
		UpdateDatadogAgentStatusConditions(status, now, conditionType, metav1.ConditionTrue, reason, fmt.Sprintf("msg:%s, err:%v", message, err), true)
	} else {
		UpdateDatadogAgentStatusConditions(status, now, conditionType, metav1.ConditionFalse, reason, message, true)
	}
}

// UpdateDatadogAgentStatusConditions used to update a specific string in conditions
func UpdateDatadogAgentStatusConditions(status *DatadogAgentStatus, now metav1.Time, t string, conditionStatus metav1.ConditionStatus, reason, message string, writeFalseIfNotExist bool) {
	idConditionComplete := getIndexForConditionType(status, t)
	if idConditionComplete >= 0 {
		UpdateDatadogAgentStatusCondition(&status.Conditions[idConditionComplete], now, t, conditionStatus, reason, message)
	} else if conditionStatus == metav1.ConditionTrue || writeFalseIfNotExist {
		// Only add if the condition is True
		status.Conditions = append(status.Conditions, NewDatadogAgentStatusCondition(t, conditionStatus, now, reason, message))
	}
}

// UpdateDatadogAgentStatusCondition used to update a specific string
func UpdateDatadogAgentStatusCondition(condition *metav1.Condition, now metav1.Time, t string, conditionStatus metav1.ConditionStatus, reason, message string) *metav1.Condition {
	if condition.Status != conditionStatus {
		condition.LastTransitionTime = now
		condition.Status = conditionStatus
	}
	condition.Message = message
	condition.Reason = reason

	return condition
}

// SetDatadogAgentStatusCondition use to set a condition
func SetDatadogAgentStatusCondition(status *DatadogAgentStatus, condition *metav1.Condition) {
	idConditionComplete := getIndexForConditionType(status, condition.Type)
	if idConditionComplete >= 0 {
		status.Conditions[idConditionComplete] = *condition
	} else {
		status.Conditions = append(status.Conditions, *condition)
	}
}

// DeleteDatadogAgentStatusCondition is used to delete a condition
func DeleteDatadogAgentStatusCondition(status *DatadogAgentStatus, conditionType string) {
	idConditionComplete := getIndexForConditionType(status, conditionType)
	if idConditionComplete >= 0 {
		status.Conditions = append(status.Conditions[:idConditionComplete], status.Conditions[idConditionComplete+1:]...)
	}
}

// NewDatadogAgentStatusCondition returns new metav1.Condition instance
func NewDatadogAgentStatusCondition(conditionType string, conditionStatus metav1.ConditionStatus, now metav1.Time, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
}

// GetMetav1ConditionStatus converts a boolean to metav1.ConditionStatus
func GetMetav1ConditionStatus(status bool) metav1.ConditionStatus {
	if status {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func getIndexForConditionType(status *DatadogAgentStatus, t string) int {
	idCondition := -1
	if status == nil {
		return idCondition
	}

	for i, condition := range status.Conditions {
		if condition.Type == t {
			idCondition = i
			break
		}
	}

	return idCondition
}

// UpdateDeploymentStatus updates a deployment's DeploymentStatus
func UpdateDeploymentStatus(dep *appsv1.Deployment, depStatus *DeploymentStatus, updateTime *metav1.Time) *DeploymentStatus {
	if depStatus == nil {
		depStatus = &DeploymentStatus{}
	}
	if dep == nil {
		depStatus.State = string(DatadogAgentStateFailed)
		depStatus.Status = string(DatadogAgentStateFailed)
		return depStatus
	}

	if hash, ok := dep.Annotations[MD5AgentDeploymentAnnotationKey]; ok {
		depStatus.CurrentHash = hash
	}
	if updateTime != nil {
		depStatus.LastUpdate = updateTime
	}
	depStatus.Replicas = dep.Status.Replicas
	depStatus.UpdatedReplicas = dep.Status.UpdatedReplicas
	depStatus.AvailableReplicas = dep.Status.AvailableReplicas
	depStatus.UnavailableReplicas = dep.Status.UnavailableReplicas
	depStatus.ReadyReplicas = dep.Status.ReadyReplicas

	// Deciding on deployment status based on Deployment status
	var deploymentState DatadogAgentState
	for _, condition := range dep.Status.Conditions {
		if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue {
			deploymentState = DatadogAgentStateFailed
		}
	}

	if deploymentState == "" {
		switch {
		case depStatus.UpdatedReplicas != depStatus.Replicas:
			deploymentState = DatadogAgentStateUpdating
		case depStatus.ReadyReplicas == 0:
			deploymentState = DatadogAgentStateProgressing
		default:
			deploymentState = DatadogAgentStateRunning
		}
	}

	depStatus.State = fmt.Sprintf("%v", deploymentState)
	depStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", deploymentState, depStatus.Replicas, depStatus.ReadyReplicas, depStatus.UpdatedReplicas)
	depStatus.DeploymentName = dep.ObjectMeta.Name
	return depStatus
}

// UpdateDaemonSetStatus updates a daemonset's DaemonSetStatus
func UpdateDaemonSetStatus(ds *appsv1.DaemonSet, dsStatus []*DaemonSetStatus, updateTime *metav1.Time) []*DaemonSetStatus {
	if dsStatus == nil {
		dsStatus = []*DaemonSetStatus{}
	}
	if ds == nil {
		dsStatus = append(dsStatus, &DaemonSetStatus{
			State:  string(DatadogAgentStateFailed),
			Status: string(DatadogAgentStateFailed),
		})
		return dsStatus
	}

	newStatus := DaemonSetStatus{
		Desired:       ds.Status.DesiredNumberScheduled,
		Current:       ds.Status.CurrentNumberScheduled,
		Ready:         ds.Status.NumberReady,
		Available:     ds.Status.NumberAvailable,
		UpToDate:      ds.Status.UpdatedNumberScheduled,
		DaemonsetName: ds.ObjectMeta.Name,
	}

	if updateTime != nil {
		newStatus.LastUpdate = updateTime
	}
	if hash, ok := ds.Annotations[MD5AgentDeploymentAnnotationKey]; ok {
		newStatus.CurrentHash = hash
	}

	var deploymentState DatadogAgentState
	switch {
	case newStatus.UpToDate != newStatus.Desired:
		deploymentState = DatadogAgentStateUpdating
	case newStatus.Ready == 0 && newStatus.Desired != 0:
		deploymentState = DatadogAgentStateProgressing
	default:
		deploymentState = DatadogAgentStateRunning
	}

	newStatus.State = fmt.Sprintf("%v", deploymentState)
	newStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", deploymentState, newStatus.Desired, newStatus.Ready, newStatus.UpToDate)

	// match ds name to ds status
	found := false
	for id := range dsStatus {
		if dsStatus[id].DaemonsetName == newStatus.DaemonsetName {
			*dsStatus[id] = newStatus
			found = true
		}
	}
	if !found {
		dsStatus = append(dsStatus, &newStatus)
	}

	return dsStatus
}

// UpdateExtendedDaemonSetStatus updates an ExtendedDaemonSet's DaemonSetStatus
func UpdateExtendedDaemonSetStatus(eds *edsdatadoghqv1alpha1.ExtendedDaemonSet, dsStatus []*DaemonSetStatus, updateTime *metav1.Time) []*DaemonSetStatus {
	if dsStatus == nil {
		dsStatus = []*DaemonSetStatus{}
	}

	newStatus := DaemonSetStatus{
		Desired:       eds.Status.Desired,
		Current:       eds.Status.Current,
		Ready:         eds.Status.Ready,
		Available:     eds.Status.Available,
		UpToDate:      eds.Status.UpToDate,
		DaemonsetName: eds.ObjectMeta.Name,
	}

	if updateTime != nil {
		newStatus.LastUpdate = updateTime
	}
	if hash, ok := eds.Annotations[MD5AgentDeploymentAnnotationKey]; ok {
		newStatus.CurrentHash = hash
	}

	var deploymentState DatadogAgentState
	switch {
	case eds.Status.Canary != nil:
		deploymentState = DatadogAgentStateCanary
	case newStatus.UpToDate != newStatus.Desired:
		deploymentState = DatadogAgentStateUpdating
	case newStatus.Ready == 0 && newStatus.Desired != 0:
		deploymentState = DatadogAgentStateProgressing
	default:
		deploymentState = DatadogAgentStateRunning
	}

	newStatus.State = fmt.Sprintf("%v", deploymentState)
	newStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", deploymentState, newStatus.Desired, newStatus.Ready, newStatus.UpToDate)

	// match eds name to eds status
	found := false
	for id := range dsStatus {
		if dsStatus[id].DaemonsetName == newStatus.DaemonsetName {
			*dsStatus[id] = newStatus
			found = true
		}
	}
	if !found {
		dsStatus = append(dsStatus, &newStatus)
	}

	return dsStatus
}

// UpdateCombinedDaemonSetStatus combines the status of multiple DaemonSetStatus
func UpdateCombinedDaemonSetStatus(dsStatus []*DaemonSetStatus) *DaemonSetStatus {
	combinedStatus := DaemonSetStatus{}
	if len(dsStatus) == 0 {
		return &combinedStatus
	}

	for _, status := range dsStatus {
		combinedStatus.Desired += status.Desired
		combinedStatus.Current += status.Current
		combinedStatus.Ready += status.Ready
		combinedStatus.Available += status.Available
		combinedStatus.UpToDate += status.UpToDate
		if combinedStatus.LastUpdate.Before(status.LastUpdate) {
			combinedStatus.LastUpdate = status.LastUpdate
		}
		combinedStatus.State = getCombinedState(combinedStatus.State, status.State)
		combinedStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", combinedStatus.State, combinedStatus.Desired, combinedStatus.Ready, combinedStatus.UpToDate)
	}

	return &combinedStatus
}

func getCombinedState(currentState, newState string) string {
	currentNum := assignNumeralState(currentState)
	newNum := assignNumeralState(newState)

	if currentNum == 0 {
		return newState
	}
	if newNum == 0 {
		return currentState
	}
	if currentNum < newNum {
		return currentState
	}
	return newState
}

func assignNumeralState(state string) int {
	switch state {
	case string(DatadogAgentStateFailed):
		return 1
	case string(DatadogAgentStateCanary):
		return 2
	case string(DatadogAgentStateUpdating):
		return 3
	case string(DatadogAgentStateProgressing):
		return 4
	case string(DatadogAgentStateRunning):
		return 5
	default:
		return 0
	}
}

// DatadogForwarderConditionType type use to represent a Datadog Metrics Forwarder condition.
type DatadogForwarderConditionType string

const (
	// DatadogMetricsActive forwarding metrics and events to Datadog is active.
	DatadogMetricsActive DatadogForwarderConditionType = "ActiveDatadogMetrics"
	// DatadogMetricsError cannot forward deployment metrics and events to Datadog.
	DatadogMetricsError DatadogForwarderConditionType = "DatadogMetricsError"
)
