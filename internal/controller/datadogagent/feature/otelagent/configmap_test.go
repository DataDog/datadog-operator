// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagent

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelagent/defaultconfig"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_otelAgent_buildOtelAgentConfigMap(t *testing.T) {
	// check config map
	configMapWant := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "-otel-agent-config",
		},
		Data: map[string]string{
			"otel-config.yaml": defaultconfig.DefaultOtelCollectorConfig,
		},
	}

	otelAgentFeature, ok := buildOtelAgentFeature(&feature.Options{}).(*otelAgentFeature)
	assert.True(t, ok)

	otelAgentFeature.owner = &metav1.ObjectMeta{
		Name:      "-otel-agent-config",
	}
	otelAgentFeature.configMapName = "-otel-agent-config"
	otelAgentFeature.customConfig = &v2alpha1.CustomConfig{}
	otelAgentFeature.customConfig.ConfigData = &defaultconfig.DefaultOtelCollectorConfig

	configMap, err := otelAgentFeature.buildOTelAgentCoreConfigMap()
	assert.NoError(t, err)
	assert.Equal(t, configMapWant, configMap)
}
