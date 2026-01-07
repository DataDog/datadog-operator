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
				"checksum/otel_agent-custom-config": "c4efec532ac1feb5548c7bf9a000f3bd",
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
