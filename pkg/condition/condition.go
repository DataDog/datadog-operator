// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package condition

import (
	"cmp"
	"fmt"
	"slices"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
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

// UpdateDatadogAgentStatusConditions used to update a specific string in conditions
func UpdateDatadogAgentStatusConditions(status *v2alpha1.DatadogAgentStatus, now metav1.Time, t string, conditionStatus metav1.ConditionStatus, reason, message string, writeFalseIfNotExist bool) {
	idConditionComplete := getIndexForConditionType(status, t)
	if idConditionComplete >= 0 {
		updateDatadogAgentStatusCondition(&status.Conditions[idConditionComplete], now, conditionStatus, reason, message)
	} else if conditionStatus == metav1.ConditionTrue || writeFalseIfNotExist {
		// Only add if the condition is True
		status.Conditions = append(status.Conditions, newDatadogAgentStatusCondition(t, conditionStatus, now, reason, message))
	}
}

// UpdateDatadogAgentInternalStatusConditions used to update a specific string in conditions
func UpdateDatadogAgentInternalStatusConditions(status *v1alpha1.DatadogAgentInternalStatus, now metav1.Time, t string, conditionStatus metav1.ConditionStatus, reason, message string, writeFalseIfNotExist bool) {
	idConditionComplete := getIndexForConditionTypeDDAI(status, t)
	if idConditionComplete >= 0 {
		updateDatadogAgentStatusCondition(&status.Conditions[idConditionComplete], now, conditionStatus, reason, message)
	} else if conditionStatus == metav1.ConditionTrue || writeFalseIfNotExist {
		// Only add if the condition is True
		status.Conditions = append(status.Conditions, newDatadogAgentStatusCondition(t, conditionStatus, now, reason, message))
	}
}

// updateDatadogAgentStatusCondition used to update a specific string
func updateDatadogAgentStatusCondition(condition *metav1.Condition, now metav1.Time, conditionStatus metav1.ConditionStatus, reason, message string) *metav1.Condition {
	if condition.Status != conditionStatus {
		condition.LastTransitionTime = now
		condition.Status = conditionStatus
	}
	condition.Message = message
	condition.Reason = reason

	return condition
}

// DeleteDatadogAgentStatusCondition is used to delete a condition
func DeleteDatadogAgentStatusCondition(status *v2alpha1.DatadogAgentStatus, conditionType string) {
	idConditionComplete := getIndexForConditionType(status, conditionType)
	if idConditionComplete >= 0 {
		status.Conditions = append(status.Conditions[:idConditionComplete], status.Conditions[idConditionComplete+1:]...)
	}
}

func DeleteDatadogAgentInternalStatusCondition(status *v1alpha1.DatadogAgentInternalStatus, conditionType string) {
	idConditionComplete := getIndexForConditionTypeDDAI(status, conditionType)
	if idConditionComplete >= 0 {
		status.Conditions = append(status.Conditions[:idConditionComplete], status.Conditions[idConditionComplete+1:]...)
	}
}

// newDatadogAgentStatusCondition returns new metav1.Condition instance
func newDatadogAgentStatusCondition(conditionType string, conditionStatus metav1.ConditionStatus, now metav1.Time, reason, message string) metav1.Condition {
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

func getIndexForConditionType(status *v2alpha1.DatadogAgentStatus, t string) int {
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

func getIndexForConditionTypeDDAI(status *v1alpha1.DatadogAgentInternalStatus, t string) int {
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
func UpdateDeploymentStatus(dep *appsv1.Deployment, depStatus *v2alpha1.DeploymentStatus, updateTime *metav1.Time) *v2alpha1.DeploymentStatus {
	if depStatus == nil {
		depStatus = &v2alpha1.DeploymentStatus{}
	}
	if dep == nil {
		depStatus.State = string(DatadogAgentStateFailed)
		depStatus.Status = string(DatadogAgentStateFailed)
		return depStatus
	}

	if hash, ok := dep.Annotations[constants.MD5AgentDeploymentAnnotationKey]; ok {
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
func UpdateDaemonSetStatus(dsName string, ds *appsv1.DaemonSet, dsStatus []*v2alpha1.DaemonSetStatus, updateTime *metav1.Time) []*v2alpha1.DaemonSetStatus {
	if dsStatus == nil {
		dsStatus = []*v2alpha1.DaemonSetStatus{}
	}

	var newStatus v2alpha1.DaemonSetStatus
	if ds == nil {
		newStatus = v2alpha1.DaemonSetStatus{
			State:         string(DatadogAgentStateFailed),
			Status:        string(DatadogAgentStateFailed),
			DaemonsetName: dsName,
		}
	} else {
		newStatus = v2alpha1.DaemonSetStatus{
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
		if hash, ok := ds.Annotations[constants.MD5AgentDeploymentAnnotationKey]; ok {
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
	}

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

// UpdateDaemonSetStatusDDAI updates a daemonset's DaemonSetStatus
func UpdateDaemonSetStatusDDAI(dsName string, ds *appsv1.DaemonSet, dsStatus *v2alpha1.DaemonSetStatus, updateTime *metav1.Time) *v2alpha1.DaemonSetStatus {
	if dsStatus == nil {
		dsStatus = &v2alpha1.DaemonSetStatus{}
	}

	if ds == nil {
		dsStatus.State = string(DatadogAgentStateFailed)
		dsStatus.Status = string(DatadogAgentStateFailed)
		dsStatus.DaemonsetName = dsName
	} else {
		dsStatus.Desired = ds.Status.DesiredNumberScheduled
		dsStatus.Current = ds.Status.CurrentNumberScheduled
		dsStatus.Ready = ds.Status.NumberReady
		dsStatus.Available = ds.Status.NumberAvailable
		dsStatus.UpToDate = ds.Status.UpdatedNumberScheduled
		dsStatus.DaemonsetName = ds.ObjectMeta.Name
		if updateTime != nil {
			dsStatus.LastUpdate = updateTime
		}
		if hash, ok := ds.Annotations[constants.MD5AgentDeploymentAnnotationKey]; ok {
			dsStatus.CurrentHash = hash
		}

		var deploymentState DatadogAgentState
		switch {
		case dsStatus.UpToDate != dsStatus.Desired:
			deploymentState = DatadogAgentStateUpdating
		case dsStatus.Ready == 0 && dsStatus.Desired != 0:
			deploymentState = DatadogAgentStateProgressing
		default:
			deploymentState = DatadogAgentStateRunning
		}

		dsStatus.State = fmt.Sprintf("%v", deploymentState)
		dsStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", deploymentState, dsStatus.Desired, dsStatus.Ready, dsStatus.UpToDate)
	}

	return dsStatus
}

// UpdateExtendedDaemonSetStatus updates an ExtendedDaemonSet's DaemonSetStatus
func UpdateExtendedDaemonSetStatus(eds *edsdatadoghqv1alpha1.ExtendedDaemonSet, dsStatus []*v2alpha1.DaemonSetStatus, updateTime *metav1.Time) []*v2alpha1.DaemonSetStatus {
	if dsStatus == nil {
		dsStatus = []*v2alpha1.DaemonSetStatus{}
	}

	newStatus := v2alpha1.DaemonSetStatus{
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
	if hash, ok := eds.Annotations[constants.MD5AgentDeploymentAnnotationKey]; ok {
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

// UpdateExtendedDaemonSetStatusDDAI updates an ExtendedDaemonSet's DaemonSetStatus
func UpdateExtendedDaemonSetStatusDDAI(eds *edsdatadoghqv1alpha1.ExtendedDaemonSet, dsStatus *v2alpha1.DaemonSetStatus, updateTime *metav1.Time) *v2alpha1.DaemonSetStatus {
	if dsStatus == nil {
		dsStatus = &v2alpha1.DaemonSetStatus{}
	}

	dsStatus.Desired = eds.Status.Desired
	dsStatus.Current = eds.Status.Current
	dsStatus.Ready = eds.Status.Ready
	dsStatus.Available = eds.Status.Available
	dsStatus.UpToDate = eds.Status.UpToDate
	dsStatus.DaemonsetName = eds.ObjectMeta.Name

	if updateTime != nil {
		dsStatus.LastUpdate = updateTime
	}
	if hash, ok := eds.Annotations[constants.MD5AgentDeploymentAnnotationKey]; ok {
		dsStatus.CurrentHash = hash
	}

	var deploymentState DatadogAgentState
	switch {
	case eds.Status.Canary != nil:
		deploymentState = DatadogAgentStateCanary
	case dsStatus.UpToDate != dsStatus.Desired:
		deploymentState = DatadogAgentStateUpdating
	case dsStatus.Ready == 0 && dsStatus.Desired != 0:
		deploymentState = DatadogAgentStateProgressing
	default:
		deploymentState = DatadogAgentStateRunning
	}

	dsStatus.State = fmt.Sprintf("%v", deploymentState)
	dsStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", deploymentState, dsStatus.Desired, dsStatus.Ready, dsStatus.UpToDate)

	return dsStatus
}

// UpdateCombinedDaemonSetStatus combines the status of multiple DaemonSetStatus
func UpdateCombinedDaemonSetStatus(dsStatus []*v2alpha1.DaemonSetStatus) *v2alpha1.DaemonSetStatus {
	combinedStatus := v2alpha1.DaemonSetStatus{}
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

func CombineDaemonSetStatus(dsStatus *v2alpha1.DaemonSetStatus, ddaiStatus *v2alpha1.DaemonSetStatus) *v2alpha1.DaemonSetStatus {
	if ddaiStatus == nil {
		return dsStatus
	}
	if dsStatus == nil {
		dsStatus = &v2alpha1.DaemonSetStatus{}
	}
	dsStatus.Desired += ddaiStatus.Desired
	dsStatus.Current += ddaiStatus.Current
	dsStatus.Ready += ddaiStatus.Ready
	dsStatus.Available += ddaiStatus.Available
	dsStatus.UpToDate += ddaiStatus.UpToDate
	if dsStatus.LastUpdate == nil {
		dsStatus.LastUpdate = ddaiStatus.LastUpdate
	}
	if dsStatus.LastUpdate.Before(ddaiStatus.LastUpdate) {
		dsStatus.LastUpdate = ddaiStatus.LastUpdate
	}
	dsStatus.State = getCombinedState(dsStatus.State, ddaiStatus.State)
	dsStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", dsStatus.State, dsStatus.Desired, dsStatus.Ready, dsStatus.UpToDate)
	return dsStatus
}

func CombineDeploymentStatus(deploymentStatus *v2alpha1.DeploymentStatus, ddaiStatus *v2alpha1.DeploymentStatus) *v2alpha1.DeploymentStatus {
	if ddaiStatus == nil {
		return deploymentStatus
	}
	if deploymentStatus == nil {
		deploymentStatus = &v2alpha1.DeploymentStatus{}
	}
	deploymentStatus.Replicas += ddaiStatus.Replicas
	deploymentStatus.UpdatedReplicas += ddaiStatus.UpdatedReplicas
	deploymentStatus.ReadyReplicas += ddaiStatus.ReadyReplicas
	deploymentStatus.AvailableReplicas += ddaiStatus.AvailableReplicas
	deploymentStatus.UnavailableReplicas += ddaiStatus.UnavailableReplicas
	// TODO: Set DCA token in dependencies
	if deploymentStatus.LastUpdate == nil {
		deploymentStatus.LastUpdate = ddaiStatus.LastUpdate
	}
	if deploymentStatus.LastUpdate.Before(ddaiStatus.LastUpdate) {
		deploymentStatus.LastUpdate = ddaiStatus.LastUpdate
	}
	deploymentStatus.CurrentHash = ddaiStatus.CurrentHash
	deploymentStatus.Status = ddaiStatus.Status
	deploymentStatus.State = ddaiStatus.State
	deploymentStatus.DeploymentName = ddaiStatus.DeploymentName
	return deploymentStatus
}

func IsEqualConditions(current []metav1.Condition, newCond []metav1.Condition) bool {
	if len(current) != len(newCond) {
		return false
	}

	// Compare order-insensitively. The CRD uses listMapKey=type so types are unique.
	ac := append([]metav1.Condition(nil), current...)
	bc := append([]metav1.Condition(nil), newCond...)
	slices.SortFunc(ac, func(a, b metav1.Condition) int { return cmp.Compare(a.Type, b.Type) })
	slices.SortFunc(bc, func(a, b metav1.Condition) int { return cmp.Compare(a.Type, b.Type) })
	for i := range ac {
		if !IsEqualCondition(&ac[i], &bc[i]) {
			return false
		}
	}
	return true
}

func IsEqualCondition(current *metav1.Condition, newCond *metav1.Condition) bool {
	if current.Type != newCond.Type ||
		current.Status != newCond.Status ||
		current.Reason != newCond.Reason ||
		current.Message != newCond.Message {
		return false
	}
	return true
}
