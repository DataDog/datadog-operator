// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubeactions

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kubeActionsRBACPrefix = "kube-actions"

	// DDKubeActionsEnabled is the env var that turns on the Kubernetes Actions
	// product in the Cluster Agent.
	DDKubeActionsEnabled = "DD_KUBEACTIONS_ENABLED"
)

func getRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), kubeActionsRBACPrefix, suffix)
}
