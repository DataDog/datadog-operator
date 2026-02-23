// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package hostprofiler

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/hostprofiler/defaultconfig"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_buildHostProfilerConfigMap(t *testing.T) {
	// check config map
	configMapWant := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "-host-profiler-config",
			Annotations: map[string]string{
				"checksum/host_profiler-custom-config": "7b48d4d7ca198be0a6d7d8c7a5ad5535",
			},
		},
		Data: map[string]string{
			"host-profiler-config.yaml": defaultconfig.DefaultHostProfilerConfig,
		},
	}

	hostProfilerFeature, ok := buildHostProfilerFeature(&feature.Options{}).(*hostProfilerFeature)
	assert.True(t, ok)

	hostProfilerFeature.owner = &metav1.ObjectMeta{
		Name: "-host-profiler-config",
	}
	hostProfilerFeature.configMapName = "-host-profiler-config"
	hostProfilerFeature.customConfig = &v2alpha1.CustomConfig{}
	hostProfilerFeature.customConfig.ConfigData = &defaultconfig.DefaultHostProfilerConfig

	configMap, err := hostProfilerFeature.buildHostProfilerCoreConfigMap()
	assert.NoError(t, err)
	assert.Equal(t, configMapWant, configMap)
}
