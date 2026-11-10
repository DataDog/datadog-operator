// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/flare/remoteflare"
)

// newFlareRunner returns the runner of the flares Fleet Automation requests
// through Remote Configuration. It reads the cluster with the manager's API
// reader, so a flare doesn't start informers.
func newFlareRunner(opts *options, restConfig *rest.Config, mgr manager.Manager, creds remoteflare.CredentialsGetter, isLeader func() bool, rcState func() any) (*remoteflare.Runner, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create the Kubernetes clientset for flares: %w", err)
	}
	return remoteflare.NewRunner(remoteflare.Config{
		Reader:                mgr.GetAPIReader(),
		Clientset:             clientset,
		Credentials:           creds,
		MetricsAddr:           opts.metricsAddr,
		SecureMetrics:         opts.secureMetrics,
		HealthProbeAddr:       healthProbeBindAddress,
		Features:              opts.flareFeatures(),
		LeaderElectionEnabled: opts.enableLeaderElection,
		LeaderElectionID:      leaderElectionID,
		IsLeader:              isLeader,
		RemoteConfigState:     rcState,
	}, ctrl.Log.WithName("flare")), nil
}

// flareFeatures returns the controllers and features of the Operator and
// whether they are enabled.
func (opts *options) flareFeatures() map[string]bool {
	return map[string]bool{
		"datadogAgent":              opts.datadogAgentEnabled,
		"datadogAgentProfile":       opts.datadogAgentProfileEnabled,
		"datadogMonitor":            opts.datadogMonitorEnabled,
		"datadogSLO":                opts.datadogSLOEnabled,
		"datadogDashboard":          opts.datadogDashboardEnabled,
		"datadogGenericResource":    opts.datadogGenericResourceEnabled,
		"datadogBYOCCluster":        opts.datadogBYOCClusterEnabled,
		"datadogCSIDriver":          opts.datadogCSIDriverEnabled,
		"untaintController":         opts.untaintControllerEnabled,
		"componentHealth":           opts.componentHealthEnabled,
		"componentHealthIntake":     opts.componentHealthIntakeEnabled,
		"introspection":             opts.introspectionEnabled,
		"remoteConfig":              opts.remoteConfigEnabled,
		"remoteUpdates":             opts.remoteUpdatesEnabled,
		"managedAgentInstallation":  opts.managedAgentInstallationEnabled,
		"operatorMetrics":           opts.operatorMetricsEnabled,
		"supportCilium":             opts.supportCilium,
		"createControllerRevisions": opts.createControllerRevisions,
		"rolloutOnConfigMapChange":  opts.rolloutOnConfigMapChangeEnabled,
		"defaultDataPlaneLinux":     opts.defaultDataPlaneLinuxEnabled,
		"leaderElection":            opts.enableLeaderElection,
		"secureMetrics":             opts.secureMetrics,
		"profiling":                 opts.profilingEnabled,
		"tracing":                   opts.tracingEnabled,
		"pprof":                     opts.pprofActive,
	}
}
