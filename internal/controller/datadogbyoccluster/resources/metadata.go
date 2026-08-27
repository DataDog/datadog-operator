// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"maps"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
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

func labels(cluster *datadoghqv1alpha1.DatadogBYOCCluster, overrides ...map[string]string) map[string]string {
	result := map[string]string{
		"app.kubernetes.io/name":       appName,
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/managed-by": "datadog-operator",
	}
	maps.Copy(result, cluster.Spec.Global.Labels)
	for _, override := range overrides {
		maps.Copy(result, override)
	}
	return result
}

func annotations(cluster *datadoghqv1alpha1.DatadogBYOCCluster, overrides ...map[string]string) map[string]string {
	result := map[string]string{}
	maps.Copy(result, cluster.Spec.Global.Annotations)
	for _, override := range overrides {
		maps.Copy(result, override)
	}
	return result
}
