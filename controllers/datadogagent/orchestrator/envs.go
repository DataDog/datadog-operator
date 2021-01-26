package orchestrator

import (
	"encoding/json"
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
	DefaultID                                       = "id"
)

// EnvVars returns the orchestrator vars if the feature is enabled
func EnvVars(orc *datadoghqv1alpha1.OrchestratorExplorerConfig) ([]corev1.EnvVar, error) {
	var envVars []corev1.EnvVar
	envVars = append(envVars, corev1.EnvVar{
		Name:  DDOrchestratorExplorerEnabled,
		Value: strconv.FormatBool(datadoghqv1alpha1.BoolValue(orc.Enabled)),
	})
	// Scrubbing is defaulted to true beforehand in case it is nil
	envVars = append(envVars, corev1.EnvVar{
		Name:  DDOrchestratorExplorerContainerScrubbingEnabled,
		Value: strconv.FormatBool(datadoghqv1alpha1.BoolValue(orc.Scrubbing.Containers)),
	})

	if orc.DDUrl != nil {
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
	if len(orc.ExtraTags) > 0 {
		tags, err := json.Marshal(orc.ExtraTags)
		if err != nil {
			return nil, err
		}

		envVars = append(envVars, corev1.EnvVar{
			Name:  DDOrchestratorExplorerExtraTags,
			Value: string(tags),
		})
	}

	return envVars, nil
}
