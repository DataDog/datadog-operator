// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ShouldReturn returns if we should stop the reconcile loop based on result
func ShouldReturn(result reconcile.Result, err error) bool {
	return err != nil || result.Requeue || result.RequeueAfter > 0
}

// GetDatadogLeaderElectionResourceName return the nome of the Resource managing the leader election token info.
func GetDatadogLeaderElectionResourceName(dda metav1.Object) string {
	return dda.GetName() + "-leader-election"
}

// GetDatadogAgentResourceNamespace returns the namespace of the Datadog Agent Resource
func GetDatadogAgentResourceNamespace(dda metav1.Object) string {
	return dda.GetNamespace()
}

// GetDatadogTokenResourceName returns the name of the ConfigMap used by the cluster agent to store token
func GetDatadogTokenResourceName(dda metav1.Object) string {
	return dda.GetName() + "token"
}

// GetDatadogAgentResourceUID returns the UID of the Datadog Agent Resource
func GetDatadogAgentResourceUID(dda metav1.Object) string {
	return string(dda.GetUID())
}

// GetDatadogAgentResourceCreationTime returns the creation timestamp of the Datadog Agent Resource
func GetDatadogAgentResourceCreationTime(dda metav1.Object) string {
	return strconv.FormatInt(dda.GetCreationTimestamp().Unix(), 10)
}

// UseCustomSeccompConfigMap returns true if a custom Seccomp profile configMap is configured
func UseCustomSeccompConfigMap(seccompConfig *v2alpha1.SeccompConfig) bool {
	return seccompConfig != nil && seccompConfig.CustomProfile != nil && seccompConfig.CustomProfile.ConfigMap != nil
}

// UseCustomSeccompConfigData returns true if a custom Seccomp profile configData is configured and configMap is *not* configured
func UseCustomSeccompConfigData(seccompConfig *v2alpha1.SeccompConfig) bool {
	return seccompConfig != nil && seccompConfig.CustomProfile != nil && seccompConfig.CustomProfile.ConfigMap == nil && seccompConfig.CustomProfile.ConfigData != nil
}
