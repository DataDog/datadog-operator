// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

// Environment variable names for namespace watching configuration
const (
	// AgentWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogAgent controller.
	AgentWatchNamespaceEnvVar = "DD_AGENT_WATCH_NAMESPACE"
	// WatchNamespaceEnvVar is a comma-separated list of namespaces watched by all controllers, unless a controller-specific configuration is provided.
	// An empty value means the operator is running with cluster scope.
	WatchNamespaceEnvVar = "WATCH_NAMESPACE"

	// DashboardWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogDashboard controller.
	dashboardWatchNamespaceEnvVar = "DD_DASHBOARD_WATCH_NAMESPACE"
	// GenericResourceWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogGenericResource controller.
	genericResourceWatchNamespaceEnvVar = "DD_GENERIC_RESOURCE_WATCH_NAMESPACE"
	// MonitorWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogMonitor controller.
	monitorWatchNamespaceEnvVar = "DD_MONITOR_WATCH_NAMESPACE"
	// ProfilesWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogAgentProfile controller.
	profileWatchNamespaceEnvVar = "DD_AGENT_PROFILE_WATCH_NAMESPACE"
	// SLOWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogSLO controller.
	sloWatchNamespaceEnvVar = "DD_SLO_WATCH_NAMESPACE"
	// CSIDriverWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogCSIDriver controller.
	csiDriverWatchNamespaceEnvVar = "DD_CSIDRIVER_WATCH_NAMESPACE"
)

var (
	agentObj           = &datadoghqv2alpha1.DatadogAgent{}
	dashboardObj       = &datadoghqv1alpha1.DatadogDashboard{}
	genericResourceObj = &datadoghqv1alpha1.DatadogGenericResource{}
	monitorObj         = &datadoghqv1alpha1.DatadogMonitor{}
	sloObj             = &datadoghqv1alpha1.DatadogSLO{}
	profileObj         = &datadoghqv1alpha1.DatadogAgentProfile{}
	agentInternalObj   = &datadoghqv1alpha1.DatadogAgentInternal{}
	csiDriverObj       = &datadoghqv1alpha1.DatadogCSIDriver{}
	csiDaemonSetObj    = &appsv1.DaemonSet{}
	podObj             = &corev1.Pod{}
	nodeObj            = &corev1.Node{}
)

type WatchOptions struct {
	DatadogAgentEnabled               bool
	DatadogMonitorEnabled             bool
	DatadogSLOEnabled                 bool
	DatadogAgentProfileEnabled        bool
	IntrospectionEnabled              bool
	DatadogDashboardEnabled           bool
	DatadogGenericResourceEnabled     bool
	DatadogCSIDriverEnabled           bool
	UntaintControllerEnabled          bool
	UntaintControllerWaitForCSIDriver bool
	ManagedAgentInstallationEnabled   bool
	ManagedAgentInstallationNamespace string
}

