package fleet

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewDaemonForTesting creates a Daemon with a K8s client but no RC client.
// Used by the fleet-test CLI to exercise experiment functions directly.
func NewDaemonForTesting(logger logr.Logger, kubeClient kubeclient.Client) *Daemon {
	return &Daemon{
		logger:     logger,
		kubeClient: kubeClient,
		configs:    make(map[string]installerConfig),
	}
}

// InjectConfig adds a test config that patches the specified DDA.
func (d *Daemon) InjectConfig(configID, namespace, name string, patch json.RawMessage) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.configs[configID] = installerConfig{
		ID: configID,
		Operations: []fleetManagementOperation{
			{
				Operation:        operationUpdate,
				GroupVersionKind: schema.GroupVersionKind{Group: "datadoghq.com", Version: "v2alpha1", Kind: "DatadogAgent"},
				NamespacedName:   types.NamespacedName{Namespace: namespace, Name: name},
				Config:           patch,
			},
		},
	}
}

// StartExperiment starts an experiment via the daemon.
func (d *Daemon) StartExperiment(ctx context.Context, experimentID, configID string) error {
	return d.startDatadogAgentExperiment(remoteAPIRequest{
		ID:            experimentID,
		Method:        methodStartDatadogAgentExperiment,
		ExpectedState: expectedState{ExperimentConfig: configID},
	})
}

// StopExperiment stops an experiment via the daemon.
func (d *Daemon) StopExperiment(ctx context.Context, experimentID string) error {
	return d.stopDatadogAgentExperiment(remoteAPIRequest{
		ID:     experimentID,
		Method: methodStopDatadogAgentExperiment,
	})
}

// PromoteExperiment promotes an experiment via the daemon.
func (d *Daemon) PromoteExperiment(ctx context.Context, experimentID string) error {
	return d.promoteDatadogAgentExperiment(remoteAPIRequest{
		ID:     experimentID,
		Method: methodPromoteDatadogAgentExperiment,
	})
}
