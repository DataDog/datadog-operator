// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cspmRBACPrefix = "cspm"
)

// CELENE FIX ME
func getSCCName() string {
	return "datadog-agent"
}

func getPSPName() string {
	return "datadog-agent"
}

// getRBACResourceName return the RBAC resources name
func getRBACResourceName(owner metav1.Object) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), cspmRBACPrefix, "cluster-agent")
}
