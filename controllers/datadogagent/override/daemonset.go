// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	v1 "k8s.io/api/apps/v1"
)

// DaemonSet overrides a DaemonSet according to the given override options
func DaemonSet(daemonSet *v1.DaemonSet, override *v2alpha1.DatadogAgentComponentOverride) {
	if override.Name != nil {
		daemonSet.Name = *override.Name
	}
}
