// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

var _ manager.Runnable = &Daemon{}
var _ manager.LeaderElectionRunnable = &Daemon{}

// Daemon is a placeholder for the Fleet daemon. It runs after leader election
// as a controller-runtime Runnable and holds a reference to the RC updater
// for future use.
type Daemon struct {
	logger    logr.Logger
	rcUpdater *remoteconfig.RemoteConfigUpdater
}

// NewDaemon creates a new Fleet Daemon.
func NewDaemon(logger logr.Logger, rcUpdater *remoteconfig.RemoteConfigUpdater) *Daemon {
	return &Daemon{
		logger:    logger,
		rcUpdater: rcUpdater,
	}
}

// Start implements manager.Runnable. It blocks until ctx is cancelled.
func (d *Daemon) Start(ctx context.Context) error {
	d.logger.Info("Starting Fleet daemon")
	<-ctx.Done()
	d.logger.Info("Stopping Fleet daemon")
	return nil
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
// The daemon only runs on the elected leader.
func (d *Daemon) NeedLeaderElection() bool {
	return true
}
