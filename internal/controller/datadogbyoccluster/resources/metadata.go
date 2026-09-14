// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	controllerutils "github.com/DataDog/datadog-operator/internal/controller/utils"
)

// ComponentResourceName returns the Kubernetes resource name for a BYOC component.
func ComponentResourceName(clusterName, componentName string) string {
	return clusterName + "-" + componentName
}

// ComponentNames returns all BYOC Kubernetes component names.
func ComponentNames() []string {
	return []string{
		IndexerComponentName,
		SearcherComponentName,
		MetastoreComponentName,
		ControlPlaneComponentName,
		JanitorComponentName,
		ReadOnlyMetastoreComponentName,
		CompactorComponentName,
	}
}

func componentLabel(componentName string) map[string]string {
	return map[string]string{"app.kubernetes.io/component": componentName}
}

func labels(cluster *datadoghqv1alpha1.DatadogBYOCCluster, overrides ...map[string]string) map[string]string {
	values := make([]map[string]string, 0, 2+len(overrides))
	values = append(values, map[string]string{
		"app.kubernetes.io/name":       appName,
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/managed-by": "datadog-operator",
	}, cluster.Spec.Global.Labels)
	values = append(values, overrides...)
	return controllerutils.MergeStringMaps(values...)
}

func annotations(cluster *datadoghqv1alpha1.DatadogBYOCCluster, overrides ...map[string]string) map[string]string {
	values := make([]map[string]string, 0, 1+len(overrides))
	values = append(values, cluster.Spec.Global.Annotations)
	values = append(values, overrides...)
	result := controllerutils.MergeStringMaps(values...)
	if result == nil {
		return map[string]string{}
	}
	return result
}
