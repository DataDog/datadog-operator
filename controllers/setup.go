// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controllers

import (
	"errors"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/controllers/datadogagent"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
)

const (
	agentControllerName   = "DatadogAgent"
	monitorControllerName = "DatadogMonitor"
)

// SetupOptions defines options for setting up controllers to ease testing
type SetupOptions struct {
	SupportExtendedDaemonset bool
	Creds                    config.Creds
	HaveCreds                bool
	DatadogMonitorEnabled    bool
}

type starterFunc func(logr.Logger, manager.Manager, *version.Info, SetupOptions) error

var controllerStarters = map[string]starterFunc{
	agentControllerName:   startDatadogAgent,
	monitorControllerName: startDatadogMonitor,
}

// SetupControllers starts all controllers (also used by e2e tests)
func SetupControllers(logger logr.Logger, mgr manager.Manager, options SetupOptions) error {
	// Get some information about Kubernetes version
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("unable to get discovery client: %w", err)
	}

	versionInfo, err := discoveryClient.ServerVersion()
	if err != nil {
		return fmt.Errorf("unable to get APIServer version: %w", err)
	}

	for controller, starter := range controllerStarters {
		if err := starter(logger, mgr, versionInfo, options); err != nil {
			logger.Error(err, "Couldn't start controller", "controller", controller)
		}
	}

	return nil
}

func startDatadogAgent(logger logr.Logger, mgr manager.Manager, vInfo *version.Info, options SetupOptions) error {
	return (&DatadogAgentReconciler{
		Client:      mgr.GetClient(),
		VersionInfo: vInfo,
		Log:         ctrl.Log.WithName("controllers").WithName(agentControllerName),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor(agentControllerName),
		Options: datadogagent.ReconcilerOptions{
			SupportExtendedDaemonset: options.SupportExtendedDaemonset,
		},
	}).SetupWithManager(mgr)
}

func startDatadogMonitor(logger logr.Logger, mgr manager.Manager, vInfo *version.Info, options SetupOptions) error {
	if !options.DatadogMonitorEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", monitorControllerName)
		return nil
	}

	if !options.HaveCreds {
		return errors.New("credentials not provided")
	}

	ddClient, err := datadogclient.InitDatadogClient(options.Creds)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client: %w", err)
	}

	return (&DatadogMonitorReconciler{
		Client:      mgr.GetClient(),
		DDClient:    ddClient,
		VersionInfo: vInfo,
		Log:         ctrl.Log.WithName("controllers").WithName(monitorControllerName),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor(monitorControllerName),
	}).SetupWithManager(mgr)
}
