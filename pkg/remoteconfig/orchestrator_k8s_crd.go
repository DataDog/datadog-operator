package remoteconfig

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
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

	r.logger.Info("Received CRD updates: %v\n", updates)

	var configIDs []string
	for id := range updates {
		applyStatus(id, state.ApplyStatus{State: state.ApplyStateUnacknowledged, Error: ""})
		configIDs = append(configIDs, id)
	}
	r.logger.Info("Applied status")

	mergedUpdate, err := r.parseCRDReceivedUpdates(updates, applyStatus)
	if err != nil {
		r.logger.Error(err, "Failed to merge updates")
		return
	}

	r.logger.Info("Merge updates")

	dda, err := r.getDatadogAgentInstance(ctx)
	if err != nil {
		r.logger.Error(err, "Failed to get updatable agents")
		return
	}

	if err := r.crdUpdateInstanceStatus(dda, mergedUpdate); err != nil {
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
			crds = append(crds, *rcCRDs.Crds...)
		}
	}

	// Merge configs
	var finalConfig DatadogAgentRemoteConfig

	// Cleanup CRD duplicates and add to final config
	crds = removeDuplicateStr(crds)
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
				r.logger.Info("unable to update DatadogAgent status due to update conflict")
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
