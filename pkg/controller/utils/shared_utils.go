// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ShouldReturn returns if we should stop the reconcile loop based on result
func ShouldReturn(result reconcile.Result, err error) bool {
	return err != nil || result.Requeue || result.RequeueAfter > 0
}

// GetDatadogLeaderElectionResourceName return the nome of the Resource managing the leader election token info.
func GetDatadogLeaderElectionResourceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-leader-election", dda.GetName())
}

// GetDatadogTokenResourceName returns the name of the ConfigMap used by the cluster agent to store token
func GetDatadogTokenResourceName(dda metav1.Object) string {
	return fmt.Sprintf("%stoken", dda.GetName())
}

//GetDatadogTokenResourceNamespace returns the namespace of the Datadog Agent Resource
func GetDatadogAgentResourceNamespace(dda metav1.Object) string {
	return dda.GetNamespace()
}
