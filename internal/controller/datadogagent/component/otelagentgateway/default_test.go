// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagentgateway

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestNewDefaultOtelAgentGatewayDeployment(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "agents",
		},
	}

	deployment := NewDefaultOtelAgentGatewayDeployment(dda, &dda.Spec)

	require.Equal(t, "datadog-otel-agent-gateway", deployment.Name)
	require.Equal(t, "agents", deployment.Namespace)
	require.Equal(t, int32(defaultOtelAgentGatewayReplicas), *deployment.Spec.Replicas)
	require.Equal(t, constants.DefaultOtelAgentGatewayResourceSuffix, deployment.Labels[apicommon.AgentDeploymentComponentLabelKey])
	require.Equal(t, "datadog-otel-agent-gateway", deployment.Spec.Selector.MatchLabels[kubernetes.AppKubernetesInstanceLabelKey])
	require.Equal(t, deployment.Labels, deployment.Spec.Template.Labels)
	require.Equal(t, deployment.Annotations, deployment.Spec.Template.Annotations)
	require.Equal(t, GetOtelAgentGatewayRbacResourcesName(dda), deployment.Spec.Template.Spec.ServiceAccountName)
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	require.Equal(t, string(apicommon.OtelAgent), deployment.Spec.Template.Spec.Containers[0].Name)
}