// CacheOptions function configures Controller Runtime cache options on a resource level (supported in v0.16+).
// Datadog CRDs and additional resources required for their reconciliation will be cached only if the respective feature is enabled.
func CacheOptions(logger logr.Logger, opts WatchOptions) cache.Options {
	byObject := map[client.Object]cache.ByObject{}
	agentNamespaces := GetWatchNamespacesFromEnv(logger, AgentWatchNamespaceEnvVar)
	if opts.ManagedAgentInstallationEnabled {
		agentNamespaces = includeWatchNamespace(agentNamespaces, opts.ManagedAgentInstallationNamespace)
	}

	if opts.DatadogAgentEnabled {
		logger.Info("DatadogAgent Enabled", "watching namespaces", slices.Collect(maps.Keys(agentNamespaces)))
		byObject[agentObj] = cache.ByObject{
			Namespaces: agentNamespaces,
		}
	}

	if opts.DatadogDashboardEnabled {
		dashboardNamespaces := GetWatchNamespacesFromEnv(logger, dashboardWatchNamespaceEnvVar)
		logger.Info("DatadogDashboard Enabled", "watching namespaces", slices.Collect(maps.Keys(dashboardNamespaces)))
		byObject[dashboardObj] = cache.ByObject{
			Namespaces: dashboardNamespaces,
		}
	}

	if opts.DatadogGenericResourceEnabled {
		genericResourceNamespaces := GetWatchNamespacesFromEnv(logger, genericResourceWatchNamespaceEnvVar)
		logger.Info("DatadogGenericResource Enabled", "watching namespaces", slices.Collect(maps.Keys(genericResourceNamespaces)))
		byObject[genericResourceObj] = cache.ByObject{
			Namespaces: genericResourceNamespaces,
		}
	}

	if opts.DatadogMonitorEnabled {
		monitorNamespaces := GetWatchNamespacesFromEnv(logger, monitorWatchNamespaceEnvVar)
		logger.Info("DatadogMonitor Enabled", "watching namespaces", slices.Collect(maps.Keys(monitorNamespaces)))
		byObject[monitorObj] = cache.ByObject{
			Namespaces: monitorNamespaces,
		}
	}

	if opts.DatadogSLOEnabled {
		sloNamespaces := GetWatchNamespacesFromEnv(logger, sloWatchNamespaceEnvVar)
		logger.Info("DatadogSLO Enabled", "watching namespaces", slices.Collect(maps.Keys(sloNamespaces)))
		byObject[sloObj] = cache.ByObject{
			Namespaces: sloNamespaces,
		}
	}

	if opts.DatadogAgentProfileEnabled {
		agentProfileNamespaces := GetWatchNamespacesFromEnv(logger, profileWatchNamespaceEnvVar)
		if opts.ManagedAgentInstallationEnabled {
			agentProfileNamespaces = includeWatchNamespace(agentProfileNamespaces, opts.ManagedAgentInstallationNamespace)
		}
		logger.Info("DatadogAgentProfile Enabled", "watching namespace", slices.Collect(maps.Keys(agentProfileNamespaces)))
		byObject[profileObj] = cache.ByObject{
			Namespaces: agentProfileNamespaces,
		}
	}

	if opts.DatadogAgentProfileEnabled || opts.UntaintControllerEnabled {
		// For the profiles feature and untaint controller we need to list agent pods.
		// The profiles feature needs node name and labels; the untaint controller also needs
		// Status.Conditions to check readiness. Pods are watched in DatadogAgent namespace(s).
		// When untaint is configured to wait for CSI, widen to merged agent+CSI
		// namespaces and drop the pod informer label filter so CSI node-server pods
		// (app=datadog-csi-driver-node-server) are cached for dual-readiness untaint.
		podNamespaces := agentNamespaces
		var podLabel labels.Selector
		if opts.UntaintControllerEnabled && opts.UntaintControllerWaitForCSIDriver {
			csiDriverNamespaces := GetWatchNamespacesFromEnv(logger, csiDriverWatchNamespaceEnvVar)
			podNamespaces = maps.Clone(agentNamespaces)
			maps.Copy(podNamespaces, csiDriverNamespaces)
			logger.Info("Pod cache enabled for untaint with wait-for-CSI",
				"watching Pods in namespaces", slices.Collect(maps.Keys(podNamespaces)))
		} else {
			podLabel = labels.SelectorFromSet(map[string]string{
				common.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
			})
			logger.Info("Pod cache enabled", "watching Pods in namespaces", slices.Collect(maps.Keys(agentNamespaces)))
		}
		podByObject := cache.ByObject{
			Namespaces: podNamespaces,

			Transform: func(obj any) (any, error) {
				pod := obj.(*corev1.Pod)

				newPod := &corev1.Pod{
					TypeMeta: pod.TypeMeta,
					ObjectMeta: v1.ObjectMeta{
						Namespace: pod.Namespace,
						Name:      pod.Name,
						Labels:    pod.Labels,
					},
					Spec: corev1.PodSpec{
						NodeName: pod.Spec.NodeName,
					},
				}

				// The untaint controller needs Pod.Status.Conditions (readiness check)
				// and Pod.Status.StartTime (readiness-timeout clock).
				if opts.UntaintControllerEnabled {
					newPod.Status.Conditions = pod.Status.Conditions
					newPod.Status.StartTime = pod.Status.StartTime
				}

				return newPod, nil
			},
		}
		if podLabel != nil {
			podByObject.Label = podLabel
		}
		byObject[podObj] = podByObject
	}

	if opts.DatadogAgentProfileEnabled || opts.IntrospectionEnabled || opts.UntaintControllerEnabled {
		// Also for the profiles feature, introspection and untaint controller, we need to list the
		// nodes. The untaint controller additionally needs Spec.Taints to check for the target taint.
		// Note that if in the future we need to list or get pods or nodes and use other
		// fields we'll need to modify this function.
		//
		// Node being non-namespace resources shouldn't have a namespace configuration.
		byObject[nodeObj] = cache.ByObject{
			Transform: func(obj any) (any, error) {
				node := obj.(*corev1.Node)

				newNode := &corev1.Node{
					TypeMeta: node.TypeMeta,
					ObjectMeta: v1.ObjectMeta{
						Name:   node.Name,
						Labels: node.Labels,
					},
				}

				// The untaint controller needs Spec.Taints (target taint check) and
				// metadata.CreationTimestamp (scheduling-timeout clock).
				if opts.UntaintControllerEnabled {
					newNode.Spec.Taints = node.Spec.Taints
					newNode.CreationTimestamp = node.CreationTimestamp
				}

				return newNode, nil
			},
		}
	}

	// Since v1.27, DDAI is always tied to DDA — no separate flag. Kept as DDA guard since DDAI cache is only needed when DDA is enabled.
	if opts.DatadogAgentEnabled {
		logger.Info("DatadogAgentInternal Enabled", "watching namespaces", slices.Collect(maps.Keys(agentNamespaces)))
		byObject[agentInternalObj] = cache.ByObject{
			Namespaces: agentNamespaces,
		}
	}

	if opts.DatadogCSIDriverEnabled {
		csiDriverNamespaces := GetWatchNamespacesFromEnv(logger, csiDriverWatchNamespaceEnvVar)
		logger.Info("DatadogCSIDriver Enabled", "watching namespaces", slices.Collect(maps.Keys(csiDriverNamespaces)))
		byObject[csiDriverObj] = cache.ByObject{
			Namespaces: csiDriverNamespaces,
		}
		// The DaemonSet owned by DatadogCSIDriver lives in the CSIDriver namespace, which may
		// differ from the agent namespace covered by DefaultNamespaces. Explicitly add DaemonSet
		// to ByObject merging both so neither controller loses its cache coverage.
		daemonSetNamespaces := maps.Clone(agentNamespaces)
		maps.Copy(daemonSetNamespaces, csiDriverNamespaces)
		byObject[csiDaemonSetObj] = cache.ByObject{
			Namespaces: daemonSetNamespaces,
		}
	}

	return cache.Options{
		// DefaultNamespaces is set to DatadogAgent CRD namespaces so all resources needed for DatadogAgent reconciliation
		// are cached from the same namespace(s) as the DatadogAgent.
		DefaultNamespaces: agentNamespaces,
		ByObject:          byObject,
	}
}

