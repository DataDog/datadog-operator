package datadogagent

import (
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// useDefaultDaemonset determines if we should use a legacy provider specific Daemonset for EKS and Openshift providers
func (r *Reconciler) useDefaultDaemonset(providerList map[string]struct{}) bool {
	if len(providerList) == 0 {
		return false
	}
	return kubernetes.ShouldUseDefaultDaemonset(providerList)
}

// generateNewStatusFromDDA generates a new status from a DDA status.
// If an existing DCA token is present, it is copied to the new status.
func generateNewStatusFromDDA(ddaStatus *datadoghqv2alpha1.DatadogAgentStatus) *datadoghqv2alpha1.DatadogAgentStatus {
	status := &datadoghqv2alpha1.DatadogAgentStatus{}
	if ddaStatus != nil {
		if ddaStatus.ClusterAgent != nil && ddaStatus.ClusterAgent.GeneratedToken != "" {
			status.ClusterAgent = &datadoghqv2alpha1.DeploymentStatus{
				GeneratedToken: ddaStatus.ClusterAgent.GeneratedToken,
			}
		}
		if ddaStatus.RemoteConfigConfiguration != nil {
			status.RemoteConfigConfiguration = ddaStatus.RemoteConfigConfiguration
		}
		status.Experiment = ddaStatus.Experiment.DeepCopy()
		status.CurrentRevision = ddaStatus.CurrentRevision
		status.CurrentRevisionObservedGeneration = ddaStatus.CurrentRevisionObservedGeneration
		status.CurrentRevisionObservedAnnotationsHash = ddaStatus.CurrentRevisionObservedAnnotationsHash
	}
	return status
}
