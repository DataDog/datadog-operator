// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package otelcollector

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	otelAgentVolumeName = "otel-agent-config-volume"
	otelConfigFileName  = "otel-config.yaml"
	// DefaultOTelAgentConf default otel agent ConfigMap name
	defaultOTelAgentConf string = "otel-agent-config"
)

// getRBACResourceName return the RBAC resources name
func getRBACResourceName(owner metav1.Object) string {
	return fmt.Sprintf("%s-%s", owner.GetName(), "otel-agent")
}
