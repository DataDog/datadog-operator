// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controllers

import (
	"fmt"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/controllers/datadogagent"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

const (
	agentControllerName   = "DatadogAgent"
	monitorControllerName = "DatadogMonitor"
)

// SetupOptions defines options for setting up controllers to ease testing
type SetupOptions struct {
	SupportExtendedDaemonset bool
	SupportCilium            bool
	Creds                    config.Creds
	DatadogMonitorEnabled    bool
	OperatorMetricsEnabled   bool
	V2APIEnabled             bool
}

type starterFunc func(logr.Logger, manager.Manager, *version.Info, kubernetes.PlatformInfo, SetupOptions) error

var controllerStarters = map[string]starterFunc{
	agentControllerName:   startDatadogAgent,
	monitorControllerName: startDatadogMonitor,
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

	groups, resources, _ := getServerGroupsAndResources(logger, discoveryClient)
	platformInfo := getPlatformInfo(logger, groups, resources)

	for controller, starter := range controllerStarters {
		if err := starter(logger, mgr, versionInfo, platformInfo, options); err != nil {
			logger.Error(err, "Couldn't start controller", "controller", controller)
		}
	}

	return nil
}

func getPlatformInfo(log logr.Logger, groups []*v1.APIGroup, resources []*v1.APIResourceList) (platformInfo kubernetes.PlatformInfo) {
	preferredGroupVersions := make(map[string]struct{})
	log.Info("identifyResources")

	// Identify preferred group versions
	for _, group := range groups {
		log.Info("identifyResources: Pringing groups", "group", group)
		preferredGroupVersions[group.PreferredVersion.GroupVersion] = struct{}{}
	}

	preferred := make([]*v1.APIResourceList, 0, len(resources))
	others := make([]*v1.APIResourceList, 0, len(resources))
	// Triage resources
	for _, list := range resources {
		log.Info("identifyResources: Pringing resource lists", "list", list)
		if _, found := preferredGroupVersions[list.GroupVersion]; found {
			preferred = append(preferred, list)
		} else {
			others = append(others, list)
		}
	}

	platformInfo.ApiPreferredVersions = map[string]string{}
	platformInfo.ApiOtherVersions = map[string]string{}

	for i := range preferred {
		for j := range preferred[i].APIResources {
			log.Info("Using API group for the Kind",
				"name", preferred[i].APIResources[j].Kind,
				"groupVersion", preferred[i].GroupVersion,
			)
			platformInfo.ApiPreferredVersions[preferred[i].APIResources[j].Kind] = preferred[i].GroupVersion
		}
	}

	for i := range others {
		for j := range others[i].APIResources {
			log.Info("Using API group for the Kind",
				"name", others[i].APIResources[j].Kind,
				"groupVersion", others[i].GroupVersion,
			)
			platformInfo.ApiOtherVersions[others[i].APIResources[j].Kind] = others[i].GroupVersion
		}
	}

	log.Info("identifyResources: results", "preferred", preferred, "others", others)
	log.Info("platform info", "platformInfo", platformInfo)

	return platformInfo
}

func getServerGroupsAndResources(log logr.Logger, discoveryClient *discovery.DiscoveryClient) ([]*v1.APIGroup, []*v1.APIResourceList, error) {
	groups, resources, err := discoveryClient.ServerGroupsAndResources()

	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) {
			log.Info("GetServerGroupsAndResources ERROR", "err", err)
			return nil, nil, err
		}
		// We don't handle API group errors here because we assume API groups used
		// by collectors in the orchestrator check will always be part of the result
		// even though it might be incomplete due to discovery failures on other
		// groups.
		// for group, apiGroupErr := range err.(*discovery.ErrGroupDiscoveryFailed).Groups {
		// 	log.Info("Resources for API group version %s could not be discovered:", "group", group, "apiGroupErr", apiGroupErr)
		// }
	}
	return groups, resources, nil
}

func startDatadogAgent(logger logr.Logger, mgr manager.Manager, vInfo *version.Info, pInfo kubernetes.PlatformInfo, options SetupOptions) error {
	return (&DatadogAgentReconciler{
		Client:       mgr.GetClient(),
		VersionInfo:  vInfo,
		PlatformInfo: pInfo,
		Log:          ctrl.Log.WithName("controllers").WithName(agentControllerName),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor(agentControllerName),
		Options: datadogagent.ReconcilerOptions{
			SupportExtendedDaemonset: options.SupportExtendedDaemonset,
			SupportCilium:            options.SupportCilium,
			OperatorMetricsEnabled:   options.OperatorMetricsEnabled,
			V2Enabled:                options.V2APIEnabled,
		},
	}).SetupWithManager(mgr)
}

func startDatadogMonitor(logger logr.Logger, mgr manager.Manager, vInfo *version.Info, pInfo kubernetes.PlatformInfo, options SetupOptions) error {
	if !options.DatadogMonitorEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", monitorControllerName)

		return nil
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
