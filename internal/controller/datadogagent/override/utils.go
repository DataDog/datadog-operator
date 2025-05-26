// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func getDefaultConfigMapName(ddaName, fileName string) string {
	return fmt.Sprintf("%s-%s-yaml", ddaName, strings.Split(fileName, ".")[0])
}

func hasProbeHandler(probe *corev1.Probe) bool {
	handler := &probe.ProbeHandler
	if handler.Exec != nil || handler.HTTPGet != nil || handler.TCPSocket != nil || handler.GRPC != nil {
		return true
	}
	return false
}

func SetOverrideFromDDA(dda *v2alpha1.DatadogAgent, ddaiOverride map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride) {
	ddaiOverride[v2alpha1.NodeAgentComponentName] = componentOverride(dda.Name, ddaiOverride[v2alpha1.NodeAgentComponentName])
	ddaiOverride[v2alpha1.ClusterAgentComponentName] = componentOverride(dda.Name, ddaiOverride[v2alpha1.ClusterAgentComponentName])
	ddaiOverride[v2alpha1.ClusterChecksRunnerComponentName] = componentOverride(dda.Name, ddaiOverride[v2alpha1.ClusterChecksRunnerComponentName])
}

func componentOverride(ddaName string, ddaiOverride *v2alpha1.DatadogAgentComponentOverride) *v2alpha1.DatadogAgentComponentOverride {
	override := &v2alpha1.DatadogAgentComponentOverride{}
	if ddaiOverride != nil {
		override = ddaiOverride
	}

	if override.Labels == nil {
		override.Labels = make(map[string]string)
	}
	override.Labels[apicommon.OperatorDatadogAgentLabelKey] = ddaName
	return override
}
