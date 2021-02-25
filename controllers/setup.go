// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package controllers

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/controllers/datadogagent"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
	"k8s.io/client-go/discovery"
)

// SetupOptions defines options for setting up controllers to ease testing
type SetupOptions struct {
	SupportExtendedDaemonset bool
	APIKey                   string
	AppKey                   string
	// AllowedContainerRegistries is a list of allowed container registries
	AllowedContainerRegistries []string
	// DisallowedFeatures is a list of allowed agent features
	DisallowedAgentFeatures []string
	// AgentHostStoragePath is a path allowed for use on the host
	AgentHostStoragePath string
}

// SetupControllers start all controllers (also used by e2e tests)
func SetupControllers(mgr manager.Manager, options SetupOptions) error {
	// Get some information about Kubernetes version
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("unable to get discovery client: %w", err)
	}

	versionInfo, err := discoveryClient.ServerVersion()
	if err != nil {
		return fmt.Errorf("unable to get APIServer version: %w", err)
	}

	disallowedFeatures := make([]datadogagent.AgentFeature, 0, len(options.DisallowedAgentFeatures))

	for _, f := range options.DisallowedAgentFeatures {
		feature := datadogagent.AgentFeature(f)
		if !feature.IsValid() {
			return fmt.Errorf("feature not valid: %s", f)
		}
		disallowedFeatures = append(disallowedFeatures, feature)
	}

	if err = (&DatadogAgentReconciler{
		Client:      mgr.GetClient(),
		VersionInfo: versionInfo,
		Log:         ctrl.Log.WithName("controllers").WithName("DatadogAgent"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("DatadogAgent"),
		Options: datadogagent.ReconcilerOptions{
			SupportExtendedDaemonset:   options.SupportExtendedDaemonset,
			AllowedContainerRegistries: options.AllowedContainerRegistries,
			DisallowedAgentFeatures:    disallowedFeatures,
			AgentHostStoragePath:       options.AgentHostStoragePath,
		},
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller DatadogAgent: %w", err)
	}

	ddClient, err := datadogclient.InitDatadogClient(options.APIKey, options.AppKey)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client: %w", err)
	}

	if err = (&DatadogMonitorReconciler{
		Client:      mgr.GetClient(),
		DDClient:    ddClient,
		VersionInfo: versionInfo,
		Log:         ctrl.Log.WithName("controllers").WithName("DatadogMonitor"),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("DatadogMonitor"),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller DatadogMonitor: %w", err)
	}

	return nil
}