func includeWatchNamespace(namespaces map[string]cache.Config, namespace string) map[string]cache.Config {
	if namespace == "" {
		return namespaces
	}
	if _, watchesAllNamespaces := namespaces[cache.AllNamespaces]; watchesAllNamespaces {
		return namespaces
	}
	namespaces = maps.Clone(namespaces)
	namespaces[namespace] = cache.Config{}
	return namespaces
}

// GetWatchNamespacesFromEnv retrieves the list of namespaces to watch from environment variables.
func GetWatchNamespacesFromEnv(logger logr.Logger, envVar string) map[string]cache.Config {
	cacheConfig := cache.Config{}

	nsEnvValue, found := os.LookupEnv(envVar)
	if !found {
		logger.Info(fmt.Sprintf("CRD-specific namespaces environmental variable %s not set, will be using common config", envVar))
		nsEnvValue, found = os.LookupEnv(WatchNamespaceEnvVar)
		if !found {
			logger.Info(fmt.Sprintf("Common namespaces environmental variable %s not set, will be watching all namespaces", WatchNamespaceEnvVar))
			return map[string]cache.Config{cache.AllNamespaces: cacheConfig}
		}
	}

	namespaces := strings.Split(nsEnvValue, ",")
	nsConfigs := make(map[string]cache.Config)
	for _, ns := range namespaces {
		nsConfigs[strings.TrimSpace(ns)] = cache.Config{}
	}
	return nsConfigs
}
