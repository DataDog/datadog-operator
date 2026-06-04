// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/DataDog/datadog-operator/pkg/untaint"
)

// podToleratesAgentNotReadyStartup reports whether tolerations tolerate the
// agent-not-ready startup taint, using the same rules as the scheduler/kubelet
// (corev1.Toleration.ToleratesTaint). Comparison-operator tolerations (Lt/Gt) are
// ignored unless the cluster enables that feature (we pass false).
func podToleratesAgentNotReadyStartup(tolerations []corev1.Toleration) bool {
	taint := untaint.AgentNotReadyTaint()
	for i := range tolerations {
		if tolerations[i].ToleratesTaint(klog.Background(), &taint, false) {
			return true
		}
	}
	return false
}

// EnsureAgentNotReadyStartupToleration appends the agent-not-ready Equal toleration
// when not already tolerated per Kubernetes toleration matching.
func EnsureAgentNotReadyStartupToleration(spec *corev1.PodSpec) {
	if spec == nil {
		return
	}
	if podToleratesAgentNotReadyStartup(spec.Tolerations) {
		return
	}
	spec.Tolerations = append(spec.Tolerations, untaint.AgentNotReadyEqualToleration())
}
