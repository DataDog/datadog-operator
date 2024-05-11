// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package get

import "github.com/DataDog/extendeddaemonset/api/v1alpha1"

func getCanaryRS(eds *v1alpha1.ExtendedDaemonSet) string {
	if eds.Status.Canary != nil {
		return eds.Status.Canary.ReplicaSet
	}

	return "-"
}
