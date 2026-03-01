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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

var _ manager.Runnable = &Daemon{}
var _ manager.LeaderElectionRunnable = &Daemon{}

// Daemon subscribes to fleet-specific RC products (installer configs and tasks)
// and runs after leader election as a controller-runtime Runnable.
type Daemon struct {
	logger   logr.Logger
	rcClient remoteconfig.RCClient
	mu       sync.RWMutex
	// configs stores the latest received installer configs keyed by RC config path.
	configs map[string]installerConfig
}

// NewDaemon creates a new Fleet Daemon.
func NewDaemon(logger logr.Logger, rcClient remoteconfig.RCClient) *Daemon {
	return &Daemon{
		logger:   logger,
		rcClient: rcClient,
		configs:  make(map[string]installerConfig),
	}
}

// Start implements manager.Runnable. It subscribes to fleet RC products and
// blocks until ctx is cancelled.
func (d *Daemon) Start(ctx context.Context) error {
	d.logger.Info("Starting Fleet daemon")

	if d.rcClient == nil {
		return fmt.Errorf("fleet daemon: RC client is not ready")
	}

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

// handleConfigs stores the received installer configs and logs them.
func (d *Daemon) handleConfigs(configs map[string]installerConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for path, cfg := range configs {
		d.logger.Info("Received installer config", "path", path, "id", cfg.ID, "file_operations", len(cfg.FileOperations))
		d.configs[path] = cfg
	}
	return nil
}

// handleRemoteAPIRequest logs the incoming task request. No action is taken.
func (d *Daemon) handleRemoteAPIRequest(req remoteAPIRequest) error {
	d.logger.Info("Received remote API request", "id", req.ID, "package", req.Package, "method", req.Method)
	return nil
}
