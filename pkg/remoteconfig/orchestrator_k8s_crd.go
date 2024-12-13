package remoteconfig

import (
	"context"
	"encoding/json"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
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

func (r *RemoteConfigUpdater) parseCRDReceivedUpdates(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) (OrchestratorK8sCRDRemoteConfig, error) {
	// Unmarshal configs and config order
	crds := []string{}
	for _, c := range updates {
		if c.Metadata.Product == state.ProductOrchestratorK8sCRDs {
			rcCRDs := CustomResourceDefinitionURLs{}
			err := json.Unmarshal(c.Config, &rcCRDs)
			if err != nil {
				return OrchestratorK8sCRDRemoteConfig{}, err
			}
			if rcCRDs.Crds != nil {
				crds = append(crds, *rcCRDs.Crds...)
			}
		}
	}

	if len(crds) == 0 {
		r.logger.Info("No CRDs received")
		return OrchestratorK8sCRDRemoteConfig{}, nil
	}

	// Merge configs
	var finalConfig OrchestratorK8sCRDRemoteConfig

	// Cleanup CRD duplicates and add to final config
	crds = removeDuplicateStr(crds)

	if finalConfig.CRDs == nil {
		finalConfig.CRDs = &CustomResourceDefinitionURLs{}
	}
	finalConfig.CRDs.Crds = &crds

	return finalConfig, nil
}

func (r *RemoteConfigUpdater) crdUpdateInstanceStatus(dda v2alpha1.DatadogAgent, config DatadogProductRemoteConfig) error {
	cfg, ok := config.(OrchestratorK8sCRDRemoteConfig)
	if !ok {
		return fmt.Errorf("invalid config type: %T", config)
	}

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
	newddaStatus.RemoteConfigConfiguration.Features.OrchestratorExplorer.CustomResources = []string{}
	if cfg.CRDs != nil {
		// Overwrite custom resources by the new ones
		if cfg.CRDs.Crds != nil {
			newddaStatus.RemoteConfigConfiguration.Features.OrchestratorExplorer.CustomResources = *cfg.CRDs.Crds
		}
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
