// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagentgateway

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
				"checksum/otel_agent-custom-config": "ae1d172378fb4bf7a4a3b3e712bc6bb6",
			},
		},
		Data: map[string]string{
			"otel-config.yaml": defaultconfig.DefaultOtelCollectorConfig,
		},
	}

	otelCollectorFeature, ok := buildotelAgentGatewayFeatures(&feature.Options{}).(*otelAgentGatewayFeatures)
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
