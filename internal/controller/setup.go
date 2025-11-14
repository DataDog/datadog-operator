// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
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
	CredsManager                  *config.CredentialManager
	Creds                         config.Creds
	SecretRefreshInterval         time.Duration
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
	// Metrics Forwarder created -- creds
	var metricForwardersMgr datadog.MetricsForwardersManager
	if options.OperatorMetricsEnabled {
		metricForwardersMgr = datadog.NewForwardersManager(mgr.GetClient(), &platformInfo, options.DatadogAgentInternalEnabled, options.CredsManager)
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
		Client:       mgr.GetClient(),
		PlatformInfo: pInfo,
		Log:          ctrl.Log.WithName("controllers").WithName(agentInternalControllerName),
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
		CredManager:            options.CredsManager,
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
		Client:      mgr.GetClient(),
		CredManager: options.CredsManager,
		Log:         ctrl.Log.WithName("controllers").WithName(dashboardControllerName),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor(dashboardControllerName),
	}

	return dashboardReconciler.SetupWithManager(mgr)
}

func startDatadogGenericResource(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogGenericResourceEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", genericResourceControllerName)
		return nil
	}

	genericResourceReconciler := &DatadogGenericResourceReconciler{
		Client:      mgr.GetClient(),
		CredManager: options.CredsManager,
		Log:         ctrl.Log.WithName("controllers").WithName(genericResourceControllerName),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor(genericResourceControllerName),
	}

	return genericResourceReconciler.SetupWithManager(mgr)
}

func startDatadogSLO(logger logr.Logger, mgr manager.Manager, pInfo kubernetes.PlatformInfo, options SetupOptions, metricForwardersMgr datadog.MetricsForwardersManager) error {
	if !options.DatadogSLOEnabled {
		logger.Info("Feature disabled, not starting the controller", "controller", sloControllerName)
		return nil
	}

	sloReconciler := &DatadogSLOReconciler{
		Client:      mgr.GetClient(),
		CredManager: options.CredsManager,
		Log:         ctrl.Log.WithName("controllers").WithName(sloControllerName),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor(sloControllerName),
	}

	return sloReconciler.SetupWithManager(mgr)
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

// CleanupDatadogAgentInternalResources removes leftover DatadogAgentInternal resources when DDAI controller is disabled
func CleanupDatadogAgentInternalResources(logger logr.Logger, restConfig *rest.Config) error {
	logger.Info("Cleaning up leftover DatadogAgentInternal resources")

	// Create a dynamic client for direct API server calls
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Define the GVR for DatadogAgentInternal
	ddaiGVR := schema.GroupVersionResource{
		Group:    rbac.DatadogAPIGroup,
		Version:  "v1alpha1",
		Resource: rbac.DatadogAgentInternalsResource,
	}

	// Try to list DDAI resources directly - this will fail if CRD doesn't exist
	ddaiList, err := dynamicClient.Resource(ddaiGVR).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DatadogAgentInternal CRD not found, skipping cleanup")
			return nil
		}
		return fmt.Errorf("failed to list DatadogAgentInternal resources: %w", err)
	}

	logger.Info("Found DatadogAgentInternal resources to cleanup", "count", len(ddaiList.Items))

	// Process each DDAI resource
	for _, ddai := range ddaiList.Items {
		namespace := ddai.GetNamespace()
		name := ddai.GetName()

		logger.Info("Cleaning up DatadogAgentInternal resource", "namespace", namespace, "name", name)

		// Remove finalizer if it exists
		finalizers := ddai.GetFinalizers()
		if len(finalizers) > 0 {
			// Create a patch to remove the finalizer
			patchData := []byte(`{"metadata":{"finalizers":[]}}`)

			_, err = dynamicClient.Resource(ddaiGVR).Namespace(namespace).Patch(
				context.TODO(),
				name,
				types.MergePatchType,
				patchData,
				metav1.PatchOptions{},
			)
			if err != nil {
				if apierrors.IsNotFound(err) {
					logger.Info("DatadogAgentInternal resource already deleted", "namespace", namespace, "name", name)
					continue
				}
				logger.Error(err, "Failed to remove finalizer from DatadogAgentInternal resource", "namespace", namespace, "name", name)
				// Continue with other resources even if one fails
				continue
			}

			logger.Info("Removed finalizer from DatadogAgentInternal resource", "namespace", namespace, "name", name)
		}

		// Delete the resource
		err = dynamicClient.Resource(ddaiGVR).Namespace(namespace).Delete(
			context.TODO(),
			name,
			metav1.DeleteOptions{},
		)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("DatadogAgentInternal resource already deleted", "namespace", namespace, "name", name)
				continue
			}
			logger.Error(err, "Failed to delete DatadogAgentInternal resource", "namespace", namespace, "name", name)
			// Continue with other resources even if one fails
			continue
		}

		logger.Info("Successfully deleted DatadogAgentInternal resource", "namespace", namespace, "name", name)
	}

	logger.Info("Completed cleanup of DatadogAgentInternal resources")
	return nil
}
