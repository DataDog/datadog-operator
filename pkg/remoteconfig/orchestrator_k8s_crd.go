package remoteconfig

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	crdRcProduct = "ORCHESTRATOR_K8S_CRDS"
)

// CustomResourceDefinitionURLs defines model for CustomResourceDefinitionURLs.
type CustomResourceDefinitionURLs struct {
	Crds *[]string `json:"crds,omitempty"`
}

func (r *RemoteConfigUpdater) crdConfigUpdateCallback(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	ctx := context.Background()

	var configIDs []string
	for id := range updates {
		applyStatus(id, state.ApplyStatus{State: state.ApplyStateUnacknowledged, Error: ""})
		configIDs = append(configIDs, id)
	}

	mergedUpdate, err := r.parseCRDReceivedUpdates(updates, applyStatus)
	if err != nil {
		r.logger.Error(err, "Failed to merge updates")
		return
	}

	if err := r.getAndUpdateDatadogAgentWithRetry(ctx, mergedUpdate, r.crdUpdateInstanceStatus); err != nil {
		r.logger.Error(err, "Failed to update status")
		applyStatus(configIDs[len(configIDs)-1], state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		return
	}

	// Acknowledge that configs were received
	for _, id := range configIDs {
		applyStatus(id, state.ApplyStatus{State: state.ApplyStateAcknowledged, Error: ""})
	}

	r.logger.Info("Successfully applied configuration")

}

func (r *RemoteConfigUpdater) parseCRDReceivedUpdates(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) (DatadogAgentRemoteConfig, error) {
	// Unmarshal configs and config order
	crds := []string{}
	for _, c := range updates {
		if c.Metadata.Product == crdRcProduct {
			rcCRDs := CustomResourceDefinitionURLs{}
			err := json.Unmarshal(c.Config, &rcCRDs)
			if err != nil {
				return DatadogAgentRemoteConfig{}, err
			}
			if rcCRDs.Crds != nil {
				crds = append(crds, *rcCRDs.Crds...)
			}
		}
	}

	if len(crds) == 0 {
		r.logger.Info("No CRDs received")
		return DatadogAgentRemoteConfig{}, nil
	}

	// Merge configs
	var finalConfig DatadogAgentRemoteConfig

	// Cleanup CRD duplicates and add to final config
	crds = removeDuplicateStr(crds)
	if finalConfig.ClusterAgent == nil {
		finalConfig.ClusterAgent = &ClusterAgentFeaturesConfig{}
	}
	if finalConfig.ClusterAgent.CRDs == nil {
		finalConfig.ClusterAgent.CRDs = &CustomResourceDefinitionURLs{}
	}
	finalConfig.ClusterAgent.CRDs.Crds = &crds

	return finalConfig, nil
}

func (r *RemoteConfigUpdater) crdUpdateInstanceStatus(dda v2alpha1.DatadogAgent, cfg DatadogAgentRemoteConfig) error {

	newddaStatus := dda.Status.DeepCopy()
	if newddaStatus.RemoteConfigConfiguration == nil {
		newddaStatus.RemoteConfigConfiguration = &v2alpha1.RemoteConfigConfiguration{}
	}

	if newddaStatus.RemoteConfigConfiguration.Features == nil {
		newddaStatus.RemoteConfigConfiguration.Features = &v2alpha1.DatadogFeatures{}
	}

	if newddaStatus.RemoteConfigConfiguration.Features.OrchestratorExplorer == nil {
		newddaStatus.RemoteConfigConfiguration.Features.OrchestratorExplorer = &v2alpha1.OrchestratorExplorerFeatureConfig{}
	}

	// Orchestrator Explorer
	if cfg.ClusterAgent != nil && cfg.ClusterAgent.CRDs != nil && len(*cfg.ClusterAgent.CRDs.Crds) > 0 {
		newddaStatus.RemoteConfigConfiguration.Features.OrchestratorExplorer.CustomResources = append(newddaStatus.RemoteConfigConfiguration.Features.OrchestratorExplorer.CustomResources, *cfg.ClusterAgent.CRDs.Crds...)
		newddaStatus.RemoteConfigConfiguration.Features.OrchestratorExplorer.CustomResources = removeDuplicateStr(newddaStatus.RemoteConfigConfiguration.Features.OrchestratorExplorer.CustomResources)
	}

	if !apiequality.Semantic.DeepEqual(&dda.Status, newddaStatus) {
		ddaUpdate := dda.DeepCopy()
		ddaUpdate.Status = *newddaStatus
		if err := r.kubeClient.Status().Update(context.TODO(), ddaUpdate); err != nil {
			if apierrors.IsConflict(err) {
				r.logger.Info("unable to update DatadogAgent CRD status due to update conflict")
				return nil
			}
			r.logger.Error(err, "unable to update DatadogAgent status")
			return err
		}
	}

	return nil
}

func removeDuplicateStr(s []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, item := range s {
		if _, value := keys[item]; !value {
			keys[item] = true
			list = append(list, item)
		}
	}
	return list
}
