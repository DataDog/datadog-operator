// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/go-logr/logr"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

const (
	methodStartDatadogAgentExperiment   = "operator/start_datadogagent_experiment"
	methodStopDatadogAgentExperiment    = "operator/stop_datadogagent_experiment"
	methodPromoteDatadogAgentExperiment = "operator/promote_datadogagent_experiment"
)

var _ manager.Runnable = &Daemon{}
var _ manager.LeaderElectionRunnable = &Daemon{}

// Daemon subscribes to fleet-specific RC products (installer configs and tasks)
// and runs after leader election as a controller-runtime Runnable.
type Daemon struct {
	logger     logr.Logger
	rcClient   remoteconfig.RCClient
	kubeClient kubeclient.Client
	mu       sync.RWMutex
	configs  map[string]installerConfig // keyed by config ID; replaced on each RC update
}

// NewDaemon creates a new Fleet Daemon.
func NewDaemon(logger logr.Logger, rcClient remoteconfig.RCClient, kubeClient kubeclient.Client) *Daemon {
	return &Daemon{
		logger:     logger,
		rcClient:   rcClient,
		kubeClient: kubeClient,
		configs:  make(map[string]installerConfig),
	}
}

// Start implements manager.Runnable. It subscribes to fleet RC products and
// blocks until ctx is cancelled.
func (d *Daemon) Start(ctx context.Context) error {
	d.logger.Info("Starting Fleet daemon")

	d.rcClient.Subscribe(state.ProductUpdaterAgent, handleInstallerConfigUpdate(d.handleConfigs))
	d.rcClient.Subscribe(state.ProductUpdaterTask, handleUpdaterTaskUpdate(d.handleRemoteAPIRequest))

	<-ctx.Done()
	d.logger.Info("Stopping Fleet daemon")
	return nil
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
// The daemon only runs on the elected leader.
func (d *Daemon) NeedLeaderElection() bool {
	return true
}

// handleConfigs replaces the stored installer configs with the latest RC update.
// Configs are indexed by their ID so they can be retrieved by task handlers.
func (d *Daemon) handleConfigs(configs map[string]installerConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	newConfigs := make(map[string]installerConfig, len(configs))
	for _, cfg := range configs {
		d.logger.Info("Received installer config", "id", cfg.ID, "file_operations", len(cfg.FileOperations))
		newConfigs[cfg.ID] = cfg
	}
	d.configs = newConfigs
	return nil
}

// getConfig returns the installer config with the given ID.
func (d *Daemon) getConfig(id string) (installerConfig, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	cfg, ok := d.configs[id]
	if !ok {
		return installerConfig{}, fmt.Errorf("config %s not found", id)
	}
	return cfg, nil
}

// handleRemoteAPIRequest dispatches the incoming task to the appropriate handler.
func (d *Daemon) handleRemoteAPIRequest(req remoteAPIRequest) error {
	d.logger.Info("Received remote API request", "id", req.ID, "package", req.Package, "method", req.Method)
	switch req.Method {
	case methodStartDatadogAgentExperiment:
		return d.startDatadogAgentExperiment(req)
	case methodStopDatadogAgentExperiment:
		return d.stopDatadogAgentExperiment(req)
	case methodPromoteDatadogAgentExperiment:
		return d.promoteDatadogAgentExperiment(req)
	default:
		return fmt.Errorf("unknown method: %s", req.Method)
	}
}



