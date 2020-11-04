package orchestrator

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
)

// EnvVars returns the orchestrator vars if the feature is enabled
func EnvVars(spec *datadoghqv1alpha1.DatadogAgentSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	orc := spec.DatadogFeatures.OrchestratorExplorer
	envVars = append(envVars, corev1.EnvVar{
		Name:  datadoghqv1alpha1.DDOrchestratorExplorerEnabled,
		Value: strconv.FormatBool(true),
	})
	envVars = append(envVars, corev1.EnvVar{
		Name:  datadoghqv1alpha1.DDOrchestratorExplorerContainerScrubbingEnabled,
		Value: strconv.FormatBool(datadoghqv1alpha1.BoolValue(orc.ContainerScrubbingEnabled)),
	})
	if orc.AdditionalEndpoints != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDOrchestratorExplorerDDUrl,
			Value: *orc.DDUrl,
		})
	}
	if orc.AdditionalEndpoints != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDOrchestratorExplorerAdditionalEndpoints,
			Value: *orc.AdditionalEndpoints,
		})
	}
	if orc.ExtraTags != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDOrchestratorExplorerExtraTags,
			Value: *orc.ExtraTags,
		})
	}
	return envVars
}
