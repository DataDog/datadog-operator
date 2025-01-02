// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"fmt"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"

	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"

	"github.com/go-logr/logr"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

const (
	agentControllerName     = "DatadogAgent"
	monitorControllerName   = "DatadogMonitor"
	sloControllerName       = "DatadogSLO"
	profileControllerName   = "DatadogAgentProfile"
	dashboardControllerName = "DatadogDashboard"
	genericCRControllerName = "DatadogGenericCR"
)

// SetupOptions defines options for setting up controllers to ease testing
type SetupOptions struct {
	SupportExtendedDaemonset   ExtendedDaemonsetOptions
	SupportCilium              bool
	Creds                      config.Creds
	DatadogAgentEnabled        bool
	DatadogMonitorEnabled      bool
	DatadogSLOEnabled          bool
	OperatorMetricsEnabled     bool
	V2APIEnabled               bool
	IntrospectionEnabled       bool
	DatadogAgentProfileEnabled bool
	DatadogDashboardEnabled    bool
	DatadogGenericCREnabled    bool
}

// ExtendedDaemonsetOptions defines ExtendedDaemonset options
type ExtendedDaemonsetOptions struct {
	Enabled                bool
	MaxPodUnavailable      string
	MaxPodSchedulerFailure string

	CanaryDuration                      time.Duration
	CanaryReplicas                      string
	CanaryAutoPauseEnabled              bool
	CanaryAutoPauseMaxRestarts          int
	CanaryAutoFailEnabled               bool
	CanaryAutoFailMaxRestarts           int
	CanaryAutoPauseMaxSlowStartDuration time.Duration
}

type starterFunc func(logr.Logger, manager.Manager, kubernetes.PlatformInfo, SetupOptions, datadog.MetricForwardersManager) error

var controllerStarters = map[string]starterFunc{
	agentControllerName:     startDatadogAgent,
	monitorControllerName:   startDatadogMonitor,
	sloControllerName:       startDatadogSLO,
	profileControllerName:   startDatadogAgentProfiles,
	dashboardControllerName: startDatadogDashboard,
	genericCRControllerName: startDatadogGenericCR,
}

// SetupControllers starts all controllers (also used by e2e tests)
func SetupControllers(logger logr.Logger, mgr manager.Manager, options SetupOptions) error {
	// Get some information about Kubernetes version
	// Never use original mgr.GetConfig(), always copy as clients might modify the configuration
	discoveryConfig := rest.CopyConfig(mgr.GetConfig())
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(discoveryConfig)
	if err != nil {
		return fmt.Errorf("unable to get discovery client: %w", err)
	}

	versionInfo, err := discoveryClient.ServerVersion()
	if err != nil {
		return fmt.Errorf("unable to get APIServer version: %w", err)
	}

	if versionInfo != nil {
		gitVersion := versionInfo.GitVersion
		if !utils.IsAboveMinVersion(gitVersion, "1.16-0") {
			logger.Error(nil, "Detected Kubernetes version <1.16 which requires CRD version apiextensions.k8s.io/v1beta1. "+
				"CRDs of this version will be deprecated and will not be updated starting with Operator v1.8.0 and will be removed in v1.10.0.")
		}
	}

	groups, resources, err := getServerGroupsAndResources(logger, discoveryClient)
	if err != nil {
		return fmt.Errorf("unable to get API resource versions: %w", err)
	}
	platformInfo := kubernetes.NewPlatformInfo(versionInfo, groups, resources)

	var metricForwardersMgr datadog.MetricForwardersManager
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

func getServerGroupsAndResources(log logr.Logger, discoveryClient *discovery.DiscoveryClient) ([]*v1.APIGroup, []*v1.APIResourceList, error) {
	groups, resources, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) {
			log.Info("GetServerGroupsAndResources ERROR", "err", err)
			return nil, nil, err
		}
	}
	return groups, resources, nil
}

func startDatadogAgent(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricForwardersManager) error {
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
		},
	}).SetupWithManager(mgr, metricForwardersMgr)
}

func startDatadogMonitor(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricForwardersManager) error {
	if !options.DatadogMonitorEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", monitorControllerName)

		return nil
	}

	ddClient, err := datadogclient.InitDatadogMonitorClient(logger, options.Creds)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client: %w", err)
	}

	return (&DatadogMonitorReconciler{
		Client:   mgr.GetClient(),
		DDClient: ddClient,
		Log:      ctrl.Log.WithName("controllers").WithName(monitorControllerName),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(monitorControllerName),
	}).SetupWithManager(mgr)
}

func startDatadogDashboard(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricForwardersManager) error {
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

func startDatadogGenericCR(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricForwardersManager) error {
	if !options.DatadogGenericCREnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", genericCRControllerName)
		return nil
	}

	ddClient, err := datadogclient.InitDatadogGenericClient(logger, options.Creds)
	if err != nil {
		return fmt.Errorf("unable to create Datadog API Client: %w", err)
	}

	return (&DatadogGenericCRReconciler{
		Client:   mgr.GetClient(),
		DDClient: ddClient,
		Log:      ctrl.Log.WithName("controllers").WithName(genericCRControllerName),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(genericCRControllerName),
	}).SetupWithManager(mgr)
}

func startDatadogSLO(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricForwardersManager) error {
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

func startDatadogAgentProfiles(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricForwardersManager) error {
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
