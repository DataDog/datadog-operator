package orchestrator

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
)

// Datadog orchestrator related env var names
const (
	DDOrchestratorExplorerEnabled                   = "DD_ORCHESTRATOR_EXPLORER_ENABLED"
	DDOrchestratorExplorerExtraTags                 = "DD_ORCHESTRATOR_EXPLORER_EXTRA_TAGS"
	DDOrchestratorExplorerDDUrl                     = "DD_ORCHESTRATOR_EXPLORER_DD_URL"
	DDOrchestratorExplorerAdditionalEndpoints       = "DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS"
	DDOrchestratorExplorerContainerScrubbingEnabled = "DD_ORCHESTRATOR_EXPLORER_CONTAINER_SCRUBBING_ENABLED"
	DDOrchestratorClusterID                         = "DD_ORCHESTRATOR_CLUSTER_ID"
	DefaultID                                       = "id"
)

// EnvVars returns the orchestrator vars if the feature is enabled
func EnvVars(orc *datadoghqv1alpha1.OrchestratorExplorerConfig) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	envVars = append(envVars, corev1.EnvVar{
		Name:  DDOrchestratorExplorerEnabled,
		Value: strconv.FormatBool(datadoghqv1alpha1.BoolValue(orc.Enabled)),
	})
	// ContainerScrubbingEnabled is defaulted to true beforehand in case it is nil
	envVars = append(envVars, corev1.EnvVar{
		Name:  DDOrchestratorExplorerContainerScrubbingEnabled,
		Value: strconv.FormatBool(datadoghqv1alpha1.BoolValue(orc.ContainerScrubbingEnabled)),
	})

	if orc.AdditionalEndpoints != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  DDOrchestratorExplorerDDUrl,
			Value: *orc.DDUrl,
		})
	}
	if orc.AdditionalEndpoints != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  DDOrchestratorExplorerAdditionalEndpoints,
			Value: *orc.AdditionalEndpoints,
		})
	}
	if orc.ExtraTags != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  DDOrchestratorExplorerExtraTags,
			Value: *orc.ExtraTags,
		})
	}
	return envVars
}

// ClusterID returns the ClusterID for the orchestrator. The ClusterAgent creates the ID as a configmap while the agent retrieves it from there.
func ClusterID() corev1.EnvVar {
	authTokenValue := &corev1.EnvVarSource{
		ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: datadoghqv1alpha1.DatadogClusterIDResourceName},
			Key:                  DefaultID,
		},
	}

	return corev1.EnvVar{
		Name:      DDOrchestratorClusterID,
		ValueFrom: authTokenValue,
	}
}
