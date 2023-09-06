// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StatusWrapper interface {
	GetObjectMeta() metav1.Object
	GetAgentStatus() *commonv1.DaemonSetStatus
	GetClusterAgentStatus() *commonv1.DeploymentStatus
	GetClusterChecksRunnerStatus() *commonv1.DeploymentStatus
}

func NewV1StatusWrapper(dda *v1alpha1.DatadogAgent) StatusWrapper {
	return &v1StatusWrapper{dda}
}

type v1StatusWrapper struct {
	dda *v1alpha1.DatadogAgent
}

func (sw v1StatusWrapper) GetObjectMeta() metav1.Object { return sw.dda.GetObjectMeta() }

func (sw v1StatusWrapper) GetAgentStatus() *commonv1.DaemonSetStatus {
	if sw.dda != nil {
		return sw.dda.Status.Agent
	}
	return nil
}
func (sw v1StatusWrapper) GetClusterAgentStatus() *commonv1.DeploymentStatus {
	if sw.dda != nil {
		return sw.dda.Status.ClusterAgent
	}
	return nil
}
func (sw v1StatusWrapper) GetClusterChecksRunnerStatus() *commonv1.DeploymentStatus {
	if sw.dda != nil {
		return sw.dda.Status.ClusterChecksRunner
	}
	return nil
}

func NewV2StatusWrapper(dda *v2alpha1.DatadogAgent) StatusWrapper {
	return &v2StatusWrapper{dda}
}

type v2StatusWrapper struct {
	dda *v2alpha1.DatadogAgent
}

func (sw v2StatusWrapper) GetObjectMeta() metav1.Object { return sw.dda.GetObjectMeta() }

func (sw v2StatusWrapper) GetAgentStatus() *commonv1.DaemonSetStatus {
	if sw.dda != nil {
		return sw.dda.Status.CombinedAgent
	}
	return nil
}
func (sw v2StatusWrapper) GetClusterAgentStatus() *commonv1.DeploymentStatus {
	if sw.dda != nil {
		return sw.dda.Status.ClusterAgent
	}
	return nil
}
func (sw v2StatusWrapper) GetClusterChecksRunnerStatus() *commonv1.DeploymentStatus {
	if sw.dda != nil {
		return sw.dda.Status.ClusterChecksRunner
	}
	return nil
}
