// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import "k8s.io/apimachinery/pkg/types"

const testManagedAgentInstallationNamespace = "datadog-agent"

var (
	managedAgentInstallationTarget = types.NamespacedName{
		Namespace: testManagedAgentInstallationNamespace,
		Name:      fleetDatadogAgentName,
	}
	managedAgentInstallationCredentialKey = types.NamespacedName{
		Namespace: testManagedAgentInstallationNamespace,
		Name:      fleetCredentialSecretName,
	}
	managedAgentInstallationIntentKey = types.NamespacedName{
		Namespace: testManagedAgentInstallationNamespace,
		Name:      managedAgentInstallationIntentConfigMapName,
	}
	managedAgentInstallationStateKey = types.NamespacedName{
		Namespace: testManagedAgentInstallationNamespace,
		Name:      managedAgentInstallationStateConfigMapName,
	}
	managedAgentInstallationWindowsProfileKey = types.NamespacedName{
		Namespace: testManagedAgentInstallationNamespace,
		Name:      managedAgentInstallationWindowsProfileName,
	}
)
