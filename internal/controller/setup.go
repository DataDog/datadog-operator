// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
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
	csiDriverControllerName       = "DatadogCSIDriver"
)

// SetupOptions defines options for setting up controllers to ease testing
type SetupOptions struct {
	SupportExtendedDaemonset       ExtendedDaemonsetOptions
	SupportCilium                  bool
	CredsManager                   *config.CredentialManager
	Creds                          config.Creds
	SecretRefreshInterval          time.Duration
	DatadogAgentEnabled            bool
	DatadogAgentInternalEnabled    bool
	DatadogMonitorEnabled          bool
	DatadogSLOEnabled              bool
	OperatorMetricsEnabled         bool
	V2APIEnabled                   bool
	IntrospectionEnabled           bool
	DatadogAgentProfileEnabled     bool
	OtelAgentEnabled               bool
	DatadogDashboardEnabled        bool
	DatadogGenericResourceEnabled  bool
	CreateControllerRevisions      bool
	DatadogCSIDriverEnabled        bool
	UntaintControllerEnabled       bool
	UntaintControllerEventsEnabled bool
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
	csiDriverControllerName:       startDatadogCSIDriver,
	untaintControllerName:         startUntaint,
}

// SetupControllers starts all controllers (also used by e2e tests)
func SetupControllers(logger logr.Logger, mgr manager.Manager, platformInfo kubernetes.PlatformInfo, options SetupOptions) error {
	// Metrics Forwarder created -- creds
	var metricForwardersMgr datadog.MetricsForwardersManager
	if options.OperatorMetricsEnabled {
		metricForwardersMgr = datadog.NewForwardersManager(mgr.GetClient(), &platformInfo, options.CredsManager)
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
			SupportCilium:              options.SupportCilium,
			OperatorMetricsEnabled:     options.OperatorMetricsEnabled,
			IntrospectionEnabled:       options.IntrospectionEnabled,
			DatadogAgentProfileEnabled: options.DatadogAgentProfileEnabled,
			DatadogCSIDriverEnabled:    options.DatadogCSIDriverEnabled,
			CreateControllerRevisions:  options.CreateControllerRevisions,
		},
	}).SetupWithManager(mgr, metricForwardersMgr)
}

func startDatadogAgentInternal(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	// Since v1.27, DatadogAgentInternal is always enabled when DatadogAgent is enabled.
	// There is no separate flag — DDAI is an internal implementation detail of DDA reconciliation.
	if !options.DatadogAgentEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", agentInternalControllerName)
		return nil
	}

	return (&DatadogAgentInternalReconciler{
		Client:       mgr.GetClient(),
		PlatformInfo: pInfo,
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor(agentInternalControllerName),
		Options: datadogagentinternal.ReconcilerOptions{
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
			SupportCilium:          options.SupportCilium,
			OperatorMetricsEnabled: options.OperatorMetricsEnabled,
		},
	}).SetupWithManager(mgr, metricForwardersMgr)
}

func startDatadogMonitor(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogMonitorEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", monitorControllerName)

		return nil
	}

	monitorReconciler := &DatadogMonitorReconciler{
		Client:                 mgr.GetClient(),
		CredsManager:           options.CredsManager,
		Log:                    ctrl.Log.WithName("controllers").WithName(monitorControllerName),
		Scheme:                 mgr.GetScheme(),
		Recorder:               mgr.GetEventRecorderFor(monitorControllerName),
		operatorMetricsEnabled: options.OperatorMetricsEnabled,
	}

	return monitorReconciler.SetupWithManager(mgr, metricForwardersMgr)
}

func startDatadogDashboard(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogDashboardEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", dashboardControllerName)
		return nil
	}

	dashboardReconciler := &DatadogDashboardReconciler{
		Client:       mgr.GetClient(),
		CredsManager: options.CredsManager,
		Log:          ctrl.Log.WithName("controllers").WithName(dashboardControllerName),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor(dashboardControllerName),
	}

	return dashboardReconciler.SetupWithManager(mgr)
}

func startDatadogGenericResource(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogGenericResourceEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", genericResourceControllerName)
		return nil
	}

	genericResourceReconciler := &DatadogGenericResourceReconciler{
		Client:       mgr.GetClient(),
		CredsManager: options.CredsManager,
		Log:          ctrl.Log.WithName("controllers").WithName(genericResourceControllerName),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor(genericResourceControllerName),
	}

	return genericResourceReconciler.SetupWithManager(mgr)
}

func startDatadogSLO(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogSLOEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", sloControllerName)
		return nil
	}

	sloReconciler := &DatadogSLOReconciler{
		Client:       mgr.GetClient(),
		CredsManager: options.CredsManager,
		Log:          ctrl.Log.WithName("controllers").WithName(sloControllerName),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor(sloControllerName),
	}

	return sloReconciler.SetupWithManager(mgr)
}

func startUntaint(logger logr.Logger, mgr manager.Manager, _ kubernetes.PlatformInfo, options SetupOptions, _ datadog.MetricsForwardersManager) error {
	if !options.UntaintControllerEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", untaintControllerName)
		return nil
	}

	logger.Info("untaint controller enabled", "controller", untaintControllerName, "eventsEnabled", options.UntaintControllerEventsEnabled)

	return (&UntaintReconciler{
		client:        mgr.GetClient(),
		log:           ctrl.Log.WithName("controllers").WithName(untaintControllerName),
		recorder:      mgr.GetEventRecorderFor(untaintControllerName),
		eventsEnabled: options.UntaintControllerEventsEnabled,
	}).SetupWithManager(mgr)
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

func startDatadogCSIDriver(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogCSIDriverEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", csiDriverControllerName)
		return nil
	}

	return (&DatadogCSIDriverReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(csiDriverControllerName),
	}).SetupWithManager(mgr)
}
