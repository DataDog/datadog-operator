// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"os"
	"strings"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	WatchNamespaceEnvVar = "WATCH_NAMESPACE"
	// DDAPIKeyEnvVar is the constant for the env variable DD_API_KEY which is the fallback
	// API key to use if a resource does not have it defined in its spec
	DDAPIKeyEnvVar = "DD_API_KEY"
	// DDAppKeyEnvVar is the constant for the env variable DD_APP_KEY which is the fallback
	// App key to use if a resource does not have it defined in its spec
	DDAppKeyEnvVar = "DD_APP_KEY"
	// DDURLEnvVar is the constant for the env variable DD_URL which is the
	// host of the Datadog intake server to send data to
	DDURLEnvVar = "DD_URL"
)

// GetWatchNamespaces returns the Namespaces the operator should be watching for changes
func GetWatchNamespaces() []string {
	ns, found := os.LookupEnv(WatchNamespaceEnvVar)
	if !found {
		return nil
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	if strings.Contains(ns, ",") {
		return strings.Split(ns, ",")
	}

	return []string{ns}
}

// ManagerOptionsWithNamespaces returns an updated Options with namespaces information
func ManagerOptionsWithNamespaces(logger logr.Logger, opt ctrl.Options) ctrl.Options {
	namespaces := GetWatchNamespaces()
	switch {
	case len(namespaces) == 0:
		logger.Info("Manager will watch and manage resources in all namespaces")
	case len(namespaces) == 1:
		logger.Info("Manager will be watching namespace", namespaces[0])
		opt.Namespace = namespaces[0]
	case len(namespaces) > 1:
		// configure cluster-scoped with MultiNamespacedCacheBuilder
		logger.Info("Manager will be watching multiple namespaces", namespaces)
		opt.Namespace = ""
		opt.NewCache = cache.MultiNamespacedCacheBuilder(namespaces)
	}

	return opt
}
