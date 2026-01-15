// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagentgateway

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelagentgateway/defaultconfig"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_buildOtelAgentGatewayConfigMap(t *testing.T) {
	// check config map
	configMapWant := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "-otel-agent-gateway-config",
			Annotations: map[string]string{
				"checksum/otel_agent_gateway-custom-config": "271b7a21b7215c549ce1d617f2064a3f",
			},
		},
		Data: map[string]string{
			"otel-config.yaml": defaultconfig.DefaultOtelAgentGatewayConfig,
		},
	}

	otelAgentGatewayFeature, ok := buildOtelAgentGatewayFeature(&feature.Options{}).(*otelAgentGatewayFeature)
	assert.True(t, ok)

	otelAgentGatewayFeature.owner = &metav1.ObjectMeta{
		Name: "-otel-agent-gateway-config",
	}
	otelAgentGatewayFeature.configMapName = "-otel-agent-gateway-config"
	otelAgentGatewayFeature.customConfig = &v2alpha1.CustomConfig{}
	otelAgentGatewayFeature.customConfig.ConfigData = &defaultconfig.DefaultOtelAgentGatewayConfig

	configMap, err := otelAgentGatewayFeature.buildOTelAgentCoreConfigMap()
	assert.NoError(t, err)
	assert.Equal(t, configMapWant, configMap)
}

func Test_buildOtelAgentGatewayConfigMap_NoCustomConfig(t *testing.T) {
	otelAgentGatewayFeature, ok := buildOtelAgentGatewayFeature(&feature.Options{}).(*otelAgentGatewayFeature)
	assert.True(t, ok)

	otelAgentGatewayFeature.owner = &metav1.ObjectMeta{
		Name: "test-dda",
	}
	otelAgentGatewayFeature.configMapName = "test-config"
	otelAgentGatewayFeature.customConfig = nil

	configMap, err := otelAgentGatewayFeature.buildOTelAgentCoreConfigMap()
	assert.NoError(t, err)
	assert.Nil(t, configMap)
}

func Test_buildOtelAgentGatewayConfigMap_NoConfigData(t *testing.T) {
	otelAgentGatewayFeature, ok := buildOtelAgentGatewayFeature(&feature.Options{}).(*otelAgentGatewayFeature)
	assert.True(t, ok)

	otelAgentGatewayFeature.owner = &metav1.ObjectMeta{
		Name: "test-dda",
	}
	otelAgentGatewayFeature.configMapName = "test-config"
	otelAgentGatewayFeature.customConfig = &v2alpha1.CustomConfig{
		ConfigMap: &v2alpha1.ConfigMapConfig{
			Name: "external-config",
		},
	}

	configMap, err := otelAgentGatewayFeature.buildOTelAgentCoreConfigMap()
	assert.NoError(t, err)
	assert.Nil(t, configMap)
}
