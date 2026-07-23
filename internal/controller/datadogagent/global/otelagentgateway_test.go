// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
)

func Test_applyOtelAgentGatewayResources(t *testing.T) {
	mgr := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

	applyOtelAgentGatewayResources(mgr, &v2alpha1.DatadogAgentSpec{})

	wantEnvVars := []*corev1.EnvVar{
		{
			Name:  "DD_OTELCOLLECTOR_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_OTELCOLLECTOR_INSTALLATION_METHOD",
			Value: "kubernetes",
		},
		{
			Name:  "DD_OTELCOLLECTOR_GATEWAY_MODE",
			Value: "true",
		},
		{
			Name:  "DD_OTEL_STANDALONE",
			Value: "false",
		},
		{
			Name: "DD_HOSTNAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			},
		},
		{
			Name:  "DD_OTELCOLLECTOR_CONVERTER_FEATURES",
			Value: "health_check,zpages,pprof,datadog",
		},
		{
			Name:  "DD_ENABLE_METADATA_COLLECTION",
			Value: "false",
		},
		{
			Name:  "DD_PROCESS_AGENT_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_REMOTE_CONFIGURATION_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_INVENTORIES_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_CMD_PORT",
			Value: "0",
		},
		{
			Name:  "DD_AGENT_IPC_PORT",
			Value: "0",
		},
		{
			Name:  "DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL",
			Value: "0",
		},
	}

	otelAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.OtelAgent]
	assert.ElementsMatch(t, otelAgentEnvVars, wantEnvVars, "otel-agent gateway envvars \ndiff = %s", cmp.Diff(otelAgentEnvVars, wantEnvVars))
}
