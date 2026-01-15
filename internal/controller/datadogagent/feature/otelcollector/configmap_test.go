// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelcollector

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector/defaultconfig"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_buildOtelCollectorConfigMap(t *testing.T) {
	// check config map
	configMapWant := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "-otel-agent-config",
			Annotations: map[string]string{
				"checksum/otel_agent-custom-config": "8e715f9526c27c6cd06ba9a9d8913451",
			},
		},
		Data: map[string]string{
			"otel-config.yaml": defaultconfig.DefaultOtelCollectorConfig,
		},
	}

	otelCollectorFeature, ok := buildOtelCollectorFeature(&feature.Options{}).(*otelCollectorFeature)
	assert.True(t, ok)

	otelCollectorFeature.owner = &metav1.ObjectMeta{
		Name: "-otel-agent-config",
	}
	otelCollectorFeature.configMapName = "-otel-agent-config"
	otelCollectorFeature.customConfig = &v2alpha1.CustomConfig{}
	otelCollectorFeature.customConfig.ConfigData = &defaultconfig.DefaultOtelCollectorConfig

	configMap, err := otelCollectorFeature.buildOTelAgentCoreConfigMap()
	assert.NoError(t, err)
	assert.Equal(t, configMapWant, configMap)
}

func Test_buildOtelCollectorConfigMapWithGateway(t *testing.T) {
	ddaName := "test-dda"
	gatewayConfig := defaultconfig.DefaultOtelCollectorConfigInGateway(ddaName)

	// check config map with gateway enabled
	configMapWant := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: ddaName + "-otel-agent-config",
			Annotations: map[string]string{
				"checksum/otel_agent-custom-config": "9a9429a6e4d1662ef90e6e84b6416fa6",
			},
		},
		Data: map[string]string{
			"otel-config.yaml": gatewayConfig,
		},
	}

	otelCollectorFeature, ok := buildOtelCollectorFeature(&feature.Options{}).(*otelCollectorFeature)
	assert.True(t, ok)

	otelCollectorFeature.owner = &metav1.ObjectMeta{
		Name: ddaName,
	}
	otelCollectorFeature.configMapName = ddaName + "-otel-agent-config"
	otelCollectorFeature.customConfig = &v2alpha1.CustomConfig{}
	otelCollectorFeature.customConfig.ConfigData = &gatewayConfig
	otelCollectorFeature.otelGatewayEnabled = true

	configMap, err := otelCollectorFeature.buildOTelAgentCoreConfigMap()
	assert.NoError(t, err)
	assert.Equal(t, configMapWant, configMap)
}
