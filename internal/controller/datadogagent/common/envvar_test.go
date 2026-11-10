// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestHealthPlatformDisabledEnvVar(t *testing.T) {
	assert.Equal(t, corev1.EnvVar{Name: "DD_HEALTH_PLATFORM_ENABLED", Value: "false"}, HealthPlatformDisabledEnvVar())
}
