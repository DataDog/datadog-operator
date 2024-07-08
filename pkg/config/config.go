// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/labels"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
)

const (
	// AgentWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogAgent controller.
	agentWatchNamespaceEnvVar = "DD_AGENT_WATCH_NAMESPACE"
	// SLOWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogSLO controller.
	sloWatchNamespaceEnvVar = "DD_SLO_WATCH_NAMESPACE"
	// MonitorWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogMonitor controller.
	monitorWatchNamespaceEnvVar = "DD_MONITOR_WATCH_NAMESPACE"
	// ProfilesWatchNamespaceEnvVar is a comma-separated list of namespaces watched by the DatadogAgentProfile controller.
	profileWatchNamespaceEnvVar = "DD_AGENT_PROFILE_WATCH_NAMESPACE"
	// WatchNamespaceEnvVar is a comma-separated list of namespaces watched by all controllers, unless a controller-specific configuration is provided.
	// An empty value means the operator is running with cluster scope.
	watchNamespaceEnvVar = "WATCH_NAMESPACE"
	// DDAPIKeyEnvVar is the constant for the env variable DD_API_KEY which is the fallback
	// API key to use if a resource does not have it defined in its spec.
	DDAPIKeyEnvVar = "DD_API_KEY"
	// DDAppKeyEnvVar is the constant for the env variable DD_APP_KEY which is the fallback
	// App key to use if a resource does not have it defined in its spec.
	DDAppKeyEnvVar = "DD_APP_KEY"
	// DDURLEnvVar is the constant for the env variable DD_URL which is the
	// host of the Datadog intake server to send data to.
	DDURLEnvVar = "DD_URL"
	// TODO consider moving DDSite here as well
)

// CacheOptions function configures Controller Runtime cache options on a resource level (supported in v0.16+).
// Datadog CRDs and additional resources required for their reconciliation will be cached only if the respective feature is enabled.
func CacheOptions(logger logr.Logger, datadogAgentEnabled, datadogMonitorEnabled, datadogSLOEnabled, datadogAgentProfileEnabled bool) cache.Options {
	byObject := map[client.Object]cache.ByObject{}

	if datadogAgentEnabled {
		agentNamespaces := getWatchNamespacesFromEnv(logger, agentWatchNamespaceEnvVar)
		logger.Info("DatadogAgent Enabled", "watching namespaces", maps.Keys(agentNamespaces))
		byObject[&datadoghqv2alpha1.DatadogAgent{}] = cache.ByObject{
			Namespaces: agentNamespaces,
		}
	}

	if datadogMonitorEnabled {
		monitorNamespaces := getWatchNamespacesFromEnv(logger, monitorWatchNamespaceEnvVar)
		logger.Info("DatadogMonitor Enabled", "watching namespaces", maps.Keys(monitorNamespaces))
		byObject[&datadoghqv1alpha1.DatadogMonitor{}] = cache.ByObject{
			Namespaces: monitorNamespaces,
		}
	}

	if datadogSLOEnabled {
		sloNamespaces := getWatchNamespacesFromEnv(logger, sloWatchNamespaceEnvVar)
		logger.Info("DatadogSLO Enabled", "watching namespaces", maps.Keys(sloNamespaces))
		byObject[&datadoghqv1alpha1.DatadogSLO{}] = cache.ByObject{
			Namespaces: sloNamespaces,
		}
	}

	if datadogAgentProfileEnabled {
		agentProfileNamespaces := getWatchNamespacesFromEnv(logger, profileWatchNamespaceEnvVar)
		logger.Info("DatadogAgentProfile Enabled", "watching namespace", maps.Keys(agentProfileNamespaces))
		byObject[&datadoghqv1alpha1.DatadogAgentProfile{}] = cache.ByObject{
			Namespaces: agentProfileNamespaces,
		}

		// It is very important to reduce memory usage when profiles are used.
		// For the profiles feature we need to list the agent pods, but we're only
		// interested in the node name and the labels. This function removes all the
		// rest of fields to reduce memory usage.
		agentNamespaces := getWatchNamespacesFromEnv(logger, agentWatchNamespaceEnvVar)
		logger.Info("DatadogAgentProfile Enabled", "watching Pods in namespaces", maps.Keys(agentNamespaces))
		byObject[&corev1.Pod{}] = cache.ByObject{
			Namespaces: agentNamespaces,

			Label: labels.SelectorFromSet(map[string]string{
				common.AgentDeploymentComponentLabelKey: common.DefaultAgentResourceSuffix,
			}),

			Transform: func(obj interface{}) (interface{}, error) {
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

				return newPod, nil
			},
		}

		// Also for the profiles feature, we need to list the nodes, but we're only
		// interested in the node name and the labels.
		// Note that if in the future we need to list or get pods or nodes and use other
		// fields we'll need to modify this function.
		//
		// Node being non-namespace resources shouldn't have a namespace configuration.
		byObject[&corev1.Node{}] = cache.ByObject{
			Transform: func(obj interface{}) (interface{}, error) {
				node := obj.(*corev1.Node)

				newNode := &corev1.Node{
					TypeMeta: node.TypeMeta,
					ObjectMeta: v1.ObjectMeta{
						Name:   node.Name,
						Labels: node.Labels,
					},
				}

				return newNode, nil
			},
		}
	}

	return cache.Options{
		DefaultNamespaces: getWatchNamespacesFromEnv(logger, watchNamespaceEnvVar),
		ByObject:          byObject,
	}
}

func getWatchNamespacesFromEnv(logger logr.Logger, envVar string) map[string]cache.Config {
	cacheConfig := cache.Config{}

	nsEnvValue, found := os.LookupEnv(envVar)
	if !found {
		logger.Info(fmt.Sprintf("CRD-specific namespaces environmental variable %s not set, will be using common config", envVar))
		nsEnvValue, found = os.LookupEnv(watchNamespaceEnvVar)
		if !found {
			logger.Info("Common namespaces environmental variable %s not set, will be watching all namespaces", watchNamespaceEnvVar)
			return map[string]cache.Config{cache.AllNamespaces: cacheConfig}
		}
	}

	var namespaces []string
	if strings.Contains(nsEnvValue, ",") {
		namespaces = strings.Split(nsEnvValue, ",")
	} else {
		namespaces = []string{nsEnvValue}
	}
	nsConfigs := make(map[string]cache.Config)
	for _, ns := range namespaces {
		nsConfigs[strings.TrimSpace(ns)] = cache.Config{}
	}
	return nsConfigs
}
