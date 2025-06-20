// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	agentControllerName           = "DatadogAgent"
	agentInternalControllerName   = "DatadogAgentInternal"
	monitorControllerName         = "DatadogMonitor"
	sloControllerName             = "DatadogSLO"
	profileControllerName         = "DatadogAgentProfile"
	dashboardControllerName       = "DatadogDashboard"
	genericResourceControllerName = "DatadogGenericResource"
)

// SetupOptions defines options for setting up controllers to ease testing
type SetupOptions struct {
	SupportExtendedDaemonset      ExtendedDaemonsetOptions
	SupportCilium                 bool
	Creds                         config.Creds
	DatadogAgentEnabled           bool
	DatadogAgentInternalEnabled   bool
	DatadogMonitorEnabled         bool
	DatadogSLOEnabled             bool
	OperatorMetricsEnabled        bool
	V2APIEnabled                  bool
	IntrospectionEnabled          bool
	DatadogAgentProfileEnabled    bool
	OtelAgentEnabled              bool
	DatadogDashboardEnabled       bool
	DatadogGenericResourceEnabled bool
}

// ExtendedDaemonsetOptions defines ExtendedDaemonset options
type ExtendedDaemonsetOptions struct {
	Enabled                   bool
	MaxPodUnavailable         string
	MaxPodSchedulerFailure    string
	SlowStartAdditiveIncrease string

	CanaryDuration                      time.Duration
	CanaryReplicas                      string
	CanaryAutoPauseEnabled              bool
	CanaryAutoPauseMaxRestarts          int
	CanaryAutoFailEnabled               bool
	CanaryAutoFailMaxRestarts           int
	CanaryAutoPauseMaxSlowStartDuration time.Duration
}

type starterFunc func(logr.Logger, manager.Manager, kubernetes.PlatformInfo, SetupOptions, datadog.MetricsForwardersManager) error

var controllerStarters = map[string]starterFunc{
	agentControllerName:           startDatadogAgent,
	agentInternalControllerName:   startDatadogAgentInternal,
	monitorControllerName:         startDatadogMonitor,
	sloControllerName:             startDatadogSLO,
	profileControllerName:         startDatadogAgentProfiles,
	dashboardControllerName:       startDatadogDashboard,
	genericResourceControllerName: startDatadogGenericResource,
}

// SetupControllers starts all controllers (also used by e2e tests)
func SetupControllers(logger logr.Logger, mgr manager.Manager, platformInfo kubernetes.PlatformInfo, options SetupOptions) error {

	var metricForwardersMgr datadog.MetricsForwardersManager
	if options.OperatorMetricsEnabled {
		metricForwardersMgr = datadog.NewForwardersManager(mgr.GetClient(), &platformInfo)
	}

	for controller, starter := range controllerStarters {
		if err := starter(logger, mgr, platformInfo, options, metricForwardersMgr); err != nil {
			logger.Error(err, "Couldn't start controller", "controller", controller)
		}
	}

	return nil
}

