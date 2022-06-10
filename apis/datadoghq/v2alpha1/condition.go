package v2alpha1

import (
	"fmt"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogAgentState type representing the deployment state of the different Agent components.
type DatadogAgentState string

const (
	// DatadogAgentStateProgressing the deployment is running properly.
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
		UpdateDatadogAgentStatusCondition(status.Conditions[idConditionComplete], now, t, conditionStatus, reason, message)
	} else if conditionStatus == metav1.ConditionTrue || writeFalseIfNotExist {
		// Only add if the condition is True
		status.Conditions = append(status.Conditions, NewDatadogAgentStatusCondition(t, conditionStatus, now, reason, message))
	}
}

// UpdateDatadogAgentStatusCondition used to update a specific string
func UpdateDatadogAgentStatusCondition(condition metav1.Condition, now metav1.Time, t string, conditionStatus metav1.ConditionStatus, reason, message string) metav1.Condition {
	if condition.Status != conditionStatus {
		condition.LastTransitionTime = now
		condition.Status = conditionStatus
	}
	condition.LastTransitionTime = now
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

// NewDatadogAgentStatusCondition returns new DatadogAgentCondition instance
func NewDatadogAgentStatusCondition(conditionType string, conditionStatus metav1.ConditionStatus, now metav1.Time, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
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

	if hash, ok := dep.Annotations[apicommon.MD5AgentDeploymentAnnotationKey]; ok {
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
