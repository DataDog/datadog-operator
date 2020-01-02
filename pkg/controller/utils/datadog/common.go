// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadog

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("DatadogMetricForwarders")

// MonitoredObject must be implemented by the monitored object (e.g DatadogAgentDeployment)
type MonitoredObject interface {
	GetNamespace() string
	GetName() string
}

// getObjID builds an identifier for a given monitored object
func getObjID(obj MonitoredObject) string {
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

// getNamespacedName builds a NamespacedName for a given monitored object
func getNamespacedName(obj MonitoredObject) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}