func startDatadogAgent(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogAgentEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", agentControllerName)

		return nil
	}

	return (&DatadogAgentReconciler{
		Client:       mgr.GetClient(),
		PlatformInfo: pInfo,
		Log:          ctrl.Log.WithName("controllers").WithName(agentControllerName),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor(agentControllerName),
		Options: datadogagent.ReconcilerOptions{
			ExtendedDaemonsetOptions: componentagent.ExtendedDaemonsetOptions{
				Enabled:                             options.SupportExtendedDaemonset.Enabled,
				MaxPodUnavailable:                   options.SupportExtendedDaemonset.MaxPodUnavailable,
				MaxPodSchedulerFailure:              options.SupportExtendedDaemonset.MaxPodSchedulerFailure,
				SlowStartAdditiveIncrease:           options.SupportExtendedDaemonset.SlowStartAdditiveIncrease,
				CanaryDuration:                      options.SupportExtendedDaemonset.CanaryDuration,
				CanaryReplicas:                      options.SupportExtendedDaemonset.CanaryReplicas,
				CanaryAutoPauseEnabled:              options.SupportExtendedDaemonset.CanaryAutoPauseEnabled,
				CanaryAutoPauseMaxRestarts:          int32(options.SupportExtendedDaemonset.CanaryAutoPauseMaxRestarts),
				CanaryAutoPauseMaxSlowStartDuration: options.SupportExtendedDaemonset.CanaryAutoPauseMaxSlowStartDuration,
				CanaryAutoFailEnabled:               options.SupportExtendedDaemonset.CanaryAutoFailEnabled,
				CanaryAutoFailMaxRestarts:           int32(options.SupportExtendedDaemonset.CanaryAutoFailMaxRestarts),
			},
			SupportCilium:               options.SupportCilium,
			OperatorMetricsEnabled:      options.OperatorMetricsEnabled,
			IntrospectionEnabled:        options.IntrospectionEnabled,
			DatadogAgentProfileEnabled:  options.DatadogAgentProfileEnabled,
			DatadogAgentInternalEnabled: options.DatadogAgentInternalEnabled,
		},
	}).SetupWithManager(mgr, metricForwardersMgr)
}

func startDatadogAgentInternal(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogAgentInternalEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", agentInternalControllerName)
		return nil
	}

	return (&DatadogAgentInternalReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName(agentInternalControllerName),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(agentInternalControllerName),
	}).SetupWithManager(mgr, metricForwardersMgr)
}

func startDatadogMonitor(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogMonitorEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", monitorControllerName)

		return nil
	}

	ddClient, err := datadogclient.InitDatadogMonitorClient(logger, options.Creds)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client: %w", err)
	}

	return (&DatadogMonitorReconciler{
		Client:                 mgr.GetClient(),
		DDClient:               ddClient,
		Log:                    ctrl.Log.WithName("controllers").WithName(monitorControllerName),
		Scheme:                 mgr.GetScheme(),
		Recorder:               mgr.GetEventRecorderFor(monitorControllerName),
		operatorMetricsEnabled: options.OperatorMetricsEnabled,
	}).SetupWithManager(mgr, metricForwardersMgr)
}

func startDatadogDashboard(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogDashboardEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", dashboardControllerName)
		return nil
	}

	ddClient, err := datadogclient.InitDatadogDashboardClient(logger, options.Creds)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client: %w", err)
	}

	return (&DatadogDashboardReconciler{
		Client:   mgr.GetClient(),
		DDClient: ddClient,
		Log:      ctrl.Log.WithName("controllers").WithName(dashboardControllerName),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(dashboardControllerName),
	}).SetupWithManager(mgr)
}

func startDatadogGenericResource(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogGenericResourceEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", genericResourceControllerName)
		return nil
	}

	ddClient, err := datadogclient.InitDatadogGenericClient(logger, options.Creds)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client: %w", err)
	}

	return (&DatadogGenericResourceReconciler{
		Client:   mgr.GetClient(),
		DDClient: ddClient,
		Log:      ctrl.Log.WithName("controllers").WithName(genericResourceControllerName),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(genericResourceControllerName),
	}).SetupWithManager(mgr)
}

func startDatadogSLO(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogSLOEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", sloControllerName)
		return nil
	}

	ddClient, err := datadogclient.InitDatadogSLOClient(logger, options.Creds)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client: %w", err)
	}

	controller := &DatadogSLOReconciler{
		Client:   mgr.GetClient(),
		DDClient: ddClient,
		Log:      ctrl.Log.WithName("controllers").WithName(sloControllerName),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(sloControllerName),
	}

	return controller.SetupWithManager(mgr)
}

func startDatadogAgentProfiles(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogAgentProfileEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", profileControllerName)
		return nil
	}

	return (&DatadogAgentProfileReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName(profileControllerName),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(profileControllerName),
	}).SetupWithManager(mgr)
}
