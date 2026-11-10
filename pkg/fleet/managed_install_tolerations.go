// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// IsFleetManagedDatadogAgent reports whether the DatadogAgent is a Fleet
// managed-install, stamped by Fleet Automation with the managed-by label.
func IsFleetManagedDatadogAgent(dda *v2alpha1.DatadogAgent) bool {
	return dda != nil && dda.Labels[fleetManagedByLabel] == fleetManagedByValue
}

// ManagedInstallNoScheduleToleration is the default node-Agent toleration for
// Fleet managed installs: a keyless Exists toleration limited to the
// NoSchedule effect, so the node Agent is placed on nodes carrying any
// NoSchedule taint (commonly customer-tainted GPU nodes), while NoExecute
// taints are deliberately not tolerated so operational evictions still drain
// the Agent.
func ManagedInstallNoScheduleToleration() corev1.Toleration {
	return corev1.Toleration{
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}
}
