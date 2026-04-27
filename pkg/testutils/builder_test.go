// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestDatadogAgentBuilder_WithKSMCollectSecretMetrics(t *testing.T) {
	dda := NewDatadogAgentBuilder().WithKSMEnabled(true).WithKSMCollectSecretMetrics(false).Build()
	assert.NotNil(t, dda.Spec.Features.KubeStateMetricsCore)
	assert.Equal(t, ptr.To(false), dda.Spec.Features.KubeStateMetricsCore.CollectSecretMetrics)
}

func TestDatadogAgentBuilder_WithKSMCollectConfigMaps(t *testing.T) {
	dda := NewDatadogAgentBuilder().WithKSMEnabled(true).WithKSMCollectConfigMaps(false).Build()
	assert.NotNil(t, dda.Spec.Features.KubeStateMetricsCore)
	assert.Equal(t, ptr.To(false), dda.Spec.Features.KubeStateMetricsCore.CollectConfigMaps)
}
