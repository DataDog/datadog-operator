// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	orchestratorExplorerRBACPrefix   = "orch-exp"
	orchestratorExplorerConfFileName = "orchestrator.yaml"
	orchestratorExplorerFolderName   = "orchestrator.d"

	orchestratorExplorerVolumeName = "orchestrator-explorer-config"
)

// GetOrchestratorExplorerRBACResourceName returns the RBAC resources name
func GetOrchestratorExplorerRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), orchestratorExplorerRBACPrefix, suffix)
}
