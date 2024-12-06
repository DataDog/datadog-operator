// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StatusWrapper interface {
	GetObjectMeta() metav1.Object
	GetAgentStatus() *v2alpha1.DaemonSetStatus
	GetClusterAgentStatus() *v2alpha1.DeploymentStatus
	GetClusterChecksRunnerStatus() *v2alpha1.DeploymentStatus
	GetStatusCondition() []metav1.Condition
}

func NewV2StatusWrapper(dda *v2alpha1.DatadogAgent) StatusWrapper {
	return &v2StatusWrapper{dda}
}

type v2StatusWrapper struct {
	dda *v2alpha1.DatadogAgent
}

func (sw v2StatusWrapper) GetObjectMeta() metav1.Object { return sw.dda.GetObjectMeta() }
func (sw v2StatusWrapper) GetStatusCondition() []metav1.Condition {
	return sw.dda.Status.Conditions
}

func (sw v2StatusWrapper) GetAgentStatus() *v2alpha1.DaemonSetStatus {
	if sw.dda != nil {
		return sw.dda.Status.Agent
	}
	return nil
}
func (sw v2StatusWrapper) GetClusterAgentStatus() *v2alpha1.DeploymentStatus {
	if sw.dda != nil {
		return sw.dda.Status.ClusterAgent
	}
	return nil
}
func (sw v2StatusWrapper) GetClusterChecksRunnerStatus() *v2alpha1.DeploymentStatus {
	if sw.dda != nil {
		return sw.dda.Status.ClusterChecksRunner
	}
	return nil
}
