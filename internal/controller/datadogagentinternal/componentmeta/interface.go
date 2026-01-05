// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package componentmeta

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ComponentMeta provides component-specific naming and metadata.
// This interface has minimal imports to avoid circular dependencies.
type ComponentMeta interface {
	// ComponentName returns the v2alpha1.ComponentName enum value
	// (e.g., "nodeAgent", "clusterAgent", "clusterChecksRunner")
	ComponentName() v2alpha1.ComponentName

	// ComponentSuffix returns the component suffix
	// (e.g., "cluster-agent", "agent", "cluster-checks-runner")
	ComponentSuffix() string

	// GetDefaultName returns the default workload name without overrides
	// Format: "{dda-name}-{ComponentSuffix}"
	// GetDefaultName(dda metav1.Object) string

	// GetName returns the workload name with override support
	// Checks ddaiSpec.Override[ComponentName].Name for user overrides
	GetWorkloadNameWithOverride(ddai metav1.Object, ddaiSpec *v2alpha1.DatadogAgentSpec) string

	// GetServiceAccountName returns the service account name with override support
	// Checks ddaSpec.Override[ComponentName].ServiceAccountName for user overrides
	GetServiceAccountNameWithOverride(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) string

	// GetRBACResourcesName returns the name used for Role/ClusterRole resources
	GetRBACResourcesName(ddai metav1.Object) string

	// GetServiceName returns the Kubernetes Service name
	// Returns "" if component has no service
	GetServiceName(ddai metav1.Object) string

	// GetDefaultServicePort returns the default service port
	// Returns 0 if component has no service
	// GetDefaultServicePort() int32

	// GetPDBName returns the PodDisruptionBudget name
	// Returns "" if component doesn't support PDB
	GetPDBName(ddai metav1.Object) string
}
